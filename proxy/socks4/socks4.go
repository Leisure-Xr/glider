// https://www.openssh.com/txt/socks4.protocol

// socks4 客户端

package socks4

import (
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

const (
	// Version 是 SOCKS4 的版本号。
	Version = 4
	// ConnectCommand 是连接命令字节
	ConnectCommand = 1
)

// SOCKS4 是 SOCKS4 的基础结构体。
type SOCKS4 struct {
	dialer  proxy.Dialer
	addr    string
	socks4a bool
}

func init() {
	proxy.RegisterDialer("socks4", NewSocks4Dialer)
	proxy.RegisterDialer("socks4a", NewSocks4Dialer)
}

// NewSOCKS4 返回一个 SOCKS4 代理。
func NewSOCKS4(s string, dialer proxy.Dialer) (*SOCKS4, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	h := &SOCKS4{
		dialer:  dialer,
		addr:    u.Host,
		socks4a: u.Scheme == "socks4a",
	}

	return h, nil
}

// NewSocks4Dialer 返回一个 SOCKS4 代理拨号器。
func NewSocks4Dialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewSOCKS4(s, dialer)
}

// Addr 返回转发器的地址。
func (s *SOCKS4) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过 SOCKS4 代理连接到网络 net 上的地址 addr。
func (s *SOCKS4) Dial(network, addr string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4":
	default:
		return nil, errors.New("[socks4] no support for connection type " + network)
	}

	c, err := s.dialer.Dial(network, s.addr)
	if err != nil {
		log.F("[socks4] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if err := s.connect(c, addr); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

// DialUDP 通过代理连接到给定地址（UDP）。
func (s *SOCKS4) DialUDP(network, addr string) (pc net.PacketConn, err error) {
	return nil, proxy.ErrNotSupported
}

func (s *SOCKS4) lookupIP(host string) (ip net.IP, err error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return
	}
	if len(ips) == 0 {
		err = errors.New("[socks4] Cannot resolve host: " + host)
		return
	}
	ip = ips[0].To4()
	if len(ip) != net.IPv4len {
		err = errors.New("[socks4] IPv6 is not supported by socks4")
		return
	}
	return
}

// connect 接受一个已有的 SOCKS4 代理服务器连接，
// 并命令服务器将该连接延伸至目标地址，
// 目标地址必须是包含主机和端口的规范地址。
func (s *SOCKS4) connect(conn net.Conn, target string) error {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return errors.New("[socks4] failed to parse port number: " + portStr)
	}

	const baseBufSize = 8 + 1 // 1 is the len(userid)
	bufSize := baseBufSize
	var ip net.IP
	if ip = net.ParseIP(host); ip == nil {
		if s.socks4a {
			// 客户端应将 DSTIP 的前三个字节设为 NULL，
			// 最后一个字节设为非零值。
			ip = []byte{0, 0, 0, 1}
			bufSize += len(host) + 1
		} else {
			ip, err = s.lookupIP(host)
			if err != nil {
				return err
			}
		}
	} else {
		ip = ip.To4()
		if ip == nil {
			return errors.New("[socks4] IPv6 is not supported by socks4")
		}
	}
	// 参考自 https://github.com/h12w/socks/blob/master/socks.go 和 https://en.wikipedia.org/wiki/SOCKS
	buf := pool.GetBuffer(bufSize)
	defer pool.PutBuffer(buf)
	copy(buf, []byte{
		Version,
		ConnectCommand,
		byte(port >> 8), // 目标端口高字节
		byte(port),      // 目标端口低字节（大端序）
		ip[0], ip[1], ip[2], ip[3],
		0, // 用户 ID
	})
	if s.socks4a {
		copy(buf[baseBufSize:], host)
		buf[len(buf)-1] = 0
	}

	resp := pool.GetBuffer(8)
	defer pool.PutBuffer(resp)

	if _, err := conn.Write(buf); err != nil {
		return errors.New("[socks4] failed to write greeting to socks4 proxy at " + s.addr + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, resp); err != nil {
		return errors.New("[socks4] failed to read greeting from socks4 proxy at " + s.addr + ": " + err.Error())
	}

	switch resp[1] {
	case 0x5a:
		// 请求已批准
	case 0x5b:
		err = errors.New("[socks4] connection request rejected or failed")
	case 0x5c:
		err = errors.New("[socks4] connection request request failed because client is not running identd (or not reachable from the server)")
	case 0x5d:
		err = errors.New("[socks4] connection request request failed because client's identd could not confirm the user ID in the request")
	default:
		err = errors.New("[socks4] connection request failed, unknown error")
	}

	return err
}

func init() {
	proxy.AddUsage("socks4", `
SOCKS4 方案：
  socks4://host:port
`)
}
