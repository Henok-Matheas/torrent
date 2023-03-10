package connection

import (
	"bytes"
	"fmt"
	"net"
	"time"
	"torrent/bitfield"
	"torrent/handshake"
	"torrent/message"
	"torrent/peers"
)

// A Connection is a TCP connection with a peer
type Connection struct {
	Conn         net.Conn
	Choked       bool
	PeerBitfield bitfield.Bitfield
	MyBitfield   bitfield.Bitfield
	peer         peers.Peer
	infoHash     [20]byte
	ID           [20]byte
}

func SendUnchoke(conn net.Conn) error {
	msg := message.Message{ID: message.Unchoke}
	_, err := conn.Write(msg.Serialize())
	return err
}

func completeHandshake(conn net.Conn, infohash, peerID [20]byte) (*handshake.Handshake, error) {
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{}) // Disable the deadline

	req := handshake.New(infohash, peerID)
	_, err := conn.Write(req.Serialize())
	if err != nil {
		return nil, err
	}

	res, err := handshake.Read(conn)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(res.InfoHash[:], infohash[:]) {
		return nil, fmt.Errorf("Expected infohash %x but got %x", res.InfoHash, infohash)
	}
	return res, nil
}

func recvBitfield(conn net.Conn) (bitfield.Bitfield, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetDeadline(time.Time{}) // Disable the deadline

	msg, err := message.Read(conn)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		err := fmt.Errorf("Expected bitfield but got %s", msg)
		return nil, err
	}
	if msg.ID != message.Bitfield {
		err := fmt.Errorf("Expected bitfield but got ID %d", msg.ID)
		return nil, err
	}

	return msg.Payload, nil
}

// NewSeeder connects with a seeder, completes a handshake, and receives a handshake
func NewSeeder(peer peers.Peer, peerID, infoHash [20]byte) (*Connection, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 3*time.Second)
	if err != nil {
		return nil, err
	}

	_, err = completeHandshake(conn, infoHash, peerID)
	if err != nil {
		conn.Close()
		return nil, err
	}

	bf, err := recvBitfield(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &Connection{
		Conn:         conn,
		Choked:       true,
		PeerBitfield: bf,
		peer:         peer,
		infoHash:     infoHash,
		ID:           peerID,
	}, nil
}

// New comp
func NewLeecher(peer peers.Peer, peerID, infoHash [20]byte) (*Connection, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 3*time.Second)
	if err != nil {
		return nil, err
	}

	_, err = completeHandshake(conn, infoHash, peerID)
	if err != nil {
		conn.Close()
		return nil, err
	}

	bf, err := recvBitfield(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &Connection{
		Conn:         conn,
		Choked:       true,
		PeerBitfield: bf,
		peer:         peer,
		infoHash:     infoHash,
		ID:           peerID,
	}, nil
}

// Read reads and consumes a message from the connection
func (c *Connection) Read() (*message.Message, error) {
	msg, err := message.Read(c.Conn)
	return msg, err
}

// SendRequest sends a Request message to the peer
func (c *Connection) SendRequest(index, begin, length int) error {
	req := message.FormatRequest(index, begin, length)
	_, err := c.Conn.Write(req.Serialize())
	return err
}

// SendUnchoke sends an Unchoke message to the peer
func (c *Connection) SendUnchoke() error {
	msg := message.Message{ID: message.Unchoke}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}
