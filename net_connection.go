package main

import (
	"bufio"
	"encoding/binary"
	"fmt"

	// "fmt"
	"net"
	"rtmp/mem_pool"
	"rtmp/utils"
	"sync/atomic"

	"github.com/pkg/errors"
)

const (
	SendAckWindowSizeMessage    = "Send Window Acknowledgement Size Message"
	SendSetPeerBandWidthMessage = "Send Set Peer Bandwidth Message"
	SendStreamBeginMessage      = "Send Stream Begin Message"
	SendConnectResponseMessage  = "Send Connect Response Message"
)

const (
	EngineVersion = "M/1.0"
)

type NetConnection struct {
	conn           net.Conn
	rw             *bufio.ReadWriter
	writeChunkSize int
	readChunkSize  int

	streamID uint32

	// rtmp 传输的头部可能是不完全的，但是第一个一定是完整的 这个完整的我们需要记录下来
	// 然后后面再来相同ChunkID的头部，我们直接修改这个完整的就好了
	rtmpHeader map[uint32]*ChunkHeader
	// rtmp 的body可能是不完全的 因为每个chunk最大128byte 我们就需要将每个body拼接起来
	rtmpBody       map[uint32][]byte
	appName        string
	objectEncoding float64
	readSeqNum     uint32 // 已经读取到的byte数
	writeSeqNum    uint32 // 一些发送出去的byte数
	totalWrite     uint32 // 一共发送出去的byte数
	totalRead      uint32 // 一共已经读取到的byte数
}

func (nc *NetConnection) addReadSeqNum(n int) {
	atomic.AddUint32(&nc.readSeqNum, uint32(n))
}

func (nc *NetConnection) addWriteSeqNum(n int) {
	atomic.AddUint32(&nc.writeSeqNum, uint32(n))
}

func (nc *NetConnection) readByte() (b byte, err error) {
	b, err = nc.rw.ReadByte()
	nc.addReadSeqNum(1)

	return
}

func (nc *NetConnection) readFull(b []byte) (n int, err error) {
	n, err = nc.rw.Read(b)
	nc.addReadSeqNum(n)

	return
}

func (nc *NetConnection) writeByte(b byte) error {
	err := nc.rw.WriteByte(b)
	nc.addWriteSeqNum(1)

	return err
}

func (nc *NetConnection) writeFull(b []byte) (n int, err error) {
	n, err = nc.rw.Write(b)
	nc.addWriteSeqNum(n)

	return
}

func (nc *NetConnection) HandlerMessage() {
	var err error
	if err = handshake(nc.rw); err != nil {
		fmt.Println("HandShake Fail")

		return
	}
	fmt.Println("HandShake Success")
	if err = nc.onConnect(); err != nil {
		fmt.Println("Try To Connect Fail")

		return
	}
	fmt.Println("Try Connect Success")
}

func (nc *NetConnection) onConnect() (err error) {
	msg, err := nc.readChunk()
	if err != nil {
		return
	}
	defer chunkMsgPool.Put(msg)

	connect, ok := msg.MsgData.(*CallMessage)

	if ok {
		if connect.CommandName != CommandConnect {
			return nil
		}
		v := DecodeAMFObject(connect.Object)
		if v == nil {
			err = errors.Errorf("OnConnect Decode AMF Object Fail")

			return
		}
		if appName, ok := v["app"]; ok {
			nc.appName = appName.(string)
		}

		if objEncoding, ok := v["objectEncoding"]; ok {
			nc.objectEncoding = objEncoding.(float64)
		}

		// 回复消息
		_ = nc.SendMessage(SendAckWindowSizeMessage, uint32(512<<10))
		_ = nc.SendMessage(SendSetPeerBandWidthMessage, uint32(512<<10))
		_ = nc.SendMessage(SendStreamBeginMessage, nil)
		_ = nc.SendMessage(SendConnectResponseMessage, nc.objectEncoding)
		fmt.Println("OnConnect TryConnect Response is Done.")
	}

	return nil
}

