package torrentfile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"torrent/bitfield"
	"torrent/leecher"

	"github.com/jackpal/bencode-go"
)

// TorrentFile is the content inside the .torrent file
type TorrentFile struct {
	Announce    string
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

// Torrent is simmilar to TorrentFile but with the file descriptor and bitfield
type Torrent struct {
	Announce    string
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
	File        *os.File
	Bitfield    bitfield.Bitfield
}

type bencodeInfo struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
}

type bencodeTorrent struct {
	Announce string      `bencode:"announce"`
	Info     bencodeInfo `bencode:"info"`
}

// gives us the infoHash
func (i *bencodeInfo) hash() ([20]byte, error) {
	var buf bytes.Buffer
	err := bencode.Marshal(&buf, *i)
	if err != nil {
		return [20]byte{}, err
	}
	h := sha1.Sum(buf.Bytes())
	return h, nil
}

// hashes the individual pieces one by one
func (i *bencodeInfo) hashPieces() ([][20]byte, error) {
	hashLen := 20 // Length of SHA-1 hash
	buf := []byte(i.Pieces)
	if len(buf)%hashLen != 0 {
		err := fmt.Errorf("received malformed pieces of length %d", len(buf))
		return nil, err
	}
	numHashes := len(buf) / hashLen
	hashes := make([][20]byte, numHashes)

	for i := 0; i < numHashes; i++ {
		copy(hashes[i][:], buf[i*hashLen:(i+1)*hashLen])
	}
	return hashes, nil
}

// changes bencode torrent into torrentfile
func (bto *bencodeTorrent) toTorrentFile() (TorrentFile, error) {
	infoHash, err := bto.Info.hash()
	if err != nil {
		return TorrentFile{}, err
	}
	pieceHashes, err := bto.Info.hashPieces()

	if err != nil {
		return TorrentFile{}, err
	}
	t := TorrentFile{
		Announce:    bto.Announce,
		InfoHash:    infoHash,
		PieceHashes: pieceHashes,
		PieceLength: bto.Info.PieceLength,
		Length:      bto.Info.Length,
		Name:        bto.Info.Name,
	}
	return t, nil
}

// calculates the bound for a single piece
func (t *Torrent) calculateBoundsForPiece(index int) (begin int, end int) {
	begin = index * t.PieceLength
	end = begin + t.PieceLength
	if end > t.Length {
		end = t.Length
	}
	return begin, end
}

// calculates the size for a single piece
func (t *Torrent) calculatePieceSize(index int) int {
	begin, end := t.calculateBoundsForPiece(index)
	return end - begin
}

// checks if the given piece hash and the hash from the torrent file match
func checkIntegrity(piece int, piecehash [20]byte, buf []byte) error {
	bufferhash := sha1.Sum(buf)
	if !bytes.Equal(bufferhash[:], piecehash[:]) {
		return fmt.Errorf("piece %d failed integrity check", piece)
	}
	return nil
}

// Checks if the pieces have been succesfully downloaded
func (torrent Torrent) Restore() {
	for piece, hash := range torrent.PieceHashes {
		begin, _ := torrent.calculateBoundsForPiece(piece)
		length := torrent.calculatePieceSize(piece)

		data := make([]byte, length)
		_, err := torrent.File.ReadAt(data, int64(begin))

		if err != nil {
			fmt.Errorf("something went wrong while trying to Reading File", err)
		}

		// Check Integrity of the piece
		integrityerr := checkIntegrity(piece, hash, data)

		if integrityerr == nil {
			log.Printf("Restored Piece: %d from Disk\n", piece)
			torrent.Bitfield.SetPiece(piece)
		}
	}
}

func (torrentFile TorrentFile) toTorrent() (Torrent, error) {
	outFile, err := os.OpenFile(torrentFile.Name, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return Torrent{}, err
	}
	defer outFile.Close()

	// Instantiate Bitfield
	lengthPieces := float64(len(torrentFile.PieceHashes))
	const ByteSize = float64(8)
	bitField := make(bitfield.Bitfield, int(math.Ceil(float64(lengthPieces)/ByteSize)))

	t := Torrent{
		Announce:    torrentFile.Announce,
		InfoHash:    torrentFile.InfoHash,
		PieceHashes: torrentFile.PieceHashes,
		PieceLength: torrentFile.PieceLength,
		Length:      torrentFile.Length,
		Name:        torrentFile.Name,
		File:        outFile,
		Bitfield:    bitField,
	}

	t.Restore()

	return t, nil
}

// Unmarshal unmarshals .torrent file to torrent struct
func Unmarshal(path string) (Torrent, error) {
	file, err := os.Open(path)
	if err != nil {
		return Torrent{}, err
	}

	bto := bencodeTorrent{}
	err = bencode.Unmarshal(file, &bto)
	if err != nil {
		return Torrent{}, err
	}

	torrentFile, err := bto.toTorrentFile()

	if err != nil {
		return Torrent{}, fmt.Errorf("Something went wrong while parsing TorrentFile", err)
	}

	return torrentFile.toTorrent()

}

// Creates a Leecher
func (t *Torrent) CreateLeecher(Port uint16) (*leecher.Leecher, error) {
	var peerID [20]byte
	_, err := rand.Read(peerID[:])
	if err != nil {
		return nil, err
	}

	log.Printf("Listening on Ip: %s and port : %d", net.IP(peerID[:]).String(), Port)

	peers, err := t.requestPeers(peerID, Port)
	if err != nil {
		return nil, err
	}

	leecher := leecher.Leecher{
		Peers:       peers,
		PeerID:      peerID,
		Port:        Port,
		InfoHash:    t.InfoHash,
		PieceHashes: t.PieceHashes,
		PieceLength: t.PieceLength,
		Length:      t.Length,
		Name:        t.Name,
		File:        t.File,
		Bitfield:    t.Bitfield,
	}
	return &leecher, nil
}
