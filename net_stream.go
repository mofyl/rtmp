package main

//
//func processStream(nc *bufio.ReadWriter) (*Chunk, error) {
//
//	head, err := nc.ReadByte()
//	if err != nil {
//		return nil, err
//	}
//	// 这里Head的构成是  ChunkType(2bit) | ChunkStreamID(6bit)
//	// 先获取streamId
//	// 0x3f : 0011 1111 这样就可以保留后6bit的值 将前2bit置为0
//	chunkStreamID := uint32(head & 0x3f)
//	// 获取chunkType
//	// 0xc0: 1100 0000 这样可以将前2bit的值 保留下来，后面6bit的值 然后左移6bit 将 chunkType取出来
//	chunkType := uint32(head&0xc0) >> 6
//	// 若chunkStreamID 为 0或1那么就表示chunkBasicHeader占用的不是1个byte 则需要进一步处理
//	chunkStreamID, err = calcChunkStreamID(chunkStreamID, nc)
//
//	if err != nil {
//		return nil, err
//	}
//	// 获取ChunkHeader
//
//	fmt.Println(chunkStreamID, chunkType)
//	return nil, nil
//}
