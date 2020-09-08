package main

import (
	"bufio"
	"fmt"
	"net"
	"rtmp/mem_pool"
)

func main() {

	mem_pool.InitPool()

	l, err := net.Listen("tcp4", "127.0.0.1:9000")

	if err != nil {
		fmt.Println("listen err is ", err.Error())
		return
	}
	fmt.Println("Net is Listen Addr is ", l.Addr().String())
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Accept Err is ", err.Error())
			continue
		}
		fmt.Println("Success")
		//nc := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

		nc := &NetConnection{
			conn:           conn,
			rw:             bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
			writeChunkSize: RtmpDefaultChunkSize,
			readChunkSize:  RtmpDefaultChunkSize,
			rtmpHeader:     make(map[uint32]*ChunkHeader),
			rtmpBody:       make(map[uint32][]byte),
		}

		err = handshake(nc.rw)

		if err != nil {
			fmt.Println("handshake err is ", err.Error())
			conn.Close()
			break
		}
		//// 开始读Chunk
		nc.OnConnect()

	}

	l.Close()
}
