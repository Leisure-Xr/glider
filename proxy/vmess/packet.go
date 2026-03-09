package vmess

import (
	"net"
)

// PktConn 是一个 UDP 数据包连接。
type PktConn struct {
	net.Conn
	target *net.UDPAddr
}

// NewPktConn 返回一个 PktConn。
func NewPktConn(c net.Conn, target *net.UDPAddr) *PktConn {
	return &PktConn{Conn: c, target: target}
}

// ReadFrom 实现 net.PacketConn 所需的函数。
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := pc.Read(b)
	return n, pc.target, err
}

// WriteTo 实现 net.PacketConn 所需的函数。
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return pc.Write(b)
}
