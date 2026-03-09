package pool

import (
	"bufio"
	"io"
	"sync"
)

var bufReaderPool sync.Pool

// GetBufReader 从对象池中获取一个 *bufio.Reader。
func GetBufReader(r io.Reader) *bufio.Reader {
	if v := bufReaderPool.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(r)
		return br
	}
	return bufio.NewReader(r)
}

// PutBufReader 将 *bufio.Reader 放回对象池。
func PutBufReader(br *bufio.Reader) {
	bufReaderPool.Put(br)
}
