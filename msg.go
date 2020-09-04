package main

import "sync"

const (
	RTMP_DEFAULT_CHUNK_SIZE = 128
)

var (
	rtmpHeadPool = &sync.Pool{
		New: func() interface{} {
			return &ChunkHeader{}
		},
	}

	chunkMsgPool = &sync.Pool{
		New: func() interface{} {
			return &Chunk{}
		},
	}
)

func GetRtmpMsgData(chunk *Chunk) {

	switch chunk.MessageTypeID {

	}

}
