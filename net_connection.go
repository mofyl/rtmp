package main

import (
	"bufio"
	"net"
	"rtmp/mem_pool"
)

type NetConnection struct {
	conn           net.Conn
	rw             *bufio.ReadWriter
	writeChunkSize int64
	readChunkSize  int64

	streamID   uint32
	rtmpHeader map[uint32]*ChunkHeader // rtmp 传输的头部可能是不完全的，但是第一个一定是完整的 这个完整的我们需要记录下来
	appName    string

	readSeqNum  uint32
	writeSeqNum uint32
}

func (conn *NetConnection) addReadSeqNum() {
	conn.readSeqNum++
}

func (conn *NetConnection) addWriteSeqNum() {
	conn.writeSeqNum++
}

func (conn *NetConnection) readByte() (byte, error) {
	conn.addReadSeqNum()
	return conn.rw.ReadByte()
}

func (conn *NetConnection) readChunk() (*Chunk, error) {

	// 先读 BasicHeader
	// 先读 先读BasicHeader中的 ChunkType

	basicHeader, err := conn.readByte()

	if err != nil {
		return nil, err
	}

	streamID := uint32(basicHeader & 0x3f) //  0011 1111
	chunkType := (basicHeader & 0xc3) >> 6 // 1100 0000
	streamID, err = conn.getChunkStreamID(streamID)

	if err != nil {
		return nil, err
	}

	fullHead, ok := conn.rtmpHeader[streamID]

	if !ok {
		fullHead = &ChunkHeader{}
		fullHead.ChunkStreamID = streamID
		fullHead.ChunkType = chunkType
		conn.rtmpHeader[streamID] = fullHead
	}

	conn.buildChunkHeader(chunkType, fullHead)
	return nil, nil
}

/*
	根据ChunkType 的不同 MsgHeader可以分为 0，1，2，3
		0时 MsgHeader为全量头部 占用11个字节
		1时 MsgHeader为部分头部 占用7byte
		2时 MsgHeader为部分头部 占用 3byte
		3时 MsgHeader为部分头部 占用0byte

*/
func (conn *NetConnection) buildChunkHeader(chunkType byte, chunkHeader *ChunkHeader) (*ChunkHeader, error) {

	switch chunkType {
	case 0:
		// 前三个byte为 timestamp
		b := mem_pool.GetSlice(3)

	case 1:
	case 2:
	case 3:
	}
	return nil, nil
}

func (conn *NetConnection) getChunkStreamID(csid uint32) (uint32, error) {
	chunkStreamID := csid

	switch csid {
	case 0:
		// 表示占用2个字节
		b1, err := conn.readByte()
		if err != nil {
			return chunkStreamID, err
		}

		chunkStreamID = 64 + uint32(b1)
	case 1:
		// 表示占用3个字节
		b1, err := conn.readByte()
		if err != nil {
			return chunkStreamID, err
		}

		b2, err := conn.readByte()
		if err != nil {
			return chunkStreamID, err
		}

		//chunkStreamID = uint32(b1) + 64 + uint32(b2) * 256
		chunkStreamID = uint32(b1) + 64 + (uint32(b2) << 8)
	}

	return chunkStreamID, nil
}
