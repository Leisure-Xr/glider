package socks5

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/pkg/socks"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("socks5", NewSocks5Dialer)
}

// NewSocks5Dialer 返回一个 socks5 代理拨号器。
func NewSocks5Dialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewSocks5(s, d, nil)
}

// Addr 返回转发器的地址。
func (s *Socks5) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过 SOCKS5 代理连接到网络 net 上的地址 addr。
func (s *Socks5) Dial(network, addr string) (net.Conn, error) {
	c, err := s.dial(network, s.addr)
	if err != nil {
		log.F("[socks5]: dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if _, err := s.connect(c, addr, socks.CmdConnect); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

func (s *Socks5) dial(network, addr string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp6", "tcp4":
	default:
		return nil, errors.New("[socks5]: no support for connection type " + network)
	}

	c, err := s.dialer.Dial(network, s.addr)
	if err != nil {
		log.F("[socks5]: dial to %s error: %s", s.addr, err)
		return nil, err
	}

	return c, nil
}

// DialUDP 通过代理连接到指定地址。
func (s *Socks5) DialUDP(network, addr string) (pc net.PacketConn, err error) {
	c, err := s.dial("tcp", s.addr)
	if err != nil {
		log.F("[socks5] dialudp dial tcp to %s error: %s", s.addr, err)
		return nil, err
	}

	var uAddr socks.Addr
	if uAddr, err = s.connect(c, addr, socks.CmdUDPAssociate); err != nil {
		c.Close()
		return nil, err
	}

	buf := pool.GetBuffer(socks.MaxAddrLen)
	defer pool.PutBuffer(buf)

	uAddress := uAddr.String()
	h, p, _ := net.SplitHostPort(uAddress)
	// if returned bind ip is unspecified
	if ip, err := netip.ParseAddr(h); err == nil && ip.IsUnspecified() {
		// indicate using conventional addr
		h, _, _ = net.SplitHostPort(s.addr)
		uAddress = net.JoinHostPort(h, p)
	}

	pc, err = s.dialer.DialUDP(network, uAddress)
	if err != nil {
		log.F("[socks5] dialudp to %s error: %s", uAddress, err)
		return nil, err
	}

	writeTo, err := net.ResolveUDPAddr("udp", uAddress)
	if err != nil {
		log.F("[socks5] resolve addr error: %s", err)
		return nil, err
	}

	return NewPktConn(pc, writeTo, socks.ParseAddr(addr), c), err
}

// connect 接受一个已有的到 socks5 代理服务器的连接，
// 并命令服务器将该连接延伸到目标地址，
// 目标地址必须是包含主机和端口的规范地址。
func (s *Socks5) connect(conn net.Conn, target string, cmd byte) (addr socks.Addr, err error) {
	// 此处大小仅为估算
	buf := pool.GetBuffer(socks.MaxAddrLen)
	defer pool.PutBuffer(buf)

	buf = append(buf[:0], Version)
	if len(s.user) > 0 && len(s.user) < 256 && len(s.password) < 256 {
		buf = append(buf, 2 /* num auth methods */, socks.AuthNone, socks.AuthPassword)
	} else {
		buf = append(buf, 1 /* num auth methods */, socks.AuthNone)
	}

	if _, err := conn.Write(buf); err != nil {
		return addr, errors.New("proxy: failed to write greeting to SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return addr, errors.New("proxy: failed to read greeting from SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}
	if buf[0] != Version {
		return addr, errors.New("proxy: SOCKS5 proxy at " + s.addr + " has unexpected version " + strconv.Itoa(int(buf[0])))
	}
	if buf[1] == 0xff {
		return addr, errors.New("proxy: SOCKS5 proxy at " + s.addr + " requires authentication")
	}

	if buf[1] == socks.AuthPassword {
		buf = buf[:0]
		buf = append(buf, 1 /* password protocol version */)
		buf = append(buf, uint8(len(s.user)))
		buf = append(buf, s.user...)
		buf = append(buf, uint8(len(s.password)))
		buf = append(buf, s.password...)

		if _, err := conn.Write(buf); err != nil {
			return addr, errors.New("proxy: failed to write authentication request to SOCKS5 proxy at " + s.addr + ": " + err.Error())
		}

		if _, err := io.ReadFull(conn, buf[:2]); err != nil {
			return addr, errors.New("proxy: failed to read authentication reply from SOCKS5 proxy at " + s.addr + ": " + err.Error())
		}

		if buf[1] != 0 {
			return addr, errors.New("proxy: SOCKS5 proxy at " + s.addr + " rejected username/password")
		}
	}

	buf = buf[:0]
	buf = append(buf, Version, cmd, 0 /* reserved */)
	buf = append(buf, socks.ParseAddr(target)...)

	if _, err := conn.Write(buf); err != nil {
		return addr, errors.New("proxy: failed to write connect request to SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	// 读取 VER REP RSV
	if _, err := io.ReadFull(conn, buf[:3]); err != nil {
		return addr, errors.New("proxy: failed to read connect reply from SOCKS5 proxy at " + s.addr + ": " + err.Error())
	}

	failure := "unknown error"
	if int(buf[1]) < len(socks.Errors) {
		failure = socks.Errors[buf[1]].Error()
	}

	if len(failure) > 0 {
		return addr, errors.New("proxy: SOCKS5 proxy at " + s.addr + " failed to connect: " + failure)
	}

	return socks.ReadAddr(conn)
}
