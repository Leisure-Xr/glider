package tcp

import (
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// TCP 结构体。
type TCP struct {
	addr   string
	dialer proxy.Dialer
	proxy  proxy.Proxy
}

func init() {
	proxy.RegisterDialer("tcp", NewTCPDialer)
	proxy.RegisterServer("tcp", NewTCPServer)
}

// NewTCP 返回一个 tcp 结构体。
func NewTCP(s string, d proxy.Dialer, p proxy.Proxy) (*TCP, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[tcp] parse url err: %s", err)
		return nil, err
	}

	t := &TCP{
		dialer: d,
		proxy:  p,
		addr:   u.Host,
	}

	return t, nil
}

// NewTCPDialer 返回一个 tcp 拨号器。
func NewTCPDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewTCP(s, d, nil)
}

// NewTCPServer 返回真实服务器前的 tcp 传输层。
func NewTCPServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewTCP(s, nil, p)
}

// ListenAndServe 在服务器地址上监听并处理连接。
func (s *TCP) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[tcp] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[tcp] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[tcp] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve 处理一个连接。
func (s *TCP) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	rc, dialer, err := s.proxy.Dial("tcp", "")
	if err != nil {
		log.F("[tcp] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}
	defer rc.Close()

	log.F("[tcp] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[tcp] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// 仅记录远程连接失败
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// Addr 返回转发器的地址。
func (s *TCP) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到网络 net 上的地址 addr。
func (s *TCP) Dial(network, addr string) (net.Conn, error) {
	return s.dialer.Dial("tcp", s.addr)
}

// DialUDP 通过代理连接到给定地址。
func (s *TCP) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}
