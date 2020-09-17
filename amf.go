package main

import (
	"bytes"
	"encoding/binary"
	"rtmp/mem_pool"
	"rtmp/utils"

	"github.com/pkg/errors"
)

const (
	// AMF0 编码中 自定义的类型.
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

var endObj = []byte{0, 0, AMF0EndObject}

type AMFObject interface{}

type AMFObjects map[string]AMFObject

func newAMFObjects() AMFObjects {
	return make(AMFObjects)
}

type AMF struct {
	*bytes.Buffer
}

func NewAMFEncode() *AMF {
	return &AMF{
		&bytes.Buffer{},
	}
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

func (amf *AMF) writeString(v string) error {
	// 先将类型写进去
	err := amf.WriteByte(byte(AMF0String))
	if err != nil {
		return err
	}
	vB := []byte(v)
	// 将长度写进去
	if err = amf.writeSize16(uint16(len(vB))); err != nil {
		return err
	}

	return binary.Write(amf, binary.BigEndian, vB)
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

func (amf *AMF) writeObjectKey(key string) error {
	keyB := []byte(key)
	if err := amf.writeSize16(uint16(len(keyB))); err != nil {
		return err
	}

	if err := binary.Write(amf, binary.BigEndian, keyB); err != nil {
		return err
	}

	return nil
}

func (amf *AMF) readSize16() (int, error) {
	b, cancel, err := readBytes(amf.Buffer, 2)
	defer cancel()
	if err != nil {
		return 0, err
	}

	return int(binary.BigEndian.Uint16(b)), nil
}

func (amf *AMF) writeSize16(l uint16) error {
	b := mem_pool.GetSlice(2)
	defer mem_pool.RecycleSlice(b)

	binary.BigEndian.PutUint16(b, l)
	_, err := amf.Write(b)

	return err
}

func (amf *AMF) readSize32() (int, error) {
	b, cancel, err := readBytes(amf.Buffer, 4)
	defer cancel()
	if err != nil {
		return 0, err
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

func (amf *AMF) writeBool(b bool) error {
	// 写入类型
	if err := amf.WriteByte(AMF0Boolean); err != nil {
		return err
	}
	// 写入值
	if b {
		return amf.WriteByte(byte(1))
	}

	return amf.WriteByte(byte(0))
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
		return nil, errors.Errorf("no enough bytes %d", amf.Len())
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

	return nil, errors.Errorf("Not Support Type , type is %d", t)
}

func (amf *AMF) encodeObject(t AMFObjects) error {
	// 写入类型名字
	amf.WriteByte(AMF0Object)

	// 将类型写入
	for k, v := range t {
		// 写一个key 写一个值
		switch vv := v.(type) {
		case string:
			if err := amf.writeObjectString(k, vv); err != nil {
				return err
			}
		case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			if err := amf.writeObjectNumer(k, utils.ToFloat64(vv)); err != nil {
				return err
			}
		case bool:
			if err := amf.writeObjectBool(k, vv); err != nil {
				return err
			}
		}
	}

	// 最后写入结束
	if err := amf.writeObjectEnd(); err != nil {
		return err
	}

	return nil
}

func (amf *AMF) writeObjectEnd() error {
	_, err := amf.Write(endObj)

	return err
}

func (amf *AMF) writeObjectString(key, value string) error {
	if err := amf.writeObjectKey(key); err != nil {
		return err
	}

	return amf.writeString(value)
}

func (amf *AMF) writeObjectNumer(key string, v float64) error {
	if err := amf.writeObjectKey(key); err != nil {
		return err
	}

	return amf.writeNumber(v)
}

func (amf *AMF) writeObjectBool(key string, v bool) error {
	if err := amf.writeObjectKey(key); err != nil {
		return err
	}

	return amf.writeBool(v)
}

// longString.
func (amf *AMF) readLongString() (string, error) {
	_, err := amf.ReadByte()
	if err != nil {
		return "", err
	}

	size, err := amf.readSize32()
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

// 第一个字节是 类型
// 后面8个byte 是时间 就是一个uint64类型
// 后面 2个byte是 date-marker.
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

	_, cancel2, err := readBytes(amf.Buffer, 2)
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
// 后面4个Byte就是size.
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
		return nil, f, errors.Errorf("not enough bytes,%v/%v", buf.Len(), length)
	}

	return b, f, nil
}

func (amf *AMF) readNumber() (float64, error) {
	// 这里的err不能检查 因为ReadByte总是会读取8个byte
	_, err := amf.ReadByte()
	if err != nil {
		return 0, err
	}
	var num float64
	err = binary.Read(amf.Buffer, binary.BigEndian, &num)

	return num, err
}

func (amf *AMF) writeNumber(l float64) error {
	// 写入类型
	if err := amf.WriteByte(byte(AMF0Number)); err != nil {
		return err
	}

	return binary.Write(amf, binary.BigEndian, l)
}

func DecodeAMFObject(obj interface{}) AMFObjects {
	if v, ok := obj.(AMFObjects); ok {
		return v
	}

	return nil
}
