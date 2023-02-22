package main

import (
	"log"
	"os"
	"strconv"

	"torrent/leecher"
	"torrent/seeder"
	"torrent/torrentfile"
)

func main() {
	portString := os.Args[1]
	file := os.Args[2]

	Port, err := strconv.Atoi(portString)
	if err != nil {
		log.Fatal("Port Number could not be parsed", err)
	}

	torrent, err := torrentfile.Unmarshal(file)
	if err != nil {
		log.Fatal(err)
		return
	}

	defer torrent.File.Close()

	leecher, err := leecher.CreateLeecher(torrent, uint16(Port))

	if err != nil {
		log.Fatal("Leecher could not be Initalized", err)
		return
	}

	leecher.Download()

	seeder.HandleSeed(&torrent, uint16(Port))
}
