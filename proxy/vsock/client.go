//go:build linux

package vsock

import (
	"net"

	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("vsock", NewVSockDialer)
}

// NewVSockDialer 返回一个虚拟机套接字拨号器。
func NewVSockDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewVSock(s, d, nil)
}

// Dial 通过代理连接到网络 net 上的地址 addr。
// 注意：必须是链中的第一个拨号器
func (s *vsock) Dial(network, addr string) (net.Conn, error) {
	return Dial(s.cid, s.port)
}

// DialUDP 通过代理连接到给定地址。
func (s *vsock) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}
