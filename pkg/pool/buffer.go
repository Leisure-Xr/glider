package pool

import (
	"math/bits"
	"sync"
	"unsafe"
)

const (
	// 缓冲池数量。
	num     = 17
	maxsize = 1 << (num - 1)
)

var (
	sizes [num]int
	pools [num]sync.Pool
)

func init() {
	for i := range num {
		size := 1 << i
		sizes[i] = size
		pools[i].New = func() any {
			buf := make([]byte, size)
			return unsafe.SliceData(buf)
		}
	}
}

// GetBuffer 从缓冲池中获取一个缓冲区，size 应在范围 [1, 65536] 内，
// 否则此函数将直接调用 make([]byte, size)。
func GetBuffer(size int) []byte {
	if size >= 1 && size <= maxsize {
		i := bits.Len32(uint32(size - 1))
		if p := pools[i].Get().(*byte); p != nil {
			return unsafe.Slice(p, 1<<i)[:size]
		}
	}
	return make([]byte, size)
}

// PutBuffer 将缓冲区放回缓冲池。
func PutBuffer(buf []byte) {
	if size := cap(buf); size >= 1 && size <= maxsize {
		i := bits.Len32(uint32(size - 1))
		if sizes[i] == size {
			pools[i].Put(unsafe.SliceData(buf))
		}
	}
}
