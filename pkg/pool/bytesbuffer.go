package pool

import (
	"bytes"
	"sync"
)

var bytesBufPool = sync.Pool{
	New: func() any { return &bytes.Buffer{} },
}

// GetBytesBuffer 从对象池中获取一个 bytes.Buffer。
func GetBytesBuffer() *bytes.Buffer {
	return bytesBufPool.Get().(*bytes.Buffer)
}

// PutBytesBuffer 将 bytes.Buffer 放回对象池。
func PutBytesBuffer(buf *bytes.Buffer) {
	if buf.Cap() <= 64<<10 {
		buf.Reset()
		bytesBufPool.Put(buf)
	}
}
