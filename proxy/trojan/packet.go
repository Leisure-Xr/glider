package trojan

import (
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/pkg/socks"
)

// PktConn 是 UDP 数据包连接。
type PktConn struct {
	net.Conn
	target socks.Addr
}

// NewPktConn 返回一个 PktConn。
func NewPktConn(c net.Conn, target socks.Addr) *PktConn {
	return &PktConn{Conn: c, target: target}
}

// ReadFrom 实现 net.PacketConn 所需的函数。
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	// 地址类型, 目标地址, 目标端口
	tgtAddr, err := socks.ReadAddr(pc.Conn)
	if err != nil {
		return 0, nil, err
	}

	target, err := net.ResolveUDPAddr("udp", tgtAddr.String())
	if err != nil {
		return 0, nil, err
	}

	// TODO: 我们知道在 proxy.CopyUDP 中使用时 b 的长度足够，稍后再检查。
	if len(b) < 2 {
		return 0, nil, errors.New("buf size is not enough")
	}

	// 长度
	if _, err = io.ReadFull(pc.Conn, b[:2]); err != nil {
		return 0, nil, err
	}

	length := int(binary.BigEndian.Uint16(b[:2]))

	if len(b) < length {
		return 0, nil, errors.New("buf size is not enough")
	}

	// 回车换行
	if _, err = io.ReadFull(pc.Conn, b[:2]); err != nil {
		return 0, nil, err
	}

	// 载荷
	n, err := io.ReadFull(pc.Conn, b[:length])
	if err != nil {
		return n, nil, err
	}

	return n, target, err
}

// WriteTo 实现 net.PacketConn 所需的函数。
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
	binary.Write(buf, binary.BigEndian, uint16(len(b)))
	buf.WriteString("\r\n")
	buf.Write(b)

	n, err := pc.Write(buf.Bytes())
	if n > tgtLen+4 {
		return n - tgtLen - 4, nil
	}

	return 0, err
}
