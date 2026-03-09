package obfs

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net"

	"github.com/nadoo/glider/pkg/pool"
)

// HTTPObfs 结构体
type HTTPObfs struct {
	obfsHost string
	obfsURI  string
	obfsUA   string
}

// NewHTTPObfs 返回一个 HTTPObfs 对象。
func NewHTTPObfs(obfsHost, obfsURI, obfsUA string) *HTTPObfs {
	return &HTTPObfs{obfsHost, obfsURI, obfsUA}
}

// HTTPObfsConn 结构体
type HTTPObfsConn struct {
	*HTTPObfs

	net.Conn
	reader io.Reader
}

// NewConn 返回一个新的混淆连接。
func (p *HTTPObfs) NewConn(c net.Conn) (net.Conn, error) {
	cc := &HTTPObfsConn{
		Conn:     c,
		HTTPObfs: p,
	}

	// 向远程服务器发送 http 头部
	_, err := cc.writeHeader()
	return cc, err
}

func (c *HTTPObfsConn) writeHeader() (int, error) {
	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	buf.WriteString("GET " + c.obfsURI + " HTTP/1.1\r\n")
	buf.WriteString("Host: " + c.obfsHost + "\r\n")
	buf.WriteString("User-Agent: " + c.obfsUA + "\r\n")
	buf.WriteString("Upgrade: websocket\r\n")
	buf.WriteString("Connection: Upgrade\r\n")

	b := pool.GetBuffer(16)
	rand.Read(b)
	buf.WriteString("Sec-WebSocket-Key: " + base64.StdEncoding.EncodeToString(b) + "\r\n")
	pool.PutBuffer(b)

	buf.WriteString("\r\n")

	return c.Conn.Write(buf.Bytes())
}

func (c *HTTPObfsConn) Read(b []byte) (n int, err error) {
	if c.reader == nil {
		r := bufio.NewReader(c.Conn)
		c.reader = r
		for {
			l, _, err := r.ReadLine()
			if err != nil {
				return 0, err
			}

			if len(l) == 0 {
				break
			}
		}
	}

	return c.reader.Read(b)
}