func (nc *NetConnection) SendMessage(msgType string, args interface{}) error {
	switch msgType {
	case SendAckWindowSizeMessage:
		size, ok := args.(uint32)
		if !ok {
			return errors.New(SendAckWindowSizeMessage + ", The args must be a uint32")
		}

		return nc.writeMessage(RtmpMsgChunkSize, Uint32Message(size))
	case SendSetPeerBandWidthMessage:
		size, ok := args.(uint32)
		if !ok {
			return errors.New(SendSetPeerBandWidthMessage + ", The args must be a uint32")
		}

		return nc.writeMessage(RtmpMsgBandWidth, &SetPeerBandWidthMessage{
			AcknowledgementWindowSize: size,
			LimitType:                 byte(2),
		})
	case SendStreamBeginMessage:
		if args == nil {
			return errors.New(SendStreamBeginMessage + ", The paramter is nil")
		}
		// 其实这里还没有streamID 后面客户端回复 建立连接的时候会把streamID带过来
		return nc.writeMessage(RtmpMsgUserControl, &StreamIDMessage{UserControlMessage{EventType: RtmpUserStreamBegin}, nc.streamID})
	case SendConnectResponseMessage:
		pro := newAMFObjects()
		info := newAMFObjects()

		pro["fmsVer"] = EngineVersion
		pro["capabilities"] = 31
		pro["mode"] = 1
		pro["Author"] = "dexter"
		info["level"] = LevelStatus
		info["code"] = NetConnectionConnectSuccess
		info["objectEncoding"] = args.(float64)
		m := new(ResponseConnectMessage)
		m.CommandName = ResponseResult
		m.TransactionID = 1
		m.Properties = pro
		m.Infomation = info

		return nc.writeMessage(RtmpMsgAMF0Command, m)
	}

	return errors.New("SendMessage Not Support Type is " + msgType)
}

func (nc *NetConnection) readChunk() (*Chunk, error) {
	// 先读 BasicHeader
	// 先读 先读BasicHeader中的 ChunkType

	basicHeader, err := nc.readByte()

	if err != nil {
		return nil, err
	}

	streamID := uint32(basicHeader & 0x3f) //  0011 1111
	chunkType := (basicHeader & 0xc3) >> 6 // 1100 0000
	streamID, err = nc.getChunkStreamID(streamID)

	if err != nil {
		return nil, err
	}

	fullHead, ok := nc.rtmpHeader[streamID]

	if !ok {
		fullHead = &ChunkHeader{}
		fullHead.ChunkStreamID = streamID
		fullHead.ChunkType = chunkType
		nc.rtmpHeader[streamID] = fullHead
	}

	err = nc.buildChunkHeader(chunkType, fullHead)
	if err != nil {
		return nil, err
	}

	currentBody, ok := nc.rtmpBody[streamID]

	msgLen := int(fullHead.MessageLength)
	if !ok {
		currentBody = mem_pool.GetSlice(msgLen)[:0]
		nc.rtmpBody[streamID] = currentBody
	}
	// 已经读取的长度
	readed := len(currentBody)

	// 这里的ChunkSize是推流端发送过来的 可能每个包的chunkSize都不同
	needRead := nc.readChunkSize
	// 用总的长度减去 已读的，就是需要读取的
	unRead := msgLen - readed
	// 若没读取的needRead < 当前的ChunkSize 那么就不管
	if unRead < needRead {
		needRead = unRead
	}

	// if n, err := io.ReadFull(nc.rw, currentBody[readed:needRead+readed]); err != nil {
	// 	readed += n
	// }
	if n, err := nc.readFull(currentBody[readed : needRead+readed]); err != nil {
		fmt.Println("nc ReadFull Err ", err.Error())
	} else {
		readed += n
	}

	currentBody = currentBody[:readed]
	nc.rtmpBody[streamID] = currentBody

	// 若是已经读完了 就不需要在读取了
	// 这里一定要放到最后 若在前面的话 读取完 完整的包后，还会进行递归，那么就还去读数据
	// 但是此时流中已经没有数据了就会阻塞
	if readed == msgLen {
		msg := chunkMsgPool.Get().(*Chunk)

		msg.MsgData = nil
		msg.Body = currentBody
		msg.ChunkHeader = fullHead
		msg.ChunkHeader = fullHead.Clone()
		msg.MsgData, err = GetRtmpMsgData(msg)
		if err != nil {
			return nil, err
		}
		delete(nc.rtmpBody, msg.ChunkStreamID)

		return msg, nil
	}

	return nc.readChunk()
}

/*
	根据ChunkType 的不同 MsgHeader可以分为 0，1，2，3
		0时 MsgHeader为全量头部 占用11个字节 若加上BasicHeader就是 12byte
		1时 MsgHeader为部分头部 占用7byte  若加上BasicHeader就是 8byte
		2时 MsgHeader为部分头部 占用 3byte 若加上BasicHeader 就是 4byte
		3时 MsgHeader为部分头部 占用0byte 若加上BasicHeader 就是1byte

*/
func (nc *NetConnection) buildChunkHeader(chunkType byte, h *ChunkHeader) error {
	switch chunkType {
	case 0:
		return nc.chunkType0(h)
	case 1:
		return nc.chunkType1(h)
	case 2:
		return nc.chunkType2(h)
	case 3:
		h.ChunkType = chunkType

		return nil
	}

	return errors.Errorf("Not Support ChunkType type is %d", chunkType)
}

