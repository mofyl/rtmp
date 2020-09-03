package main

import (
	"bufio"
	"fmt"
	"net"
)

func main() {

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
			writeChunkSize: RTMP_DEFAULT_CHUNK_SIZE,
			readChunkSize:  RTMP_DEFAULT_CHUNK_SIZE,
			rtmpHeader:     make(map[uint32]*ChunkHeader),
		}

		err = handshake(nc.rw)

		if err != nil {
			fmt.Println("handshake err is ", err.Error())
			conn.Close()
			break
		}
		//// 开始读Chunk
		//if _, err := processStream(nc.rw); err != nil {
		//	fmt.Println("processStream err is ", err)
		//	conn.Close()
		//	break
		//}

	}

	l.Close()
}
