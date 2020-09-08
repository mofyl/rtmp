package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"rtmp/mem_pool"
)

const (
	// AMF0 编码中 自定义的类型
	AMF0Number      = 0x00
	AMF0Boolean     = 0x01
	AMF0String      = 0x02
	AMF0Object      = 0x03
	AMF0MovieClip   = 0x04
	AMF0Null        = 0x05
	AMF0Undefined   = 0x06
	AMF0Reference   = 0x07
	AMF0MixedArray  = 0x08
	AMF0EndObject   = 0x09
	AMF0Array       = 0x0a
	AMF0Date        = 0x0b
	AMF0LongString  = 0x0c
	AMF0UnSupported = 0x0d
	AMF0ReCordset   = 0x0e
	AMF0Xml         = 0x0f
)

type AMFObject interface{}

type AMFObjects map[string]AMFObject

func newAMFObjects() AMFObjects {
	return make(AMFObjects, 0)
}

type AMF struct {
	*bytes.Buffer
}

func NewAMF(b []byte) *AMF {
	return &AMF{
		bytes.NewBuffer(b),
	}
}

func (amf *AMF) readString() (string, error) {
	// 第一个byte为类型，由于这里确定是String就可以跳过
	// 后2两个byte为字符的长度，是一个uint16
	// 然后就是N个byte 表示字符内容
	_, err := amf.ReadByte()
	if err != nil {
		return "", err
	}

	size, err := amf.readSize16()
	if err != nil {
		return "", err
	}
	b, cancel, err := readBytes(amf.Buffer, size)
	defer cancel()
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (amf *AMF) readObjectKey() (string, error) {
	size, err := amf.readSize16()
	if err != nil {
		return "", err
	}
	b, cancel, err := readBytes(amf.Buffer, size)
	defer cancel()
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (amf *AMF) readSize16() (int, error) {
	b, cancel, err := readBytes(amf.Buffer, 2)
	defer cancel()
	if err != nil {
		return 0, err
	}
	return int(binary.BigEndian.Uint16(b)), nil
}

func (amf *AMF) readSize32() (int, error) {
	b, cancel, err := readBytes(amf.Buffer, 4)
	defer cancel()
	if err != nil {
		return 0, nil
	}
	return int(binary.BigEndian.Uint32(b)), nil
}

func (amf *AMF) readBool() (bool, error) {

	_, err := amf.ReadByte()
	if err != nil {
		return false, err
	}

	b, err := amf.ReadByte()
	if err != nil {
		return false, err
	}

	return b == 1, nil

}

func (amf *AMF) readObject() (AMFObjects, error) {
	if amf.Len() == 0 {
		return nil, nil
	}
	// 跳过类型
	_, err := amf.ReadByte()
	if err != nil {
		return nil, err
	}

	m := newAMFObjects()
	k := ""
	var v AMFObject
	for {
		// 读取一个Key
		k, err = amf.readObjectKey()
		if err != nil {
			return nil, err
		}
		v, err = amf.decodeObject()

		if err != nil {
			return nil, err
		}

		if v == AMF0EndObject {
			return m, nil
		}
		if v != AMF0EndObject && k != "" {
			m[k] = v
		}

	}
}

func (amf *AMF) decodeObject() (AMFObject, error) {

	if amf.Len() == 0 {
		return nil, fmt.Errorf("no enough bytes %d", amf.Len())
	}

	// 解析类型 第一个byte为类型
	t, err := amf.ReadByte()
	if err != nil {
		return nil, err
	}

	// 由于解析类型时 内部会去跳过1个byte 所以这里需要将 刚读取的byte还原会去
	if err = amf.UnreadByte(); err != nil {
		return nil, err
	}
	switch t {
	case AMF0Number:
		return amf.readNumber()
	case AMF0String:
		return amf.readString()
	case AMF0Boolean:
		return amf.readBool()
	case AMF0Object:
		return amf.readObject()
	case AMF0MovieClip:
	case AMF0Null:
		return amf.readNull()
	case AMF0Undefined:
		return amf.readUndefined()
	case AMF0Reference:
	case AMF0Array:
		return amf.readArray()
	case AMF0EndObject:
		return amf.readEndObject()
	case AMF0Date:
		return amf.readDate()
	case AMF0LongString:
		// 和string不同的是 size的长度不同， string的size是2byte
		// LongString的是4byte
		return amf.readLongString()
	}

	return nil, fmt.Errorf("Not Support Type , type is %d", t)

}

// longString
func (amf *AMF) readLongString() (string, error) {

	_, err := amf.ReadByte()
	if err != nil {
		return "", err
	}

	size, err := amf.readSize32()

	b, cancel, err := readBytes(amf.Buffer, size)
	defer cancel()

	if err != nil {
		return "", err
	}

	return string(b), nil

}

// 第一个字节是 类型
// 后面8个byte 是时间 就是一个uint64类型
// 后面 2个byte是 date-marker
func (amf *AMF) readDate() (uint64, error) {
	_, err := amf.ReadByte()

	if err != nil {
		return 0, nil
	}

	b, cancel1, err := readBytes(amf.Buffer, 8)
	defer cancel1()
	if err != nil {
		return 0, nil
	}

	t := binary.BigEndian.Uint64(b)

	b, cancel2, err := readBytes(amf.Buffer, 2)
	defer cancel2()

	return t, err

}

func (amf *AMF) readEndObject() (AMFObject, error) {

	_, err := amf.ReadByte()
	if err != nil {
		return nil, err
	}

	return AMF0EndObject, nil

}

// 这里的array也是一个map 只不过可以读出来 size
// 第一个字节是类型
// 后面4个Byte就是size
func (amf *AMF) readArray() (AMFObjects, error) {

	m := newAMFObjects()

	_, err := amf.ReadByte()

	if err != nil {
		return nil, err
	}

	size, err := amf.readSize32()

	if err != nil {
		return nil, err
	}

	for i := 0; i < size; i++ {
		if k, err := amf.readString(); err == nil {
			if v, err := amf.decodeObject(); err == nil {
				if k != "" || v != AMF0EndObject {
					m[k] = v
					continue
				}
			}
		}
		return nil, err
	}
	return m, nil
}

func (amf *AMF) readUndefined() (AMFObject, error) {
	_, err := amf.ReadByte()
	return AMF0Undefined, err
}

func (amf *AMF) readNull() (AMFObject, error) {
	_, err := amf.ReadByte()
	return nil, err
}

func readBytes(buf *bytes.Buffer, length int) ([]byte, func(), error) {

	b := mem_pool.GetSlice(length)
	f := func() {
		mem_pool.RecycleSlice(b)
	}
	i, err := buf.Read(b)
	if err != nil {
		return nil, f, err
	}

	if i != length {
		return nil, f, fmt.Errorf("not enough bytes,%v/%v", buf.Len(), length)
	}

	return b, f, nil
}

func (amf *AMF) readNumber() (float64, error) {

	_, err := amf.ReadByte()

	if err != nil {
		return 0, err
	}
	var num float64
	err = binary.Read(amf.Buffer, binary.BigEndian, &num)

	return num, err
}

func DecodeAMFObject(obj interface{}) AMFObjects {
	if v, ok := obj.(AMFObjects); ok {
		return v
	}
	return nil
}
