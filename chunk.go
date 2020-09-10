package main

import (
	"encoding/binary"
	"errors"
	"rtmp/mem_pool"
	"rtmp/utils"
)

const (
	// 这里左移6位 其实是表示BasicHeader中 前两bit是 fmt 后6位才是StreamID 这里要为StreamID 留出位置
	// 至于为什么是0,1,2,3 这里和前面是对应的，fmt=0是全量消息, fmt=1为头部是7byte
	RtmpChunkHead12 = 0 << 6
	RtmpChunkHead8  = 1 << 6
	RtmpChunkHead4  = 2 << 6
	RtmpChunkHead1  = 3 << 6
)

type ChunkHeader struct {
	ChunkBasicHeader
	ChunkMessageHeader
	// 该字段在时间戳溢出时才会出现，因为时间戳只有 3byte
	ChunkExtendedTimestamp
	// ChunkData
}

func (h *ChunkHeader) Clone() *ChunkHeader {
	head := rtmpHeadPool.Get().(*ChunkHeader)

	head.ChunkStreamID = h.ChunkStreamID
	head.Timestamp = h.Timestamp
	head.MessageLength = h.MessageLength
	head.MessageTypeID = h.MessageTypeID
	head.MessageStreamID = h.MessageStreamID
	head.ExtendTimestamp = h.ExtendTimestamp

	return head
}

type Chunk struct {
	*ChunkHeader
	Body    []byte
	MsgData interface{}
}

// 共占用的大小不定，关键看ChunkStreamID 可能为 1byte,2byte,3byte
// 但是 ChunkType的大小是固定的，始终占用2bit
type ChunkBasicHeader struct {
	/*
		决定了后面 MessageHeader的格式，
	*/
	ChunkType byte // 2bit
	/*
		标识了一条特定的流通道 简称为CSID
	*/
	ChunkStreamID uint32 // 6bit
}

type ChunkMessageHeader struct {
	Timestamp       uint32 // 3byte
	MessageLength   uint32 // 2byte
	MessageTypeID   byte   //1 byte
	MessageStreamID uint32 // 4byte
}

type ChunkExtendedTimestamp struct {
	ExtendTimestamp uint32 `json:",omitempty"`
}

func (nc *NetConnection) beforEncodeChunk(payload []byte, size int) error {
	if size > RtmpMaxChunkSize || payload == nil || len(payload) == 0 {
		return errors.New("encodeChunk12 Error")
	}

	return nil
}

func (nc *NetConnection) afterEncodeChunk(payload []byte, size int) ([]byte, error) {

	if len(payload) > size {
		n, err := nc.writeFull(payload[0:size])
		if err != nil {
			return nil, err
		}
		need := payload[n:]
		return need, nil
	}
	nc.writeFull(payload)
	return nil, nil
}

func (nc *NetConnection) encodeChunk12(head *ChunkHeader, payload []byte, size int) ([]byte, error) {
	// 若 没有payload就不写了
	if err := nc.beforEncodeChunk(payload, size); err != nil {
		return nil, err
	}

	b := mem_pool.GetSlice(12)

	b[0] = byte(RtmpChunkHead12 + head.ChunkBasicHeader.ChunkStreamID)
	utils.BigEndian.PutUint24(b[1:], head.ChunkMessageHeader.Timestamp)
	utils.BigEndian.PutUint24(b[4:], head.ChunkMessageHeader.MessageLength)
	b[7] = head.ChunkMessageHeader.MessageTypeID
	// 这里写StreamID的时候一定要注意使用小端
	binary.LittleEndian.PutUint32(b[8:], uint32(head.ChunkMessageHeader.MessageStreamID))

	nc.writeFull(b)
	mem_pool.RecycleSlice(b)

	// 查看是否需要写 ExtendTimestamp
	if head.ChunkMessageHeader.Timestamp == 0xffffff {
		b := mem_pool.GetSlice(4)
		binary.LittleEndian.PutUint32(b, head.ChunkExtendedTimestamp.ExtendTimestamp)
		nc.writeFull(b)
		mem_pool.RecycleSlice(b)
	}

	// 开始写入payload
	// 检查payload是否大于 指定的size
	return nc.afterEncodeChunk(payload, size)
}

func (nc *NetConnection) encodeChunk1(head *ChunkHeader, payload []byte, size int) ([]byte, error) {

	if size > RtmpMaxChunkSize || payload == nil || len(payload) == 0 {
		return nil, errors.New("enCode Chunk1 Error")
	}

	basicHeader := byte(RtmpChunkHead1 + head.ChunkBasicHeader.ChunkStreamID)
	if err := nc.writeByte(basicHeader); err != nil {
		return nil, err
	}

	return nc.afterEncodeChunk(payload, size)
}
