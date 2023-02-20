package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type messageID uint8

const (
	// MsgChoke chokes the receiver
	MsgChoke messageID = 0
	// MsgUnchoke unchokes the receiver
	MsgUnchoke messageID = 1
	// MsgInterested expresses interest in receiving data
	MsgInterested messageID = 2
	// MsgNotInterested expresses disinterest in receiving data
	MsgNotInterested messageID = 3
	// MsgHave alerts the receiver that the sender has downloaded a piece
	MsgHave messageID = 4
	// MsgBitfield encodes which pieces that the sender has downloaded
	MsgBitfield messageID = 5
	// MsgRequest requests a block of data from the receiver
	MsgRequest messageID = 6
	// MsgPiece delivers a block of data to fulfill a request
	MsgPiece messageID = 7
	// MsgCancel cancels a request
	MsgCancel messageID = 8
)

// Message stores ID and payload of a message
type Message struct {
	ID      messageID
	Payload []byte
}

type Request struct {
	Index      int
	BlockBegin int
	Begin      int
	End        int
	BlockSize  int
}

type Handshake struct {
	Pstr     string
	InfoHash [20]byte
	PeerID   [20]byte
}

func FormatChoke() *Message {
	return &Message{ID: MsgChoke}
}

// New creates a new handshake with the standard pstr
func New(infoHash, peerID [20]byte) *Handshake {
	return &Handshake{
		Pstr:     "BitTorrent protocol",
		InfoHash: infoHash,
		PeerID:   peerID,
	}
}

func (h *Handshake) Serialize() []byte {
	buf := make([]byte, len(h.Pstr)+49)
	buf[0] = byte(len(h.Pstr))
	curr := 1
	curr += copy(buf[curr:], h.Pstr)
	curr += copy(buf[curr:], make([]byte, 8)) // 8 reserved bytes
	curr += copy(buf[curr:], h.InfoHash[:])
	curr += copy(buf[curr:], h.PeerID[:])

	fmt.Println(buf, curr)
	return buf
}

func Read(r io.Reader) (*Handshake, error) {
	lengthBuf := make([]byte, 1)
	_, err := io.ReadFull(r, lengthBuf)

	if err != nil {
		return nil, err
	}

	pstrlen := int(lengthBuf[0])

	if pstrlen == 0 {
		err := fmt.Errorf("pstrlen cannot be 0")
		return nil, err
	}

	handshakeBuf := make([]byte, 48+pstrlen)
	_, err = io.ReadFull(r, handshakeBuf)
	if err != nil {
		return nil, err
	}

	var infoHash, peerID [20]byte

	copy(infoHash[:], handshakeBuf[pstrlen+8:pstrlen+8+20])
	copy(peerID[:], handshakeBuf[pstrlen+8+20:])

	h := Handshake{
		Pstr:     string(handshakeBuf[0:pstrlen]),
		InfoHash: infoHash,
		PeerID:   peerID,
	}

	fmt.Println("Successful")

	return &h, nil
}

type Backend struct {
	net.Conn
	Reader *bufio.Reader
	Writer *bufio.Writer
}

var backendQueue chan *Backend
var requestBytes map[string]int64
var requestLock sync.Mutex

func init() {
	requestBytes = make(map[string]int64)
	backendQueue = make(chan *Backend, 10)
}

func getBackend() (*Backend, error) {
	select {
	case be := <-backendQueue:
		return be, nil
	case <-time.After(100 * time.Millisecond):
		be, err := net.Dial("tcp", "127.0.0.1:8081")
		if err != nil {
			return nil, err
		}

		return &Backend{
			Conn:   be,
			Reader: bufio.NewReader(be),
			Writer: bufio.NewWriter(be),
		}, nil
	}
}

func queueBackend(be *Backend) {
	select {
	case backendQueue <- be:
	case <-time.After(1 * time.Second):
		be.Close()
	}
}

func updateStats(req *http.Request, resp *http.Response) int64 {
	requestLock.Lock()
	defer requestLock.Unlock()

	bytes := requestBytes[req.URL.Path] + resp.ContentLength
	requestBytes[req.URL.Path] = bytes
	return bytes
}

func (m *Message) Serialize() []byte {
	if m == nil {
		return make([]byte, 4)
	}
	length := uint32(len(m.Payload) + 1) // +1 for id
	buf := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buf[0:4], length)
	fmt.Println(m.ID)
	buf[4] = byte(m.ID)
	copy(buf[5:], m.Payload)
	fmt.Println("buffer", buf)
	return buf
}

func ParseUnchoke(msg *Message) (int, error) {
	if msg.ID != MsgUnchoke {
		return 0, fmt.Errorf("Expected HAVE (ID %d), got ID %d", MsgUnchoke, msg.ID)
	}
	if len(msg.Payload) != 4 {
		return 0, fmt.Errorf("Expected payload length 4, got length %d", len(msg.Payload))
	}
	index := int(binary.BigEndian.Uint32(msg.Payload))
	return index, nil
}

