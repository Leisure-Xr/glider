// https://www.rfc-editor.org/rfc/rfc5246
// https://golang.org/src/crypto/tls/handshake_messages.go

// NOTE:
// 官方 obfs-server 仅检查 client hello 数据包的 6 个静态字节，
// 因此如果我们发送一个格式错误的数据包，例如：设置了错误的扩展长度，
// obfs-server 会将其视为正确的数据包，但在 wireshark 中它是格式错误的。

package obfs

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"time"

	"github.com/nadoo/glider/pkg/pool"
)

const (
	lenSize   = 2
	chunkSize = 1 << 13 // 8192
)

// TLSObfs 结构体
type TLSObfs struct {
	obfsHost string
}

// NewTLSObfs 返回一个 TLSObfs 对象。
func NewTLSObfs(obfsHost string) *TLSObfs {
	return &TLSObfs{obfsHost: obfsHost}
}

// TLSObfsConn 结构体
type TLSObfsConn struct {
	*TLSObfs

	net.Conn
	reqSent   bool
	reader    *bufio.Reader
	buf       [lenSize]byte
	leftBytes int
}

// NewConn 返回一个新的混淆连接。
func (p *TLSObfs) NewConn(c net.Conn) (net.Conn, error) {
	cc := &TLSObfsConn{Conn: c, TLSObfs: p}
	return cc, nil
}

func (c *TLSObfsConn) Write(b []byte) (int, error) {
	if !c.reqSent {
		c.reqSent = true
		return c.handshake(b)
	}

	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	n := len(b)
	for i := 0; i < n; i += chunkSize {
		buf.Reset()
		end := min(i+chunkSize, n)

		buf.Write([]byte{0x17, 0x03, 0x03})
		binary.Write(buf, binary.BigEndian, uint16(len(b[i:end])))
		buf.Write(b[i:end])

		_, err := c.Conn.Write(buf.Bytes())
		if err != nil {
			return 0, err
		}
	}

	return n, nil
}

func (c *TLSObfsConn) Read(b []byte) (int, error) {
	if c.reader == nil {
		c.reader = bufio.NewReader(c.Conn)
		// 服务器 Hello
		// TLSv1.2 记录层：握手协议：Server Hello（96 字节）
		// TLSv1.2 记录层：Change Cipher Spec 协议：Change Cipher Spec（6 字节）
		c.reader.Discard(102)
	}

	if c.leftBytes == 0 {
		// TLSv1.2 记录层：
		// 第一个数据包：握手加密消息 / 后续数据包：应用数据
		// 1 字节：内容类型：握手 (22) / 应用数据 (23)
		// 2 字节：版本：TLS 1.2 (0x0303)
		c.reader.Discard(3)

		// 获取长度
		_, err := io.ReadFull(c.reader, c.buf[:lenSize])
		if err != nil {
			return 0, err
		}

		c.leftBytes = int(binary.BigEndian.Uint16(c.buf[:lenSize]))
	}

	readLen := len(b)
	if readLen > c.leftBytes {
		readLen = c.leftBytes
	}

	m, err := c.reader.Read(b[:readLen])
	if err != nil {
		return 0, err
	}

	c.leftBytes -= m

	return m, nil
}

func (c *TLSObfsConn) handshake(b []byte) (int, error) {
	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	bufExt := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(bufExt)

	bufHello := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(bufHello)

	// 准备扩展和 clientHello 内容
	extension(b, c.obfsHost, bufExt)
	clientHello(bufHello)

	// 准备长度
	extLen := bufExt.Len()
	helloLen := bufHello.Len() + 2 + extLen // 2: len(extContentLength)
	handshakeLen := 4 + helloLen            // 1: len(0x01) + 3: len(clientHelloContentLength)

	// TLS 记录层开始
	// 内容类型：握手 (22)
	buf.WriteByte(0x16)

	// 版本：TLS 1.0 (0x0301)
	buf.Write([]byte{0x03, 0x01})

	// 长度
	binary.Write(buf, binary.BigEndian, uint16(handshakeLen))

	// 握手开始
	// 握手类型：Client Hello (1)
	buf.WriteByte(0x01)

	// 长度：uint24（3 字节），但 golang 没有此类型
	buf.Write([]byte{uint8(helloLen >> 16), uint8(helloLen >> 8), uint8(helloLen)})

	// clientHello 内容
	buf.Write(bufHello.Bytes())

	// 扩展开始
	// 扩展内容长度
	binary.Write(buf, binary.BigEndian, uint16(extLen))

	// 扩展内容
	buf.Write(bufExt.Bytes())

	_, err := c.Conn.Write(buf.Bytes())

	if err != nil {
		return 0, err
	}

	return len(b), nil
}

