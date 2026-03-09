package smux

import (
	"net"
	"net/url"
	"sync"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/smux"
	"github.com/nadoo/glider/proxy"
)

// SmuxClient 结构体。
type SmuxClient struct {
	dialer  proxy.Dialer
	addr    string
	mu      sync.Mutex
	session *smux.Session
}

func init() {
	proxy.RegisterDialer("smux", NewSmuxDialer)
}

// NewSmuxDialer 返回一个 smux 拨号器。
func NewSmuxDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[smux] parse url err: %s", err)
		return nil, err
	}

	c := &SmuxClient{
		dialer: d,
		addr:   u.Host,
	}

	return c, nil
}

// Addr 返回转发器的地址。
func (s *SmuxClient) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到网络 net 上的地址 addr。
func (s *SmuxClient) Dial(network, addr string) (net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != nil {
		if c, err := s.session.OpenStream(); err == nil {
			return c, err
		}
		s.session.Close()
	}
	if err := s.initConn(); err != nil {
		return nil, err
	}
	return s.session.OpenStream()
}

// DialUDP 通过代理连接到给定地址。
func (s *SmuxClient) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}

func (s *SmuxClient) initConn() error {
	conn, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[smux] dial to %s error: %s", s.addr, err)
		return err
	}
	s.session, err = smux.Client(conn, nil)
	return err
}
