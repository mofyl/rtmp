package main

import (
	"bufio"
	"fmt"
)

func processStream(nc *bufio.ReadWriter) error{

	head , err := nc.ReadByte()
	if err != nil {
		return err
	}
	// 这里Head的构成是  ChunkType(2bit) | ChunkStreamID(6bit)
	// 先获取streamId
	// 0x3f : 0011 1111 这样就可以保留后6bit的值 将前2bit置为0
	chunkStreamID := uint32(head & 0x3f)
	// 获取chunkType
	// 0xc0: 1100 0000 这样可以将前2bit的值 保留下来，后面6bit的值 然后左移6bit 将 chunkType取出来
	chunkType := uint32(head & 0xc0) >> 6
	// 若chunkStreamID 为 0或1那么就表示chunkBasicHeader占用的不是1个byte 则需要进一步处理
	chunkStreamID , err = calcChunkStreamID(chunkStreamID , nc)

	if err != nil {
		return err
	}




	fmt.Println(chunkStreamID , chunkType)
	return nil
}

func calcChunkStreamID(csid uint32 , nc *bufio.ReadWriter)(uint32 , error) {
	chunkStreamID := csid

	switch csid {
	case 0 :
		// 表示占用2个字节
		b1 , err := nc.ReadByte()
		if err != nil {
			return chunkStreamID , err
		}

		chunkStreamID = 64 + uint32(b1)
	case 1 :
		// 表示占用3个字节
		b1 , err := nc.ReadByte()
		if err != nil {
			return chunkStreamID , err
		}

		b2 , err := nc.ReadByte()
		if err != nil {
			return chunkStreamID , err
		}

		//chunkStreamID = uint32(b1) + 64 + uint32(b2) * 256
		chunkStreamID = uint32(b1) + 64 + (uint32(b2) << 8)
	}

	return chunkStreamID , nil
}
