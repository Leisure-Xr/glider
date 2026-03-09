package ss

import (
	"errors"
	"net"

	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/pkg/socks"
)

// PktConn 数据包连接。
type PktConn struct {
	net.PacketConn
	writeTo net.Addr
	target  socks.Addr // 若 target 不为 nil，则可能是隧道模式
}

// NewPktConn 返回一个 PktConn，writeAddr 必须是 *net.UDPAddr 或 *net.UnixAddr。
func NewPktConn(c net.PacketConn, writeAddr net.Addr, targetAddr socks.Addr) *PktConn {
	return &PktConn{PacketConn: c, writeTo: writeAddr, target: targetAddr}
}

// ReadFrom 覆盖 net.PacketConn 中的原始函数。
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, _, target, err := pc.readFrom(b)
	return n, target, err
}

func (pc *PktConn) readFrom(b []byte) (int, net.Addr, net.Addr, error) {
	buf := pool.GetBuffer(len(b))
	defer pool.PutBuffer(buf)

	n, raddr, err := pc.PacketConn.ReadFrom(buf)
	if err != nil {
		return n, raddr, nil, err
	}

	tgtAddr := socks.SplitAddr(buf[:n])
	if tgtAddr == nil {
		return n, raddr, nil, errors.New("can not get target addr")
	}

	target, err := net.ResolveUDPAddr("udp", tgtAddr.String())
	if err != nil {
		return n, raddr, nil, errors.New("wrong target addr")
	}

	if pc.writeTo == nil {
		pc.writeTo = raddr
	}

	if pc.target == nil {
		pc.target = make([]byte, len(tgtAddr))
		copy(pc.target, tgtAddr)
	}

	n = copy(b, buf[len(tgtAddr):n])
	return n, raddr, target, err
}

// WriteTo 覆盖 net.PacketConn 中的原始函数
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	target := pc.target
	if addr != nil {
		target = socks.ParseAddr(addr.String())
	}

	if target == nil {
		return 0, errors.New("invalid addr")
	}

	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	tgtLen, _ := buf.Write(target)
	buf.Write(b)

	n, err := pc.PacketConn.WriteTo(buf.Bytes(), pc.writeTo)
	if n > tgtLen {
		return n - tgtLen, err
	}

	return 0, err
}
