# torrent
A torrent seeder and leecher using go

## Note
The Torrent implementation here can download from remote seeders but the seeder can only seed to local peers.

## To leech a Torrent
run the code: 
go run main.go <Insert Port> <Insert Torrent>

once the leecher has finished downloading the file then you can replace the peers found by the tracker with your own peer that is running in the same network.

to do that follow the steps below:
1. go to the peers.go file
2. comment out line 19
3. uncomment line 20
4. comment out lines [33-35]
5. uncomment lines 36 and 37.
6. run the command go run main.go <Insert Port> <Insert Torrent>.
