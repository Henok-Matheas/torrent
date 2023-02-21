package peers

import (
	"fmt"
	"net"
	"strconv"
)

// Peer encodes connection information for a peer
type Peer struct {
	IP   net.IP
	Port uint16
}

// Unmarshal parses peer IP addresses and ports from a buffer
func Unmarshal(peersBin []byte) ([]Peer, error) {
	const peerSize = 6 // 4 for IP, 2 for port
	// numPeers := len(peersBin) / peerSize
	numPeers := 1
	if len(peersBin)%peerSize != 0 {
		err := fmt.Errorf("received malformed peers")
		return nil, err
	}
	peer := make([]byte, 4)
	// 10.6.250.226
	peer[0] = 127
	peer[1] = 0
	peer[2] = 0
	peer[3] = 1

	peers := make([]Peer, numPeers)
	for i := 0; i < numPeers; i++ {
		// offset := i * peerSize
		// peers[i].IP = net.IP(peersBin[offset : offset+4])
		// peers[i].Port = binary.BigEndian.Uint16([]byte(peersBin[offset+4 : offset+6]))
		peers[i].IP = net.IP(peer)
		peers[i].Port = 8080
		// fmt.Println(peers[i].IP.String(), strconv.Itoa(int(peers[i].Port)))
	}

	return peers, nil
}

func (p Peer) String() string {
	return net.JoinHostPort(p.IP.String(), strconv.Itoa(int(p.Port)))
}
