package torrentfil

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
	"torrent/p2p"

	"github.com/jackpal/bencode-go"
)

// TorrentFile is the content inside the .torrent file and adds the file handle and bitfield to it
type TorrentFile struct {
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

// Download downloads a torrent and writes it to a file
func (t *TorrentFile) DownloadToFile(Port uint16) (*p2p.Torrent, error) {
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

	lengthPieces := float64(len(t.PieceHashes))
	const ByteSize = float64(8)
	bitField := make(bitfield.Bitfield, int(math.Ceil(float64(lengthPieces)/ByteSize)))

	outFile, err := os.OpenFile(t.Name, os.O_RDWR|os.O_CREATE, 0666)

	if err != nil {
		return nil, err
	}
	defer outFile.Close()

	torrent := p2p.Torrent{
		Peers:       peers,
		PeerID:      peerID,
		Port:        Port,
		InfoHash:    t.InfoHash,
		PieceHashes: t.PieceHashes,
		PieceLength: t.PieceLength,
		Length:      t.Length,
		Name:        t.Name,
		File:        outFile,
		Bitfield:    bitField,
	}
	downloaderr := torrent.Download()
	if downloaderr != nil {
		return nil, downloaderr
	}

	return &torrent, nil
}

// Unmarshal unmarshals .torrent file to torrent struct
func Unmarshal(path string) (TorrentFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return TorrentFile{}, err
	}
	defer file.Close()

	bto := bencodeTorrent{}
	err = bencode.Unmarshal(file, &bto)
	if err != nil {
		return TorrentFile{}, err
	}
	return bto.toTorrentFile()
}

func (i *bencodeInfo) hash() ([20]byte, error) {
	var buf bytes.Buffer
	err := bencode.Marshal(&buf, *i)
	if err != nil {
		return [20]byte{}, err
	}
	h := sha1.Sum(buf.Bytes())
	return h, nil
}

func (i *bencodeInfo) splitPieceHashes() ([][20]byte, error) {
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

func (bto *bencodeTorrent) toTorrentFile() (TorrentFile, error) {
	infoHash, err := bto.Info.hash()
	if err != nil {
		return TorrentFile{}, err
	}
	pieceHashes, err := bto.Info.splitPieceHashes()

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

// Creates a Leecher
func (t *TorrentFile) createLeecher(Port uint16) (*leecher.Torrent, error) {
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

	lengthPieces := float64(len(t.PieceHashes))
	const ByteSize = float64(8)
	bitField := make(bitfield.Bitfield, int(math.Ceil(float64(lengthPieces)/ByteSize)))

	outFile, err := os.OpenFile(t.Name, os.O_RDWR|os.O_CREATE, 0666)

	// the bitfield needs to have been set

	if err != nil {
		return nil, err
	}
	defer outFile.Close()

	torrent := leecher.Torrent{
		Peers:       peers,
		PeerID:      peerID,
		Port:        Port,
		InfoHash:    t.InfoHash,
		PieceHashes: t.PieceHashes,
		PieceLength: t.PieceLength,
		Length:      t.Length,
		Name:        t.Name,
		File:        outFile,
		Bitfield:    bitField,
	}
	return &torrent, nil
}
