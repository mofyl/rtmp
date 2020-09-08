package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"rtmp/mem_pool"
	"rtmp/utils"
)

const (
	SendAckWindowSizeMessage = "Send Window Acknowledgement Size Message"
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
	readSeqNum     uint32
	writeSeqNum    uint32
}

func (conn *NetConnection) addReadSeqNum(n int) {
	conn.readSeqNum += uint32(n)
}

func (conn *NetConnection) addWriteSeqNum(n int) {
	conn.writeSeqNum += uint32(n)
}

func (conn *NetConnection) readByte() (b byte, err error) {
	b, err = conn.rw.ReadByte()
	conn.addReadSeqNum(1)
	return
}

func (conn *NetConnection) readFull(b []byte) (n int, err error) {
	n, err = conn.rw.Read(b)
	conn.addReadSeqNum(n)
	return
}

func (conn *NetConnection) OnConnect() (err error) {

	if msg, err := conn.readChunk(); err == nil {
		defer chunkMsgPool.Put(msg)
		if connect, ok := msg.MsgData.(*CallMessage); ok {
			if connect.CommandName == CommandConnect {
				v := DecodeAMFObject(connect.Object)
				if v == nil {
					err = fmt.Errorf("OnConnect Decode AMF Object Fail")
				}
				if appName, ok := v["app"]; ok {
					conn.appName = appName.(string)
				}

				if objEncoding, ok := v["objectEncoding"]; ok {
					conn.objectEncoding = objEncoding.(float64)
				}

			}
		}
	}
	return
}

// func (conn *NetConnection) SendMessage(message string, args interface{}) error {

// }

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

	currentBody, ok := conn.rtmpBody[streamID]

	//
	//if chunkType != 3 && !ok {
	//
	//}
	//
	msgLen := int(fullHead.MessageLength)
	if !ok {
		currentBody = mem_pool.GetSlice(msgLen)[:0]
		conn.rtmpBody[streamID] = currentBody
	}
	// 已经读取的长度
	readed := len(currentBody)

	// 这里的ChunkSize是推流端发送过来的 可能每个包的chunkSize都不同
	needRead := conn.readChunkSize
	// 用总的长度减去 已读的，就是需要读取的
	unRead := msgLen - readed
	// 若没读取的needRead < 当前的ChunkSize 那么就不管
	if unRead < needRead {
		needRead = unRead
	}

	// if n, err := io.ReadFull(conn.rw, currentBody[readed:needRead+readed]); err != nil {
	// 	readed += n
	// }
	if n, err := conn.readFull(currentBody[readed : needRead+readed]); err != nil {
		fmt.Println("conn ReadFull Err ", err.Error())
	} else {
		readed += n
	}

	currentBody = currentBody[:readed]
	conn.rtmpBody[streamID] = currentBody

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
		delete(conn.rtmpBody, msg.ChunkStreamID)
		return msg, nil
	}

	return conn.readChunk()
}

/*
	根据ChunkType 的不同 MsgHeader可以分为 0，1，2，3
		0时 MsgHeader为全量头部 占用11个字节
		1时 MsgHeader为部分头部 占用7byte
		2时 MsgHeader为部分头部 占用 3byte
		3时 MsgHeader为部分头部 占用0byte

*/
func (conn *NetConnection) buildChunkHeader(chunkType byte, h *ChunkHeader) error {

	switch chunkType {
	case 0:
		return conn.chunkType0(h)
	case 1:
		return conn.chunkType1(h)
	case 2:
		return conn.chunkType2(h)
	case 3:
		h.ChunkType = chunkType
		return nil
	}
	return fmt.Errorf("Not Support ChunkType type is %d", chunkType)
}

func (conn *NetConnection) chunkType0(h *ChunkHeader) error {

	// 前三个byte为 timestamp
	b := mem_pool.GetSlice(3)
	defer mem_pool.RecycleSlice(b)
	_, err := conn.readFull(b)
	if err != nil {
		return err
	}
	h.Timestamp = utils.BigEndian.Uint24(b)

	// 再3个为 Message Len
	if _, err := conn.readFull(b); err != nil {
		return err
	}
	h.MessageLength = utils.BigEndian.Uint24(b)
	// 后面一个byte是 message Type
	mb, err := conn.readByte()
	if err != nil {
		return err
	}
	h.MessageTypeID = mb
	// 再来4个是 msgStreamID 和 前面 basicHeader 中的chunkID相同 不过这里的ID是用小端来存储的
	b4 := mem_pool.GetSlice(4)
	mem_pool.RecycleSlice(b4)
	_, err = conn.readFull(b4)
	if err != nil {
		return err
	}
	h.MessageStreamID = binary.LittleEndian.Uint32(b4)

	err = conn.getExtendTimestamp(h)

	if err != nil {
		return err
	}

	return nil
}

func (conn *NetConnection) chunkType1(h *ChunkHeader) error {
	// 前3byte为timestamp 这里的timestamp是前一个包的时间差值
	b3 := mem_pool.GetSlice(3)
	mem_pool.RecycleSlice(b3)
	_, err := conn.readFull(b3)
	if err != nil {
		return err
	}
	h.Timestamp += utils.BigEndian.Uint24(b3)

	// 后3byte为messageLength
	_, err = conn.readFull(b3)
	if err != nil {
		return err
	}

	b1, err := conn.readByte()
	if err != nil {
		return err
	}

	h.MessageTypeID = b1

	err = conn.getExtendTimestamp(h)
	if err != nil {
		return err
	}

	return nil
}

func (conn *NetConnection) chunkType2(h *ChunkHeader) error {

	b3 := mem_pool.GetSlice(3)
	defer mem_pool.RecycleSlice(b3)
	_, err := conn.readFull(b3)

	if err != nil {
		return err
	}

	h.Timestamp += binary.BigEndian.Uint32(b3)

	if err = conn.getExtendTimestamp(h); err != nil {
		return err
	}

	return nil
}

func (conn *NetConnection) getExtendTimestamp(h *ChunkHeader) error {
	b4 := mem_pool.GetSlice(4)
	defer mem_pool.RecycleSlice(b4)
	// 判断是否要读取ExtendTimestamp中的值
	if h.Timestamp == 0xfffff {
		if _, err := conn.readFull(b4); err != nil {
			return err
		}
		h.ExtendTimestamp = binary.BigEndian.Uint32(b4)
	}
	return nil
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