func (nc *NetConnection) chunkType0(h *ChunkHeader) error {
	// 前三个byte为 timestamp
	b := mem_pool.GetSlice(3)
	defer mem_pool.RecycleSlice(b)
	_, err := nc.readFull(b)
	if err != nil {
		return err
	}
	h.Timestamp = utils.BigEndian.Uint24(b)

	// 再3个为 Message Len
	if _, err := nc.readFull(b); err != nil {
		return err
	}
	h.MessageLength = utils.BigEndian.Uint24(b)
	// 后面一个byte是 message Type
	mb, err := nc.readByte()
	if err != nil {
		return err
	}
	h.MessageTypeID = mb
	// 再来4个是 msgStreamID 和 前面 basicHeader 中的chunkID相同 不过这里的ID是用小端来存储的
	b4 := mem_pool.GetSlice(4)
	mem_pool.RecycleSlice(b4)
	_, err = nc.readFull(b4)
	if err != nil {
		return err
	}
	h.MessageStreamID = binary.LittleEndian.Uint32(b4)

	err = nc.getExtendTimestamp(h)

	if err != nil {
		return err
	}

	return nil
}

func (nc *NetConnection) chunkType1(h *ChunkHeader) error {
	// 前3byte为timestamp 这里的timestamp是前一个包的时间差值
	b3 := mem_pool.GetSlice(3)
	mem_pool.RecycleSlice(b3)
	_, err := nc.readFull(b3)
	if err != nil {
		return err
	}
	h.Timestamp += utils.BigEndian.Uint24(b3)

	// 后3byte为messageLength
	_, err = nc.readFull(b3)
	if err != nil {
		return err
	}

	b1, err := nc.readByte()
	if err != nil {
		return err
	}

	h.MessageTypeID = b1

	err = nc.getExtendTimestamp(h)
	if err != nil {
		return err
	}

	return nil
}

func (nc *NetConnection) chunkType2(h *ChunkHeader) error {
	b3 := mem_pool.GetSlice(3)
	defer mem_pool.RecycleSlice(b3)
	_, err := nc.readFull(b3)

	if err != nil {
		return err
	}

	h.Timestamp += binary.BigEndian.Uint32(b3)

	if err = nc.getExtendTimestamp(h); err != nil {
		return err
	}

	return nil
}

func (nc *NetConnection) getExtendTimestamp(h *ChunkHeader) error {
	b4 := mem_pool.GetSlice(4)
	defer mem_pool.RecycleSlice(b4)
	// 判断是否要读取ExtendTimestamp中的值
	if h.Timestamp == 0xfffff {
		if _, err := nc.readFull(b4); err != nil {
			return err
		}
		h.ExtendTimestamp = binary.BigEndian.Uint32(b4)
	}

	return nil
}

func (nc *NetConnection) getChunkStreamID(csid uint32) (uint32, error) {
	chunkStreamID := csid

	switch csid {
	case 0:
		// 表示占用2个字节
		b1, err := nc.readByte()
		if err != nil {
			return chunkStreamID, err
		}

		chunkStreamID = 64 + uint32(b1)
	case 1:
		// 表示占用3个字节
		b1, err := nc.readByte()
		if err != nil {
			return chunkStreamID, err
		}

		b2, err := nc.readByte()
		if err != nil {
			return chunkStreamID, err
		}

		//chunkStreamID = uint32(b1) + 64 + uint32(b2) * 256
		chunkStreamID = uint32(b1) + 64 + (uint32(b2) << 8)
	}

	return chunkStreamID, nil
}

/*
	这里回包的格式和发来包的格式相同
		若是fmt=0那么 BaseicHeader也是 fmt占2bit streamID占6bit
			MessageHeader中 3byte的timestamp 3byte的MessageLen
				然后是1byte的 MessageType 最后是4byte的steamID(使用小端存储的)
				若是有ExtendTimestamp也要写到最后去
*/
func (nc *NetConnection) writeMessage(t byte, en MessageEncode) error {
	body := en.Encode()
	head := newChunkHeaderFromMessageType(t)

	head.MessageLength = uint32(len(body))

	if sid, ok := en.(HaveStreamID); ok {
		head.MessageStreamID = sid.GetStreamID()
	}

	// 这里根据ChunkType的不同 也会有不同大小的ChunkHead
	// 分为 12 8 4 1 这些数字指的都是ChunkHeader的大小
	// 开始我们先使用12byte 的全量包发，若发不完就需要分包，这时就需要使用 1byte的ChunkHeader发送直到发完
	// 因为我们的ChunkHeader和上一个包都相同，所以使用的就是1byte的Header包
	need, err := nc.encodeChunk12(head, body, nc.writeChunkSize)

	if err != nil {
		return err
	}

	if err := nc.rw.Flush(); err != nil {
		return err
	}

	for need != nil {
		if need, err = nc.encodeChunk1(head, need, nc.writeChunkSize); err != nil {
			return err
		}

		if err = nc.rw.Flush(); err != nil {
			return err
		}
	}

	return nil
}