func clientHello(buf *bytes.Buffer) {
	// 版本：TLS 1.2 (0x0303)
	buf.Write([]byte{0x03, 0x03})

	// 随机数
	// https://tools.ietf.org/id/draft-mathewson-no-gmtunixtime-00.txt
	// 注意：
	// 大多数 tls 实现不处理前 4 字节的 unix 时间，
	// 客户端不发送当前时间，服务器也不检查，
	// golang tls 客户端和 chrome 浏览器发送随机字节。
	//
	binary.Write(buf, binary.BigEndian, uint32(time.Now().Unix()))
	random := make([]byte, 28)
	// 以上 2 行代码是为了与某些服务器实现保持兼容，
	// 如果不需要兼容性，可以使用以下代码替代。
	// random := make([]byte, 32)

	rand.Read(random)
	buf.Write(random)

	// 会话 ID 长度: 32
	buf.WriteByte(32)
	// 会话 ID
	sessionID := make([]byte, 32)
	rand.Read(sessionID)
	buf.Write(sessionID)

	// https://github.com/shadowsocks/simple-obfs/blob/7659eeccf473aa41eb294e92c32f8f60a8747325/src/obfs_tls.c#L57
	// 密码套件长度: 56
	binary.Write(buf, binary.BigEndian, uint16(56))
	// 密码套件 (28 suites)
	buf.Write([]byte{
		0xc0, 0x2c, 0xc0, 0x30, 0x00, 0x9f, 0xcc, 0xa9, 0xcc, 0xa8, 0xcc, 0xaa, 0xc0, 0x2b, 0xc0, 0x2f,
		0x00, 0x9e, 0xc0, 0x24, 0xc0, 0x28, 0x00, 0x6b, 0xc0, 0x23, 0xc0, 0x27, 0x00, 0x67, 0xc0, 0x0a,
		0xc0, 0x14, 0x00, 0x39, 0xc0, 0x09, 0xc0, 0x13, 0x00, 0x33, 0x00, 0x9d, 0x00, 0x9c, 0x00, 0x3d,
		0x00, 0x3c, 0x00, 0x35, 0x00, 0x2f, 0x00, 0xff,
	})

	// 压缩方法长度: 1
	buf.WriteByte(0x01)
	// 压缩方法（1 种方法）
	buf.WriteByte(0x00)
}

func extension(b []byte, server string, buf *bytes.Buffer) {
	// 扩展：SessionTicket TLS
	buf.Write([]byte{0x00, 0x23}) // 类型
	// 注意：在 sessionticket 中发送一些数据，服务器也会将其视为数据
	binary.Write(buf, binary.BigEndian, uint16(len(b))) // 长度
	buf.Write(b)

	// 扩展：服务器名称
	buf.Write([]byte{0x00, 0x00})                              // 类型
	binary.Write(buf, binary.BigEndian, uint16(len(server)+5)) // 长度
	binary.Write(buf, binary.BigEndian, uint16(len(server)+3)) // 服务器名称列表长度
	buf.WriteByte(0x00)                                        // 服务器名称类型：host_name (0)
	binary.Write(buf, binary.BigEndian, uint16(len(server)))   // 服务器名称长度
	buf.WriteString(server)

	// https://github.com/shadowsocks/simple-obfs/blob/7659eeccf473aa41eb294e92c32f8f60a8747325/src/obfs_tls.c#L88
	// 扩展：ec_point_formats (len=4)
	buf.Write([]byte{0x00, 0x0b})                  // 类型
	binary.Write(buf, binary.BigEndian, uint16(4)) // 长度
	buf.WriteByte(0x03)                            // 格式长度
	buf.Write([]byte{0x01, 0x00, 0x02})

	// 扩展：supported_groups (len=10)
	buf.Write([]byte{0x00, 0x0a})                   // 类型
	binary.Write(buf, binary.BigEndian, uint16(10)) // 长度
	binary.Write(buf, binary.BigEndian, uint16(8))  // 支持的组列表长度：8
	buf.Write([]byte{0x00, 0x1d, 0x00, 0x17, 0x00, 0x19, 0x00, 0x18})

	// 扩展：signature_algorithms (len=32)
	buf.Write([]byte{0x00, 0x0d})                   // 类型
	binary.Write(buf, binary.BigEndian, uint16(32)) // 长度
	binary.Write(buf, binary.BigEndian, uint16(30)) // 签名哈希算法长度：30
	buf.Write([]byte{
		0x06, 0x01, 0x06, 0x02, 0x06, 0x03, 0x05, 0x01, 0x05, 0x02, 0x05, 0x03, 0x04, 0x01, 0x04, 0x02,
		0x04, 0x03, 0x03, 0x01, 0x03, 0x02, 0x03, 0x03, 0x02, 0x01, 0x02, 0x02, 0x02, 0x03,
	})

	// 扩展：encrypt_then_mac (len=0)
	buf.Write([]byte{0x00, 0x16})                  // 类型
	binary.Write(buf, binary.BigEndian, uint16(0)) // 长度

	// 扩展：extended_master_secret (len=0)
	buf.Write([]byte{0x00, 0x17})                  // 类型
	binary.Write(buf, binary.BigEndian, uint16(0)) // 长度
}
