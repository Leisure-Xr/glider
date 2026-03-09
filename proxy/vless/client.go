package vless

import (
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

// NewVLessDialer 返回一个 vless 代理拨号器。
func NewVLessDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewVLess(s, dialer, nil)
}

// Addr 返回转发器的地址。
func (s *VLess) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到网络 net 上的地址 addr。
func (s *VLess) Dial(network, addr string) (net.Conn, error) {
	return s.dial(network, addr)
}

func (s *VLess) dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[vless]: dial to %s error: %s", s.addr, err)
		return nil, err
	}
	return NewClientConn(rc, s.uuid, network, addr)
}

// DialUDP 通过代理连接到给定地址（UDP）。
func (s *VLess) DialUDP(network, addr string) (net.PacketConn, error) {
	c, err := s.dial("udp", addr)
	if err != nil {
		return nil, err
	}

	tgtAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.F("[vless] error in ResolveUDPAddr: %v", err)
		return nil, err
	}

	return NewPktConn(c, tgtAddr), err
}

// ClientConn 是 vless 客户端连接。
type ClientConn struct {
	net.Conn
	rcved bool
}

// NewClientConn 返回一个新的 vless 客户端连接。
func NewClientConn(c net.Conn, uuid [16]byte, network, target string) (*ClientConn, error) {
	atyp, addr, port, err := ParseAddr(target)
	if err != nil {
		return nil, err
	}

	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	buf.WriteByte(Version) // 版本
	buf.Write(uuid[:])     // UUID
	buf.WriteByte(0)       // 附加数据长度

	cmd := CmdTCP
	if network == "udp" {
		cmd = CmdUDP
	}
	buf.WriteByte(byte(cmd)) // 命令

	// 目标地址
	err = binary.Write(buf, binary.BigEndian, uint16(port)) // 端口
	if err != nil {
		return nil, err
	}
	buf.WriteByte(byte(atyp)) // 地址类型
	buf.Write(addr)           // 地址

	_, err = c.Write(buf.Bytes())
	return &ClientConn{Conn: c}, err
}

func (c *ClientConn) Read(b []byte) (n int, err error) {
	if !c.rcved {
		buf := pool.GetBuffer(2)
		defer pool.PutBuffer(buf)

		n, err = io.ReadFull(c.Conn, buf)
		if err != nil {
			return
		}

		if buf[0] != Version {
			return n, errors.New("version not supported")
		}

		if addLen := int64(buf[1]); addLen > 0 {
			proxy.CopyN(io.Discard, c.Conn, addLen)
		}
		c.rcved = true
	}

	return c.Conn.Read(b)
}
