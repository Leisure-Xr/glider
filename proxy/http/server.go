package http

import (
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

// NewHTTPServer 返回一个 HTTP 代理服务器。
func NewHTTPServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewHTTP(s, nil, p)
}

// ListenAndServe 在服务器地址上监听并处理连接。
func (s *HTTP) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[http] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[http] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[http] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve 处理一个连接。
func (s *HTTP) Serve(cc net.Conn) {
	if c, ok := cc.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c := proxy.NewConn(cc)
	defer c.Close()

	req, err := parseRequest(c.Reader())
	if err != nil {
		log.F("[http] can not parse request from %s, error: %v", c.RemoteAddr(), err)
		return
	}

	if s.pretend {
		fmt.Fprintf(c, "%s 404 Not Found\r\nServer: nginx\r\n\r\n404 Not Found\r\n", req.proto)
		log.F("[http] %s <-> %s, pretend as web server", c.RemoteAddr().String(), s.Addr())
		return
	}

	s.servRequest(req, c)
}

func (s *HTTP) servRequest(req *request, c *proxy.Conn) {
	// 认证
	if s.user != "" && s.password != "" {
		if user, pass, ok := extractUserPass(req.auth); !ok || user != s.user || pass != s.password {
			io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic\r\n\r\n")
			log.F("[http] auth failed from %s, auth info: %s:%s", c.RemoteAddr(), user, pass)
			return
		}
	}

	if req.method == "CONNECT" {
		s.servHTTPS(req, c)
		return
	}

	s.servHTTP(req, c)
}

func (s *HTTP) servHTTPS(r *request, c net.Conn) {
	rc, dialer, err := s.proxy.Dial("tcp", r.uri)
	if err != nil {
		io.WriteString(c, r.proto+" 502 ERROR\r\n\r\n")
		log.F("[http] %s <-> %s [c] via %s, error in dial: %v", c.RemoteAddr(), r.uri, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")

	log.F("[http] %s <-> %s [c] via %s", c.RemoteAddr(), r.uri, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[http] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), r.uri, dialer.Addr(), err)
		// 仅记录远程连接失败
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

func (s *HTTP) servHTTP(req *request, c *proxy.Conn) {
	rc, dialer, err := s.proxy.Dial("tcp", req.target)
	if err != nil {
		fmt.Fprintf(c, "%s 502 ERROR\r\n\r\n", req.proto)
		log.F("[http] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), req.target, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	// 向远程服务器发送请求
	req.WriteBuf(buf)
	_, err = rc.Write(buf.Bytes())
	if err != nil {
		return
	}

	// 将剩余请求字节复制到远程服务器，例如：指定长度或分块的请求体。
	go func() {
		if _, err := c.Reader().Peek(1); err == nil {
			proxy.Copy(rc, c)
			rc.SetDeadline(time.Now())
			c.SetDeadline(time.Now())
		}
	}()

	r := pool.GetBufReader(rc)
	defer pool.PutBufReader(r)

	tpr := textproto.NewReader(r)
	line, err := tpr.ReadLine()
	if err != nil {
		return
	}

	proto, code, status, ok := parseStartLine(line)
	if !ok {
		return
	}

	header, err := tpr.ReadMIMEHeader()
	if err != nil {
		log.F("[http] read header error:%s", err)
		return
	}

	header.Set("Proxy-Connection", "close")
	header.Set("Connection", "close")

	buf.Reset()
	writeStartLine(buf, proto, code, status)
	writeHeaders(buf, header)

	log.F("[http] %s <-> %s via %s", c.RemoteAddr(), req.target, dialer.Addr())
	c.Write(buf.Bytes())

	proxy.Copy(c, r)
}
