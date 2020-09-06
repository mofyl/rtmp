package main

import (
	"encoding/binary"
	"sync"
)

const (
	RTMP_DEFAULT_CHUNK_SIZE = 128

	// Chunk
	RTMP_MSG_CHUNK_SIZE = 1
	RTMP_MSG_ABORT      = 2

	// RTMP
	RTMP_MSG_ACK          = 3
	RTMP_MSG_USER_CONTROL = 4
	RTMP_MSG_ACK_SIZE     = 5
	RTMP_MSG_BANDWIDTH    = 6
	RTMP_MSG_EDGE         = 7
	RTMP_MSG_AUDIO        = 8
	RTMP_MSG_VIDEO        = 9

	// User Control Event
	// 服务端向客户端发送本事件通知对方 一个流开始起作用并可以用于通讯，
	// 默认情况下 服务器成功的从客户端接收连接命令后发送本事件 streamID=0，事件数据为开始起作用的流ID
	RTMP_USER_STREAM_BEGIN       = 0
	RTMP_USER_STREAM_EOF         = 1
	RTMP_USER_STREAM_DRY         = 2
	RTMP_USER_SET_BUFFLEN        = 3
	RTMP_USER_STREAM_IS_RECORDED = 4
	RTMP_USER_PING_REQUEST       = 6
	RTMP_USER_PING_REPONSE       = 7
	RTMP_USER_EMPTY              = 31
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

type UserControlMessage struct {
	EventType uint16
	EventData []byte
}

type StreamIDMessage struct {
	UserControlMessage
	StreamID uint32
}

type SetBufferMessage struct {
	StreamIDMessage
	Millisecond uint32
}

func (msg *SetBufferMessage) Encode() []byte {
	b := make([]byte, 10)
	binary.BigEndian.PutUint16(b, msg.EventType)
	binary.BigEndian.PutUint32(b[2:], msg.StreamID)
	binary.BigEndian.PutUint32(b[6:], msg.Millisecond)
	msg.EventData = b[2:]
	return b
}

type PingRequestMessage struct {
	UserControlMessage
	Timestamp uint32
}

func GetRtmpMsgData(chunk *Chunk) {

	switch chunk.MessageTypeID {
	// 这里由于 1,2,3,5的body都是 4byte 所以可以一起解析
	case RTMP_MSG_CHUNK_SIZE, RTMP_MSG_ABORT, RTMP_MSG_ACK, RTMP_MSG_ACK_SIZE:
		chunk.MsgData = binary.BigEndian.Uint32(chunk.Body)
	case RTMP_MSG_USER_CONTROL:
		// 这里分为 EventType 和EventData
		// 前2byte是 EventType 后面的就是EventData
		userData := UserControlMessage{
			EventType: binary.BigEndian.Uint16(chunk.Body),
			EventData: chunk.Body[2:],
		}

		chunk.MsgData = handlerUserControlMessage(userData)
	}

}

func handlerUserControlMessage(controlMsg UserControlMessage) interface{} {

	switch controlMsg.EventType {
	case RTMP_USER_STREAM_BEGIN:
		m := &StreamIDMessage{
			UserControlMessage: controlMsg,
			StreamID:           0,
		}
		if len(controlMsg.EventData) > 4 {
			m.StreamID = binary.BigEndian.Uint32(controlMsg.EventData)
		}
		return m
	case RTMP_USER_STREAM_EOF, RTMP_USER_STREAM_DRY, RTMP_USER_STREAM_IS_RECORDED:
		// 服务端向客户端发送本事件，通知数据回放完成，
		// 若没有额外的命令，就不在发送数据，客户端丢弃从流中接收的消息
		// 4字节的事件ID，表示回放结束的流的ID
		return &StreamIDMessage{
			UserControlMessage: controlMsg,
			StreamID:           binary.BigEndian.Uint32(controlMsg.EventData),
		}
	case RTMP_USER_SET_BUFFLEN:
		// 客户端向服务端发送该事件告知 自己存储了当前流的数据的缓存长度(毫秒单位).\
		// 当服务端开始处理的一个流的时候发送本事件，事件的头4byte表示流ID， 后四个表示缓存长度(单位毫秒)
		return &SetBufferMessage{
			StreamIDMessage: StreamIDMessage{
				UserControlMessage: controlMsg,
				StreamID:           binary.BigEndian.Uint32(controlMsg.EventData),
			},
			Millisecond: binary.BigEndian.Uint32(controlMsg.EventData[4:]),
		}
	case RTMP_USER_PING_REPONSE, RTMP_USER_EMPTY:
		//
		return &controlMsg
	default:
		return &controlMsg
	}

}
