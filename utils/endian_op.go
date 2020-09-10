package utils

var (
	BigEndian bigEndian
)

// 低位在高地址  高位在低地址
type bigEndian struct{}

func (bigEndian) Uint24(b []byte) uint32 { return uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16 }

func (bigEndian) PutUint24(b []byte, v uint32) {
	b[0] = byte(v >> 16) // 这里先跳开 后2byte直接取高位byte 然后写进去
	b[1] = byte(v >> 8)  // 这里是跳开 最低位的byte取出第二个byte 写进去
	b[2] = byte(v)
}
