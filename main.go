package main

import (
	"log"
	"os"
	"strconv"

	"torrent/seeder"
	"torrent/torrentfile"
)

func main() {
	inPath := os.Args[1]
	portString := os.Args[2]

	Port, err := strconv.Atoi(portString)

	tf, err := torrentfile.Open(inPath)
	if err != nil {
		log.Fatal(err)
	}

	torrent, err := tf.DownloadToFile(tf.Name, uint16(Port))
	if err != nil {
		log.Fatal(err)
	}

	seeder.HandleServer(torrent)
}
