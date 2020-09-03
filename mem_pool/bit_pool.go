package mem_pool

import "github.com/funny/slab"

var bitPool *slab.ChanPool

func InitPool() {
	// 最小的Chunk为16 最大的为 64kb ，增长系数为 2
	// 每个page的大小
	bitPool = slab.NewChanPool(16, 64*1024, 2, 1024*1024)
}

func GetSlice(s int) []byte {
	return bitPool.Alloc(s)
}

func RecycleSlice(slice []byte) {
	bitPool.Free(slice)
}

//
//func GetSlice(n int) []byte {
//
//}
