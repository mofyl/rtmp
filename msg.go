package main

import (
	"encoding/binary"
	"sync"

	"github.com/pkg/errors"
)

const (
	// RtmpDefaultChunkSize = 128
	RtmpDefaultChunkSize = 128
	RtmpMaxChunkSize     = 65536
	// Chunk
	RtmpMsgChunkSize = 1
	RtmpMsgAbort     = 2

	// RTMP
	RtmpMsgAck         = 3
	RtmpMsgUserControl = 4
	RtmpMsgAckSize     = 5
	RtmpMsgBandWidth   = 6
	RtmpMsgEdge        = 7
	RtmpMsgAudio       = 8
	RtmpMsgVideo       = 9

	// User Control Event
	// 服务端向客户端发送本事件通知对方 一个流开始起作用并可以用于通讯，
	// 默认情况下 服务器成功的从客户端接收连接命令后发送本事件 streamID=0，事件数据为开始起作用的流ID
	RtmpUserStreamBegin    = 0
	RtmpUserStreamEOF      = 1
	RtmpUserStreamDay      = 2
	RtmpUserSetBufferLen   = 3
	RtmpUserStreamRecorded = 4
	RtmpUserPingRequest    = 6
	RtmpUserPingResponse   = 7

	// 命令消息
	RtmpMsgAMF3Command = 17
	RtmpMsgAMF0Command = 20

	RtmpUserEmpty = 31

	CommandConnect       = "connect"
	CommandCall          = "call"
	CommandCreateStream  = "createStream"
	CommandPlay          = "play"
	CommandPlay2         = "play2"
	CommandPublish       = "publish"
	CommandPause         = "pause"
	CommandSeek          = "seek"
	CommandDeleteStream  = "deleteStream"
	CommandCloseStream   = "closeStream"
	CommandReleaseStream = "releaseStream"
	CommandReceiveAudio  = "receiveAudio"
	CommandReceiveVideo  = "receiveVideo"
	CommandResult        = "_result"
	CommandError         = "_error"
	CommandOnStatus      = "onStatus"
	CommandFCPublish     = "FCPublish"
	CommandFcUnpublish   = "FCUnpublish"

	RtmpCSIDControl = 0x02
	RtmpCSIDCommand = 0x03
	RtmpCSIDAudio   = 0x06
	RtmpCSIDData    = 0x05
	RtmpCSIDVideo   = 0x05
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

type MessageEncode interface {
	Encode() []byte
}

type HaveStreamID interface {
	GetStreamID() uint32
}

type UserControlMessage struct {
	EventType uint16
	EventData []byte
}

func (msg *UserControlMessage) Encode() []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, msg.EventType)
	msg.EventData = b[2:]

	return b
}

type StreamIDMessage struct {
	UserControlMessage
	StreamID uint32
}

func (msg *StreamIDMessage) Encode() []byte {
	b := make([]byte, 6)
	binary.BigEndian.PutUint16(b, msg.EventType)
	binary.BigEndian.PutUint32(b[2:], msg.StreamID)
	msg.EventData = b[2:]

	return b
}

type SetBufferMessage struct {
	StreamIDMessage
	Millisecond uint32
}

type SetPeerBandWidthMessage struct {
	AcknowledgementWindowSize uint32 // 4 byte
	LimitType                 byte
}

func (msg *SetPeerBandWidthMessage) Encode() []byte {
	b := make([]byte, 5)
	binary.BigEndian.PutUint32(b, msg.AcknowledgementWindowSize)
	b[4] = msg.LimitType

	return b
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

type CommandMessage struct {
	CommandName   string
	TransactionID uint64
}

type CallMessage struct {
	CommandMessage
	Object   interface{} // 这里是一个 Object
	Optional interface{} // 这里是一个Object
}

type CreateStreamMessage struct {
	CommandMessage
	cmdMsg AMFObject
}

type PlayMessage struct {
	CommandMessage
	StreamName string
	Start      uint64
	Duration   uint64
	Reset      bool
}

type Uint32Message uint32

func (msg Uint32Message) Encode() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(msg))

	return b
}

type ResponseConnectMessage struct {
	CommandMessage
	Properties AMFObjects `json:",omitempty"`
	Infomation AMFObjects `json:",omitempty"`
}

func (msg *ResponseConnectMessage) Encode() []byte {
	amf := NewAMFEncode()

	_ = amf.writeString(msg.CommandName)
	_ = amf.writeNumber(float64(msg.TransactionID))

	if msg.Properties != nil {
		_ = amf.encodeObject(msg.Properties)
	}

	if msg.Infomation != nil {
		_ = amf.encodeObject(msg.Infomation)
	}

	return amf.Bytes()
}

func newChunkHeaderFromMessageType(msgType byte) *ChunkHeader {
	head := &ChunkHeader{}

	head.ChunkStreamID = RtmpCSIDControl

	if msgType == RtmpMsgAMF0Command {
		head.ChunkStreamID = RtmpCSIDCommand
	}

	head.Timestamp = 0
	head.MessageTypeID = msgType
	head.MessageStreamID = 0
	head.ExtendTimestamp = 0

	return head
}

