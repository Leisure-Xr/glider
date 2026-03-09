package ss

import (
	"errors"
	"net"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/socks"
	"github.com/nadoo/glider/proxy"
)

// NewSSDialer 返回一个 ss 代理拨号器。
func NewSSDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewSS(s, d, nil)
}

// Addr 返回转发器的地址。
func (s *SS) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到网络 net 上的地址 addr。
func (s *SS) Dial(network, addr string) (net.Conn, error) {
	target := socks.ParseAddr(addr)
	if target == nil {
		return nil, errors.New("[ss] unable to parse address: " + addr)
	}

	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[ss] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	c = s.StreamConn(c)
	if _, err = c.Write(target); err != nil {
		c.Close()
		return nil, err
	}

	return c, err
}

// DialUDP 通过代理连接到指定地址。
func (s *SS) DialUDP(network, addr string) (net.PacketConn, error) {
	pc, err := s.dialer.DialUDP(network, s.addr)
	if err != nil {
		log.F("[ss] dialudp to %s error: %s", s.addr, err)
		return nil, err
	}

	writeTo, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		log.F("[ss] resolve addr error: %s", err)
		return nil, err
	}

	pkc := NewPktConn(s.PacketConn(pc), writeTo, socks.ParseAddr(addr))
	return pkc, nil
}
