package main

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
