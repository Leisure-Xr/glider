package proxy

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/nadoo/glider/pkg/pool"
)

var (
	// TCPBufSize 是 TCP 缓冲区的大小。
	TCPBufSize = 32 << 10

	// UDPBufSize 是 UDP 缓冲区的大小。
	UDPBufSize = 2 << 10
)

// Conn 是带有缓冲读取器的连接。
type Conn struct {
	r *bufio.Reader
	net.Conn
}

// NewConn 返回一个新的 Conn。
func NewConn(c net.Conn) *Conn {
	if conn, ok := c.(*Conn); ok {
		return conn
	}
	return &Conn{pool.GetBufReader(c), c}
}

// Reader 返回内部的 bufio.Reader。
func (c *Conn) Reader() *bufio.Reader      { return c.r }
func (c *Conn) Read(p []byte) (int, error) { return c.r.Read(p) }

// Peek 返回接下来的 n 个字节，但不推进读取位置。
func (c *Conn) Peek(n int) ([]byte, error) { return c.r.Peek(n) }

// WriteTo 实现 io.WriterTo 接口。
func (c *Conn) WriteTo(w io.Writer) (n int64, err error) { return c.r.WriteTo(w) }

// Close 关闭 Conn。
func (c *Conn) Close() error {
	pool.PutBufReader(c.r)
	return c.Conn.Close()
}

// Relay 在 left 和 right 之间进行数据中继。
func Relay(left, right net.Conn) error {
	var err, err1 error
	var wg sync.WaitGroup
	var wait = 5 * time.Second

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err1 = Copy(right, left)
		right.SetReadDeadline(time.Now().Add(wait)) // 解除 right 端的读阻塞
	}()

	_, err = Copy(left, right)
	left.SetReadDeadline(time.Now().Add(wait)) // 解除 left 端的读阻塞
	wg.Wait()

	if err1 != nil && !errors.Is(err1, os.ErrDeadlineExceeded) {
		return err1
	}

	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	}

	return nil
}

// Copy 从 src 复制数据到 dst。
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	dst = underlyingWriter(dst)
	switch runtime.GOOS {
	case "linux", "windows", "dragonfly", "freebsd", "solaris":
		if _, ok := dst.(*net.TCPConn); ok && worthTry(src) {
			if wt, ok := src.(io.WriterTo); ok {
				return wt.WriteTo(dst)
			}
			if rt, ok := dst.(io.ReaderFrom); ok {
				return rt.ReadFrom(src)
			}
		}
	}
	return CopyBuffer(dst, src)
}

func underlyingWriter(c io.Writer) io.Writer {
	if wrap, ok := c.(*Conn); ok {
		return wrap.Conn
	}
	return c
}

func worthTry(src io.Reader) bool {
	switch v := src.(type) {
	case *net.TCPConn, *net.UnixConn:
		return true
	case *io.LimitedReader:
		return worthTry(v.R)
	case *Conn:
		return worthTry(v.Conn)
	case *os.File:
		fi, err := v.Stat()
		if err != nil {
			return false
		}
		return fi.Mode().IsRegular()
	default:
		return false
	}
}

// CopyN 从 src 复制 n 个字节（或直到出错）到 dst。
func CopyN(dst io.Writer, src io.Reader, n int64) (written int64, err error) {
	written, err = Copy(dst, io.LimitReader(src, n))
	if written == n {
		return n, nil
	}
	if written < n && err == nil {
		// src 提前停止，必定是遇到了 EOF。
		err = io.EOF
	}
	return
}

// CopyBuffer 使用用户空间缓冲区从 src 复制数据到 dst。
func CopyBuffer(dst io.Writer, src io.Reader) (written int64, err error) {
	size := TCPBufSize
	if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
		if l.N < 1 {
			size = 1
		} else {
			size = int(l.N)
		}
	}

	buf := pool.GetBuffer(size)
	defer pool.PutBuffer(buf)

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// CopyUDP 将 src 的数据复制到 dst 的目标地址，并设置读取超时。
// 若 step 设为非零值，
// 则每次读操作的超时时间将从 0 按 step 步长递增至 timeout。
func CopyUDP(dst net.PacketConn, writeTo net.Addr, src net.PacketConn, timeout time.Duration, step time.Duration) error {
	buf := pool.GetBuffer(UDPBufSize)
	defer pool.PutBuffer(buf)

	var t time.Duration
	for {
		if t += step; t == 0 || t > timeout {
			t = timeout
		}

		src.SetReadDeadline(time.Now().Add(t))
		n, addr, err := src.ReadFrom(buf)
		if err != nil {
			return err
		}

		if writeTo != nil {
			addr = writeTo
		}

		_, err = dst.WriteTo(buf[:n], addr)
		if err != nil {
			return err
		}
	}
}
