package tracker

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

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

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err != io.EOF {
				log.Printf("Failed to Load Request %s", err)
			}
			return
		}

		be, err := getBackend()
		if err != nil {
			return
		}

		be_reader := bufio.NewReader(be)
		if err := req.Write(be); err == nil {
			be.Writer.Flush()

			resp, err := http.ReadResponse(be_reader, req)
			if err == nil {
				bytes := updateStats(req, resp)
				resp.Header.Set("X-Bytes", strconv.FormatInt(bytes, 10))

				err := resp.Write(conn)
				if err == nil {
					log.Printf("%s: %d", req.URL.Path, resp.StatusCode)
				}
			}
		}
		go queueBackend(be)

	}

}

func main() {
	ln, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Fatalf("Failed to listen: %s", err)
	}
	for {
		_, err := ln.Accept()
		if err == nil {
			fmt.Println("Tracker Accepted Connection from", ln.Addr().Network())
		}
	}
}
