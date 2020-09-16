package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
)

const (
	StmpHandShakVersion = 0x3 // 服务器默认支持 版本号为3的协议
	HandshakDataLen     = 1536

	C0C1Len = 1 + HandshakDataLen

	C1S1TimeSize    = 4
	C1S1VersionSize = 4

	C1S1DigestSize       = 764
	C1S1DigestDataSize   = 32
	C1S1DigestOffsetSize = 4
	C1S1DigestOffsetMax  = 764 - C1S1DigestDataSize - C1S1DigestOffsetSize

	C1S1KeySize       = 764
	C1S1KeyOffsetSize = 4
	C1S1KeyDataSize   = 128
	C1S1KeyOffsetMax  = 764 - C1S1KeyDataSize - C1S1KeyOffsetSize
)

var (
	FMSKey = []byte{
		0x47, 0x65, 0x6e, 0x75, 0x69, 0x6e, 0x65, 0x20,
		0x41, 0x64, 0x6f, 0x62, 0x65, 0x20, 0x46, 0x6c,
		0x61, 0x73, 0x68, 0x20, 0x4d, 0x65, 0x64, 0x69,
		0x61, 0x20, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72,
		0x20, 0x30, 0x30, 0x31, // Genuine Adobe Flash Media Server 001
		0xf0, 0xee, 0xc2, 0x4a, 0x80, 0x68, 0xbe, 0xe8,
		0x2e, 0x00, 0xd0, 0xd1, 0x02, 0x9e, 0x7e, 0x57,
		0x6e, 0xec, 0x5d, 0x2d, 0x29, 0x80, 0x6f, 0xab,
		0x93, 0xb8, 0xe6, 0x36, 0xcf, 0xeb, 0x31, 0xae,
	} // 68
	FPKey = []byte{
		0x47, 0x65, 0x6E, 0x75, 0x69, 0x6E, 0x65, 0x20,
		0x41, 0x64, 0x6F, 0x62, 0x65, 0x20, 0x46, 0x6C,
		0x61, 0x73, 0x68, 0x20, 0x50, 0x6C, 0x61, 0x79,
		0x65, 0x72, 0x20, 0x30, 0x30, 0x31, /* Genuine Adobe Flash Player 001 */
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8,
		0x2E, 0x00, 0xD0, 0xD1, 0x02, 0x9E, 0x7E, 0x57,
		0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	} // 62
)

func handshake(conn *bufio.ReadWriter) error {
	// 首先读取C0C1 ；一般会将C0C1放到一起发送
	c0c1, err := ReadByteToBuf(conn, C0C1Len)
	// c0c1 := make([]byte , C0C1Len) // C0 1个字节 C1 1537个字节
	// n , err := conn.Read(c0c1)
	if err != nil {
		return err
	}

	if c0c1[0] != StmpHandShakVersion {
		fmt.Println("The Client Version is not support, client ver is ", c0c1[0])
		return errors.New("The Client Version is not support ")
	}
	// 判断C1
	if len(c0c1[1:]) != HandshakDataLen {
		return errors.New("S1 illegal")
	}
	// 获取c1
	c1 := make([]byte, HandshakDataLen)
	copy(c1, c0c1[1:])
	/*
		C1构成(左闭右闭)： 0~3为时间戳  4字节
						4~8为协议标识 4字节  若为0 则表示使用 simple_handshake
								否则为  complex_handshake
						剩下的1528字节为 随机数据
	*/
	// 这里 & 0xff 主要是为了防止c1[4] 太大 超过1byte，&0xff就可以将未超过的部分保留下来,超过的部分截断为0
	if c1[4]&0xff == 0 {
		return simpleHandshake(conn, c1)
	}

	return complexHandshake(conn, c1)
}

func simpleHandshake(conn *bufio.ReadWriter, c1 []byte) error {
	/*

		s1 共占 1536byte
			前四个字节为时间 可以直接给0
	*/
	s1 := make([]byte, HandshakDataLen-4)
	s2 := c1
	buf := &bytes.Buffer{}

	buf.WriteByte(StmpHandShakVersion)
	err := binary.Write(buf, binary.BigEndian, uint32(0))
	if err != nil {
		return err
	}
	buf.Write(s1)
	buf.Write(s2)

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return err
	}
	conn.Flush()
	// 跳过C2
	_, err = ReadByteToBuf(conn, HandshakDataLen)

	return err
}

func complexHandshake(conn *bufio.ReadWriter, c1 []byte) error {
	// 校验本次C1是否合法
	_, digestData, ok, err := validateClient(c1)
	if err != nil {
		return err
	}

	if !ok {
		return errors.New("ValiDataClient Failed")
	}

	// 构造s1
	s1 := createS1()

	s1DigestOffset := getDigestDataOffset(s1)
	s1DigestPart1 := s1[:s1DigestOffset]
	s1DigestPart2 := s1[s1DigestOffset+C1S1DigestDataSize:]

	s1Buf := &bytes.Buffer{}
	s1Buf.Write(s1DigestPart1)
	s1Buf.Write(s1DigestPart2)
	s1Part1Part2 := s1Buf.Bytes()

	s1Hash, err := HMACSha256(s1Part1Part2, FMSKey[:36])
	if err != nil {
		return err
	}

	copy(s1[s1DigestOffset:], s1Hash)

	// 构造S2
	s2Random := createS2()

	s2Hash, err := HMACSha256(digestData, FMSKey[:68])
	if err != nil {
		return err
	}

	// s2 digest
	s2Digest, err := HMACSha256(s2Random, s2Hash)
	if err != nil {
		return err
	}

	totalBuf := &bytes.Buffer{}
	totalBuf.WriteByte(StmpHandShakVersion)
	totalBuf.Write(s1)
	totalBuf.Write(s2Random)
	totalBuf.Write(s2Digest)

	_, err = conn.Write(totalBuf.Bytes())
	if err != nil {
		return err
	}
	conn.Flush()
	// 跳过C2
	_, err = ReadByteToBuf(conn, HandshakDataLen)
	fmt.Println("complexHandshake Finish")

	return err
}

