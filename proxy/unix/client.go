package unix

import (
	"net"
	"os"

	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("unix", NewUnixDialer)
}

// NewUnixDialer 返回一个 unix 域套接字拨号器。
func NewUnixDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewUnix(s, d, nil)
}

// Addr 返回转发器的地址。
func (s *Unix) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到网络 net 上的地址 addr。
// 注意：必须是链中的第一个拨号器
func (s *Unix) Dial(network, addr string) (net.Conn, error) {
	return net.Dial("unix", s.addr)
}

// DialUDP 通过代理连接到给定地址。
// 注意：必须是链中的第一个拨号器
func (s *Unix) DialUDP(network, addr string) (net.PacketConn, error) {
	laddru := s.addru + "_" + addr
	os.Remove(laddru)

	luaddru, err := net.ResolveUnixAddr("unixgram", laddru)
	if err != nil {
		return nil, err
	}

	pc, err := net.ListenUnixgram("unixgram", luaddru)
	if err != nil {
		return nil, err
	}

	return &PktConn{pc, laddru, luaddru, s.uaddru}, nil
}

// PktConn 数据包连接。
type PktConn struct {
	*net.UnixConn
	addr      string
	uaddr     *net.UnixAddr
	writeAddr *net.UnixAddr
}

// ReadFrom 覆盖 net.PacketConn 中的原始函数。
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, _, err := pc.UnixConn.ReadFrom(b)
	return n, pc.uaddr, err
}

// WriteTo 覆盖 net.PacketConn 中的原始函数。
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return pc.UnixConn.WriteTo(b, pc.writeAddr)
}

// Close 覆盖 net.PacketConn 中的原始函数。
func (pc *PktConn) Close() error {
	pc.UnixConn.Close()
	os.Remove(pc.addr)
	return nil
}
