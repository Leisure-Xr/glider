// Package obfs 实现了 SS 的 simple-obfs 混淆功能
package obfs

import (
	"errors"
	"net"
	"net/url"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// Obfs 是混淆代理结构体。
type Obfs struct {
	dialer proxy.Dialer
	addr   string

	obfsType string
	obfsHost string
	obfsURI  string
	obfsUA   string

	obfsConn func(c net.Conn) (net.Conn, error)
}

func init() {
	proxy.RegisterDialer("simple-obfs", NewObfsDialer)
}

// NewObfs 返回一个代理结构体。
func NewObfs(s string, d proxy.Dialer) (*Obfs, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host

	query := u.Query()
	obfsType := query.Get("type")
	if obfsType == "" {
		obfsType = "http"
	}

	obfsHost := query.Get("host")
	if obfsHost == "" {
		return nil, errors.New("[obfs] host cannot be null")
	}

	obfsURI := query.Get("uri")
	if obfsURI == "" {
		obfsURI = "/"
	}

	obfsUA := query.Get("ua")
	if obfsUA == "" {
		obfsUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/71.0.3578.80 Safari/537.36"
	}

	p := &Obfs{
		dialer:   d,
		addr:     addr,
		obfsType: obfsType,
		obfsHost: obfsHost,
		obfsURI:  obfsURI,
		obfsUA:   obfsUA,
	}

	switch obfsType {
	case "http":
		httpObfs := NewHTTPObfs(obfsHost, obfsURI, obfsUA)
		p.obfsConn = httpObfs.NewConn
	case "tls":
		tlsObfs := NewTLSObfs(obfsHost)
		p.obfsConn = tlsObfs.NewConn
	default:
		return nil, errors.New("[obfs] unknown obfs type: " + obfsType)
	}

	return p, nil
}

// NewObfsDialer 返回一个代理拨号器。
func NewObfsDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewObfs(s, dialer)
}

// Addr 返回转发器地址。
func (s *Obfs) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到指定网络地址。
func (s *Obfs) Dial(network, addr string) (net.Conn, error) {
	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[obfs] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	return s.obfsConn(c)
}

// DialUDP 通过代理连接到指定地址（UDP）。
func (s *Obfs) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}

func init() {
	proxy.AddUsage("simple-obfs", `
Simple-Obfs（简单混淆）方案：
  simple-obfs://host:port[?type=TYPE&host=HOST&uri=URI&ua=UA]

Simple-Obfs 可用类型：
  http, tls
`)
}