func createS1() []byte {
	//
	//s1Time := make([]byte , 4)
	//binary.BigEndian.PutUint64(s1Time , uint64(time.Now().Unix()))
	//s1Version := make([]byte , 4)
	//binary.BigEndian.PutUint64(s1Version , StmpHandShakVersion)
	s1Time := []byte{0, 0, 0, 0}
	s1Version := []byte{1, 1, 1, 1}
	digestLen := HandshakDataLen - C1S1TimeSize - C1S1VersionSize
	s1KeyDigest := make([]byte, digestLen)

	for i := 0; i < digestLen; i++ {
		s1KeyDigest[i] = byte(rand.Int() % 256)
	}

	buf := &bytes.Buffer{}

	buf.Write(s1Time)
	buf.Write(s1Version)
	buf.Write(s1KeyDigest)

	return buf.Bytes()

}

func createS2() []byte {
	s2Random := make([]byte, HandshakDataLen-C1S1DigestDataSize)

	for i := 0; i < len(s2Random); i++ {
		s2Random[i] = byte(rand.Int() % 256)
	}

	buf := &bytes.Buffer{}
	buf.Write(s2Random)

	return buf.Bytes()
}

// 分别返回 keyData , digestData , 是否校验digestData成功，错误.
func validateClient(c1 []byte) ([]byte, []byte, bool, error) {
	digestDataOffset := getDigestDataOffset(c1)
	keyDataOffset := getKeyOffset(c1)

	// 校验digest
	digestData := c1[digestDataOffset : digestDataOffset+C1S1DigestDataSize]
	digestPart1 := c1[:digestDataOffset]
	digestPart2 := c1[digestDataOffset+C1S1DigestDataSize:]

	buf := &bytes.Buffer{}
	buf.Write(digestPart1)
	buf.Write(digestPart2)

	c1Part1Part2 := buf.Bytes()

	tmpHash, err := HMACSha256(c1Part1Part2, FPKey[:30])
	ok := false
	if err != nil {
		return nil, nil, ok, err
	}

	if bytes.Equal(digestData, tmpHash) {
		ok = true
	}

	keyData := c1[keyDataOffset : keyDataOffset+C1S1KeyDataSize]

	return keyData, digestData, ok, nil
}

// HMACSha256 利用HASH算法 以一个秘钥和一个massage 为输入，生成一个摘要 返回
func HMACSha256(message []byte, key []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, key)

	_, err := mac.Write(message)
	if err != nil {
		return nil, err
	}

	return mac.Sum(nil), nil
}

func getDigestDataOffset(c1 []byte) int {
	/*
		c1构成 共1536个字节：
			time(4byte) + version(4byte) + digest(764byte) + key(764byte)
			time + version + [offset(4byte) + random(offset byte) + digestData(32byte) + randomData(764-4-32-offset)] + key
												// 这里需要计算offset是多少 然后才知道 random的值是多少
		所以  0 <= offset <= 728 (764-4-32)
		若 offset=3 那么 digest[7(4+3)~39(32+7)] (这里是左闭右开) 为digest-data , 若 offset=728 那么digest[732~764] 为digest-data
	*/
	// 这里 &0xff 主要是为了防止 c1[8]等太大，超过1byte，未超过的部分会保留下来,超过的部分会被截断置为0
	offset := int(c1[8]&0xff) + int(c1[9]&0xff) + int(c1[10]&0xff) + int(c1[11]&0xff)
	// 这里offset可能太大 可能需要处理一下   比如  offset %  C1S1DigestOffsetMax 可以保证offset不会超出OFFSET_MAX
	digestDataOffset := (offset % C1S1DigestOffsetMax) + C1S1TimeSize + C1S1VersionSize + C1S1DigestOffsetSize
	// if digestDataOffset+32 > C1S1DigestSize {
	// 	// 这里表示该offset不合法 太大了
	// }

	return digestDataOffset
}

func ReadByteToBuf(conn io.Reader, size int) ([]byte, error) {
	buf := make([]byte, size)

	_, err := conn.Read(buf)
	if err != nil {
		return buf, err
	}

	return buf, err
}

func getKeyOffset(c1 []byte) int {
	/*
		key构成 共764byte:
			randomData(offset)  + keyData(128) + randomData(764-offset-128-4) + offset(4)
			0 <= keyOffset <= 764-128-4
	*/

	offset := int(c1[1532]&0xff) + int(c1[1533]&0xff) + int(c1[1534]&0xff) + int(c1[1535]&0xff)
	keyDataOffset := (offset % C1S1KeyOffsetMax) + C1S1VersionSize + C1S1TimeSize + C1S1DigestSize

	// if keyDataOffset > C1S1KeySize {
	// 	// 这里key太大了 需要返回 error
	// }

	return keyDataOffset
}