func GetRtmpMsgData(chunk *Chunk) (interface{}, error) {
	switch chunk.MessageTypeID {
	// 这里由于 1,2,3,5的body都是 4byte 所以可以一起解析
	case RtmpMsgChunkSize, RtmpMsgAbort, RtmpMsgAck, RtmpMsgAckSize:
		return binary.BigEndian.Uint32(chunk.Body), nil
	case RtmpMsgUserControl:
		// 这里分为 EventType 和EventData
		// 前2byte是 EventType 后面的就是EventData
		userData := UserControlMessage{
			EventType: binary.BigEndian.Uint16(chunk.Body),
			EventData: chunk.Body[2:],
		}

		return handlerUserControlMessage(userData), nil
	case RtmpMsgBandWidth:
		// 表示设置对端的 带宽
		m := &SetPeerBandWidthMessage{
			AcknowledgementWindowSize: binary.BigEndian.Uint32(chunk.Body),
		}
		if len(chunk.Body) > 4 {
			m.LimitType = chunk.Body[4]
		}

		return m, nil
	case RtmpMsgAudio:
	case RtmpMsgVideo:
	case RtmpMsgAMF3Command:
		// 这里表示 使用AMF3编码的
		return deCodeCommandAMF3(chunk)
	case RtmpMsgAMF0Command:
		// 这里表示 使用AMF0编码的
		return decodeCommandAMF0(chunk)
	}

	return nil, errors.Errorf("Not Support ChunkType type is %d", chunk.ChunkType)
}

func deCodeCommandAMF3(chunk *Chunk) (interface{}, error) {
	/*
		AMF3 开头是一个无用的0字节，然后才是CommandName
		此时的RTMPBody为
			CommandName: 命令名字 是一个字符串
			TransactionID 事务ID 数字
			Command Obj 键值对集合
			Optional User Arguments 用户自定义的额外信息
			End of ObjectMarker (0x00 0x00 0x09) object的结尾

		这里AMF3自己定义了数据类型的解析方式

	*/
	chunk.Body = chunk.Body[1:]
	var obj interface{}
	var err error
	if obj, err = decodeCommandAMF0(chunk); err != nil {
		return nil, err
	}

	return obj, nil
}

func decodeCommandAMF0(chunk *Chunk) (interface{}, error) {
	amf := NewAMF(chunk.Body)

	cmdName, err := amf.readString()

	if err != nil {
		return nil, err
	}

	transactionID, err := amf.readNumber()
	if err != nil {
		return nil, err
	}
	cmdMsg := CommandMessage{
		CommandName:   cmdName,
		TransactionID: uint64(transactionID),
	}

	return handlerCommand(cmdMsg, amf, chunk)
}

func handlerCommand(cmd CommandMessage, amf *AMF, chunk *Chunk) (interface{}, error) {
	switch cmd.CommandName {
	case CommandConnect, CommandCall:
		pro, err := amf.readObject()
		if err != nil {
			return nil, err
		}
		info, err := amf.readObject()
		if err != nil {
			return nil, err
		}

		return &CallMessage{
			CommandMessage: cmd,
			Object:         pro,
			Optional:       info,
		}, nil
	case CommandCreateStream:
		// 这里要读取一个null 是因为COMMAND_CREATESTREAM这个协议在CommandMsg后
		// 后一个null
		_, _ = amf.readNull()
		msgData := &CreateStreamMessage{
			CommandMessage: cmd,
		}
		if obj, err := amf.readObject(); err != nil {
			return nil, err
		} else if err == nil {
			msgData.cmdMsg = obj
		}

		return msgData, nil
	case CommandPlay:
		_, _ = amf.readNull()
		msgData := &PlayMessage{
			CommandMessage: cmd,
		}

		if streamName, err := amf.readString(); err != nil {
			return nil, err
		} else if err == nil {
			msgData.StreamName = streamName
		}

		if start, err := amf.readNumber(); err != nil {
			return nil, err
		} else if err == nil {
			msgData.Start = uint64(start)
		}

		if duration, err := amf.readNumber(); err != nil {
			return nil, err
		} else if err == nil {
			msgData.Duration = uint64(duration)
		}

		if reset, err := amf.readBool(); err != nil {
			return nil, err
		} else if err == nil {
			msgData.Reset = reset
		}

		return msgData, nil
	}

	return nil, errors.Errorf("not Support CommandName name is %s", cmd.CommandName)
}

func handlerUserControlMessage(controlMsg UserControlMessage) interface{} {
	switch controlMsg.EventType {
	case RtmpUserStreamBegin:
		m := &StreamIDMessage{
			UserControlMessage: controlMsg,
			StreamID:           0,
		}
		if len(controlMsg.EventData) > 4 {
			m.StreamID = binary.BigEndian.Uint32(controlMsg.EventData)
		}

		return m
	case RtmpUserStreamEOF, RtmpUserStreamDay, RtmpUserStreamRecorded:
		// 服务端向客户端发送本事件，通知数据回放完成，
		// 若没有额外的命令，就不在发送数据，客户端丢弃从流中接收的消息
		// 4字节的事件ID，表示回放结束的流的ID
		return &StreamIDMessage{
			UserControlMessage: controlMsg,
			StreamID:           binary.BigEndian.Uint32(controlMsg.EventData),
		}
	case RtmpUserSetBufferLen:
		// 客户端向服务端发送该事件告知 自己存储了当前流的数据的缓存长度(毫秒单位).\
		// 当服务端开始处理的一个流的时候发送本事件，事件的头4byte表示流ID， 后四个表示缓存长度(单位毫秒)
		return &SetBufferMessage{
			StreamIDMessage: StreamIDMessage{
				UserControlMessage: controlMsg,
				StreamID:           binary.BigEndian.Uint32(controlMsg.EventData),
			},
			Millisecond: binary.BigEndian.Uint32(controlMsg.EventData[4:]),
		}
	case RtmpUserPingResponse, RtmpUserEmpty:
		//
		return &controlMsg
	default:
		return &controlMsg
	}

}
