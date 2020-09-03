package until

var (
	BigEndian bigEndian
)

// 低位在高地址  高位在低地址
type bigEndian struct{}

func (bigEndian) Uint24(b []byte) uint32 { return uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16 }
