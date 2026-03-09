package vless

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

// NewVLessServer 返回一个 vless 代理服务器。
func NewVLessServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewVLess(s, nil, p)
}

// ListenAndServe 在服务器地址上监听并处理连接。
func (s *VLess) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[vless] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[vless] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[vless] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve 处理一个连接。
func (s *VLess) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	headBuf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(headBuf)

	cmd, target, err := s.readHeader(io.TeeReader(c, headBuf))
	if err != nil {
		// log.F("[vless] 验证来自 %s 的头部时出错: %v", c.RemoteAddr(), err)
		if s.fallback != "" {
			s.serveFallback(c, s.fallback, headBuf)
		}
		return
	}

	c = NewServerConn(c)

	network := "tcp"
	dialer := s.proxy.NextDialer(target)

	if cmd == CmdUDP {
		// 没有上游代理，直接处理
		if dialer.Addr() == "DIRECT" {
			s.ServeUoT(c, target)
			return
		}
		network = "udp"
	}

	rc, err := dialer.Dial(network, target)
	if err != nil {
		log.F("[vless] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), target, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[vless] %s <-> %s via %s", c.RemoteAddr(), target, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[vless] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), target, dialer.Addr(), err)
		// 仅记录远程连接失败
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

func (s *VLess) serveFallback(c net.Conn, tgt string, headBuf *bytes.Buffer) {
	// TODO: 应该直接访问回退地址还是通过代理？
	dialer := s.proxy.NextDialer(tgt)
	rc, err := dialer.Dial("tcp", tgt)
	if err != nil {
		log.F("[vless-fallback] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	_, err = rc.Write(headBuf.Bytes())
	if err != nil {
		log.F("[vless-fallback] write to rc error: %v", err)
		return
	}

	log.F("[vless-fallback] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[vless-fallback] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
	}
}

func (s *VLess) readHeader(r io.Reader) (CmdType, string, error) {
	// 版本: 1, uuid: 16, 附加数据长度: 1
	buf := pool.GetBuffer(18)
	defer pool.PutBuffer(buf)

	if _, err := io.ReadFull(r, buf[:18]); err != nil {
		return CmdErr, "", fmt.Errorf("read header error: %v", err)
	}

	if buf[0] != Version {
		return CmdErr, "", fmt.Errorf("version %d not supported", buf[0])
	}

	if !bytes.Equal(s.uuid[:], buf[1:17]) {
		return CmdErr, "", fmt.Errorf("auth failed, client id: %02x", buf[:16])
	}

	// 忽略附加数据
	if addonLen := int64(buf[17]); addonLen > 0 {
		proxy.CopyN(io.Discard, r, addonLen)
	}

	// 命令
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return CmdErr, "", fmt.Errorf("get cmd error: %v", err)
	}

	// 目标地址
	target, err := ReadAddrString(r)
	return CmdType(buf[0]), target, err
}

// ServeUoT 处理 UDP over TCP 请求。
func (s *VLess) ServeUoT(c net.Conn, tgt string) {
	rc, err := net.ListenPacket("udp", "")
	if err != nil {
		log.F("[vless] UDP listen error: %v", err)
		return
	}
	defer rc.Close()

	tgtAddr, err := net.ResolveUDPAddr("udp", tgt)
	if err != nil {
		log.F("[vless] error in ResolveUDPAddr: %v", err)
		return
	}

	pc := NewPktConn(c, tgtAddr)
	log.F("[vless] %s <-UoT-> %s <-> %s", c.RemoteAddr(), rc.LocalAddr(), tgt)

	go proxy.CopyUDP(rc, nil, pc, 2*time.Minute, 5*time.Second)
	proxy.CopyUDP(pc, nil, rc, 2*time.Minute, 5*time.Second)
}

// ServerConn 是 vless 服务端连接。
type ServerConn struct {
	net.Conn
	sent bool
}

// NewServerConn 返回一个新的 vless 服务端连接。
func NewServerConn(c net.Conn) *ServerConn {
	return &ServerConn{Conn: c}
}

func (c *ServerConn) Write(b []byte) (int, error) {
	if !c.sent {
		c.sent = true

		n, err := (&net.Buffers{[]byte{Version, 0}, b}).WriteTo(c.Conn)
		if n > 2 {
			return int(n) - 2, err
		}
		return 0, err
	}

	return c.Conn.Write(b)
}
