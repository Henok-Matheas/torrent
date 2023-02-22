package main

import (
	"log"
	"os"
	"strconv"

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

	// integrity checker and the other bitfield setter and all need to be here.
	torrent, err := torrentfile.Unmarshal(file)
	if err != nil {
		log.Fatal(err)
		return
	}

	defer torrent.File.Close()

	// create a leecher
	leecher, err := torrent.CreateLeecher(uint16(Port))

	if err != nil {
		log.Fatal("Leecher could not be Initalized", err)
		return
	}
	leecher.Download()

	// let the seeder handle seeding
	seeder.HandleSeed(&torrent, uint16(Port))
}