// Read parses a message from a stream. Returns `nil` on keep-alive message
func readMessage(r io.Reader) (*Message, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	// keep-alive message
	if length == 0 {
		return nil, nil
	}

	messageBuf := make([]byte, length)
	_, err = io.ReadFull(r, messageBuf)
	if err != nil {
		return nil, err
	}

	m := Message{
		ID:      messageID(messageBuf[0]),
		Payload: messageBuf[1:],
	}

	return &m, nil
}

func SendUnchoke(conn net.Conn) error {
	msg := Message{ID: MsgUnchoke}
	_, err := conn.Write(msg.Serialize())
	return err
}

func parseRequest(msg *Message) (*Request, error) {
	if msg.ID != MsgRequest {
		return nil, fmt.Errorf("Expected REQUEST (ID %d), got ID %d", MsgRequest, msg.ID)
	}

	if len(msg.Payload) < 12 {
		return nil, fmt.Errorf("Payload too short. %d < 8", len(msg.Payload))
	}
	index := int(binary.BigEndian.Uint32(msg.Payload[0:4]))
	blockStart := int(binary.BigEndian.Uint32(msg.Payload[4:8]))
	blockSize := int(binary.BigEndian.Uint32(msg.Payload[8:12]))
	pieceSize := 262144
	fileSize := 471859200
	begin := index*pieceSize + blockStart
	end := begin + blockSize

	if end > fileSize {
		end = fileSize
		blockSize = end - begin
	}
	request := Request{
		Index:      index,
		BlockBegin: blockStart,
		Begin:      begin,
		End:        end,
		BlockSize:  blockSize,
	}

	fmt.Printf("begin: %s and end: %s \n", begin, end)

	return &request, nil
}

func FormatPiece(request *Request, data []byte) *Message {
	payload := make([]byte, 8+len(data))
	binary.BigEndian.PutUint32(payload[0:4], uint32(request.Index))
	binary.BigEndian.PutUint32(payload[4:8], uint32(request.BlockBegin))
	for idx := 0; idx < len(data); idx++ {
		payload[8+idx] = data[idx]
	}
	return &Message{ID: MsgPiece, Payload: payload}
}

func Upload(msg *Message, conn net.Conn, path string) error {
	request, err := parseRequest(msg)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	defer file.Close()
	if err != nil {
		fmt.Println("error is", err)
		return err
	}
	data := make([]byte, request.BlockSize)
	read, err := file.ReadAt(data, int64(request.Begin))

	if err != nil {
		fmt.Println("Something went wrong while uploading", read)
		return err
	}

	message := FormatPiece(request, data)

	conn.Write(message.Serialize())

	// conn.Write()
	return nil
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	res, err := Read(reader)

	if err != nil {
		return
	}
	conn.Write(res.Serialize())
	Payload := make([]byte, 255)
	for i := 0; i < len(Payload); i++ {
		Payload[i] = 255
	}
	bitField := Message{ID: MsgBitfield, Payload: Payload}
	conn.Write(bitField.Serialize())
	Unchoke, err := readMessage(reader)
	fmt.Println(Unchoke)
	if err != nil {
		return
	}

	Interested, err := readMessage(reader)
	fmt.Println(Interested)
	if err != nil {
		return
	}

	// maybe add a checker if the number of goroutines hasn't been overloaded
	error := SendUnchoke(conn)

	if error != nil {
		return
	}

	for {
		requestMessage, err := readMessage(reader)
		go Upload(requestMessage, conn, "/home/henok/Desktop/golang/torrent/seeder/debian-edu-11.6.0-amd64-netinst.iso")
		fmt.Println("request", requestMessage)
		if err != nil {
			fmt.Println("Error", err)
			return
		}
	}

	// now we want a handler to upload the files
	// go handleUpload(reader, conn)

	// req, err := http.ReadRequest(reader)
	// if err != nil {
	// 	if err != io.EOF {
	// 		log.Printf("Failed to Load Request %s", err)
	// 	}
	// 	return
	// }

	// be, err := getBackend()
	// if err != nil {
	// 	return
	// }

	// be_reader := bufio.NewReader(be)
	// if err := req.Write(be); err == nil {
	// 	be.Writer.Flush()

	// 	resp, err := http.ReadResponse(be_reader, req)
	// 	if err == nil {
	// 		bytes := updateStats(req, resp)
	// 		resp.Header.Set("X-Bytes", strconv.FormatInt(bytes, 10))

	// 		err := resp.Write(conn)
	// 		if err == nil {
	// 			log.Printf("%s: %d", req.URL.Path, resp.StatusCode)
	// 		}
	// 	}
	// }
	// go queueBackend(be)

}

func main() {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to listen: %s", err)
	}
	for {
		conn, err := ln.Accept()
		if err == nil {
			fmt.Println("Accepted Connection", conn)
			go handleConnection(conn)
		}
	}
}
