package smux

import (
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/smux"
	"github.com/nadoo/glider/proxy"
)

// SmuxServer 结构体。
type SmuxServer struct {
	proxy  proxy.Proxy
	addr   string
	server proxy.Server
}

func init() {
	proxy.RegisterServer("smux", NewSmuxServer)
}

// NewSmuxServer 返回真实服务器前的 smux 传输层。
func NewSmuxServer(s string, p proxy.Proxy) (proxy.Server, error) {
	schemes := strings.SplitN(s, ",", 2)
	u, err := url.Parse(schemes[0])
	if err != nil {
		log.F("[smux] parse url err: %s", err)
		return nil, err
	}

	m := &SmuxServer{
		proxy: p,
		addr:  u.Host,
	}

	if len(schemes) > 1 {
		m.server, err = proxy.ServerFromURL(schemes[1], p)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

// ListenAndServe 在服务器地址上监听并处理连接。
func (s *SmuxServer) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[smux] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[smux] listening mux on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[smux] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve 处理一个连接。
func (s *SmuxServer) Serve(c net.Conn) {
	// 我们知道内部服务器在处理完后会关闭连接
	// defer c.Close()

	session, err := smux.Server(c, nil)
	if err != nil {
		log.F("[smux] failed to create session: %v", err)
		return
	}

	for {
		// 接受一个流
		stream, err := session.AcceptStream()
		if err != nil {
			session.Close()
			break
		}
		go s.ServeStream(stream)
	}
}

// ServeStream 处理一个 smux 流。
func (s *SmuxServer) ServeStream(c *smux.Stream) {
	if s.server != nil {
		s.server.Serve(c)
		return
	}

	defer c.Close()

	rc, dialer, err := s.proxy.Dial("tcp", "")
	if err != nil {
		log.F("[smux] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}
	defer rc.Close()

	log.F("[smux] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[smux] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// 仅记录远程连接失败
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}

}
