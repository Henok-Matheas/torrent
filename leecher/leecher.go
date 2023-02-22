package leecher

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"log"
	"net"
	"time"

	"torrent/connection"
	"torrent/message"
	"torrent/peers"
	"torrent/torrentfile"
)

// MaxBlockSize is the largest number of bytes a request can ask for
const MaxBlockSize = 16384

// MaxRequests is the number of unfulfilled requests a client can queue for
const MaxRequests = 5

// Leecher holds all the data required to download a torrent from a list of peers
type Leecher struct {
	Peers   []peers.Peer
	PeerID  [20]byte
	Port    uint16
	Torrent torrentfile.Torrent
}

type pieceWork struct {
	index  int
	hash   [20]byte
	length int
}

type pieceResult struct {
	index int
	buf   []byte
}

type pieceProgress struct {
	index      int
	client     *connection.Connection
	buf        []byte
	downloaded int
	requested  int
	backlog    int
}

// Creates a Leecher
func CreateLeecher(t torrentfile.Torrent, Port uint16) (*Leecher, error) {
	var peerID [20]byte
	_, err := rand.Read(peerID[:])
	if err != nil {
		return nil, err
	}

	log.Printf("Listening on Ip: %s and port : %d", net.IP(peerID[:]).String(), Port)

	peers, err := t.GetPeers(peerID, Port)
	if err != nil {
		return nil, err
	}

	leecher := Leecher{
		Peers:   peers,
		PeerID:  peerID,
		Port:    Port,
		Torrent: t,
	}
	return &leecher, nil
}

func (state *pieceProgress) readMessage() error {
	msg, err := state.client.Read() // this call blocks
	if err != nil {
		return err
	}

	if msg == nil { // keep-alive
		return nil
	}

	switch msg.ID {
	case message.Unchoke:
		state.client.Choked = false
	case message.Piece:
		n, err := message.ParsePiece(state.index, state.buf, msg)
		if err != nil {
			return err
		}
		state.downloaded += n
		state.backlog--
	}
	return nil
}

func attemptDownloadPiece(c *connection.Connection, pw *pieceWork) ([]byte, error) {
	state := pieceProgress{
		index:  pw.index,
		client: c,
		buf:    make([]byte, pw.length),
	}
	// Setting a deadline helps get unresponsive peers unstuck.
	// 30 seconds is more than enough time to download a 262 KB piece
	c.Conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer c.Conn.SetDeadline(time.Time{}) // Disable the deadline

	for state.downloaded < pw.length {
		// If unchoked, send requests until we have enough unfulfilled requests
		if !state.client.Choked {
			for state.backlog < MaxRequests && state.requested < pw.length {
				blockSize := MaxBlockSize
				// Last block might be shorter than the typical block
				if pw.length-state.requested < blockSize {
					blockSize = pw.length - state.requested
				}

				err := c.SendRequest(pw.index, state.requested, blockSize)
				if err != nil {
					return nil, err
				}
				state.backlog++
				state.requested += blockSize
			}
		}

		err := state.readMessage()
		if err != nil {
			return nil, err
		}
	}

	fmt.Println("attempt failed")
	return state.buf, nil
}

func checkIntegrity(pw *pieceWork, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], pw.hash[:]) {
		return fmt.Errorf("index %d failed integrity check", pw.index)
	}
	return nil
}

func (t *Leecher) startDownloadWorker(peer peers.Peer, workQueue chan *pieceWork, results chan *pieceResult) {
	c, err := connection.NewSeeder(peer, t.PeerID, t.Torrent.InfoHash)
	if err != nil {
		log.Printf("Could not handshake with %s. Disconnecting\n", peer.IP)
		return
	}
	defer c.Conn.Close()
	log.Printf("Completed handshake with %s\n", peer.IP)

	for pw := range workQueue {
		fmt.Println(pw)
		if !c.PeerBitfield.HasPiece(pw.index) {
			workQueue <- pw // Put piece back on the queue
			fmt.Println("putting back")
			continue
		}

		// Download the piece
		buf, err := attemptDownloadPiece(c, pw)
		if err != nil {
			log.Println("Exiting", err)
			workQueue <- pw // Put piece back on the queue
			return
		}

		err = checkIntegrity(pw, buf)
		if err != nil {
			log.Printf("Piece #%d failed integrity check\n", pw.index)
			workQueue <- pw // Put piece back on the queue
			continue
		}

		fmt.Println("this")
		results <- &pieceResult{pw.index, buf}
	}
}

// Download downloads the torrent. This writes to the file as soon as the piece is downloaded.
func (t *Leecher) Download() error {
	downloaded := 0

	workQueue := make(chan *pieceWork, len(t.Torrent.PieceHashes))
	results := make(chan *pieceResult)
	for index, hash := range t.Torrent.PieceHashes {
		length := t.Torrent.PieceSize(index)

		pieceWork := pieceWork{index, hash, length}

		if t.Torrent.Bitfield.HasPiece(index) {
			downloaded += 1
		} else {
			workQueue <- &pieceWork
		}
	}

	// Start workers
	for _, peer := range t.Peers {
		go t.startDownloadWorker(peer, workQueue, results)
	}

	for downloaded < len(t.Torrent.PieceHashes) {
		res := <-results
		begin, _ := t.Torrent.PieceBound(res.index)

		// Write to file as soon as it is downloaded
		_, err := t.Torrent.File.WriteAt(res.buf, int64(begin))
		if err != nil {
			return err
		}
		downloaded++

		log.Printf("(%0.2f%%) Downloaded\n", float64(downloaded)/float64(len(t.Torrent.PieceHashes))*100)
	}
	log.Printf("Finished Downloading\n")
	close(workQueue)

	return nil
}
