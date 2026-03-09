package redir

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"unsafe"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// RedirProxy 结构体。
type RedirProxy struct {
	proxy proxy.Proxy
	addr  string
	ipv6  bool
}

func init() {
	proxy.RegisterServer("redir", NewRedirServer)
	proxy.RegisterServer("redir6", NewRedir6Server)
}

// NewRedirProxy 返回一个重定向代理。
func NewRedirProxy(s string, p proxy.Proxy, ipv6 bool) (*RedirProxy, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	r := &RedirProxy{
		proxy: p,
		addr:  addr,
		ipv6:  ipv6,
	}

	return r, nil
}

// NewRedirServer 返回一个重定向服务器。
func NewRedirServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewRedirProxy(s, p, false)
}

// NewRedir6Server 返回一个用于 ipv6 的重定向服务器。
func NewRedir6Server(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewRedirProxy(s, p, true)
}

// ListenAndServe 在服务器地址上监听并处理连接。
func (s *RedirProxy) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[redir] failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("[redir] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[redir] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve 处理连接。
func (s *RedirProxy) Serve(cc net.Conn) {
	defer cc.Close()

	c, ok := cc.(*net.TCPConn)
	if !ok {
		log.F("[redir] not a tcp connection, can not chain redir proxy")
		return
	}

	c.SetKeepAlive(true)
	tgtAddr, err := getOrigDst(c, s.ipv6)
	if err != nil {
		log.F("[redir] failed to get target address: %v", err)
		return
	}
	tgt := tgtAddr.String()

	// 循环请求
	if c.LocalAddr().String() == tgt {
		log.F("[redir] %s <-> %s, unallowed request to redir port", c.RemoteAddr(), tgt)
		return
	}

	rc, dialer, err := s.proxy.Dial("tcp", tgt)
	if err != nil {
		log.F("[redir] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[redir] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[redir] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		// 仅记录远程连接失败
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// 获取 TCP 连接的原始目标地址。
func getOrigDst(c *net.TCPConn, ipv6 bool) (netip.AddrPort, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return netip.AddrPort{}, err
	}
	var addr netip.AddrPort
	rc.Control(func(fd uintptr) {
		if ipv6 {
			addr, err = getorigdstIPv6(fd)
		} else {
			addr, err = getorigdst(fd)
		}
	})
	return addr, err
}

// 调用 linux/net/ipv4/netfilter/nf_conntrack_l3proto_ipv4.c 中的 getorigdst()
func getorigdst(fd uintptr) (netip.AddrPort, error) {
	const _SO_ORIGINAL_DST = 80 // from linux/include/uapi/linux/netfilter_ipv4.h
	var raw syscall.RawSockaddrInet4
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IP, _SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return netip.AddrPort{}, err
	}
	// 注意：raw.Port 是大端序，将其转换为小端序
	// TODO: 当添加大端序 $GOARCH 支持时在此处改进
	port := raw.Port<<8 | raw.Port>>8
	return netip.AddrPortFrom(netip.AddrFrom4(raw.Addr), port), nil
}

// 调用 linux/net/ipv6/netfilter/nf_conntrack_l3proto_ipv6.c 中的 ipv6_getorigdst()
func getorigdstIPv6(fd uintptr) (netip.AddrPort, error) {
	const _IP6T_SO_ORIGINAL_DST = 80 // from linux/include/uapi/linux/netfilter_ipv6/ip6_tables.h
	var raw syscall.RawSockaddrInet6
	siz := unsafe.Sizeof(raw)
	if err := socketcall(GETSOCKOPT, fd, syscall.IPPROTO_IPV6, _IP6T_SO_ORIGINAL_DST, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&siz)), 0); err != nil {
		return netip.AddrPort{}, err
	}
	// 注意：raw.Port 是大端序，将其转换为小端序
	// TODO: 当添加大端序 $GOARCH 支持时在此处改进
	port := raw.Port<<8 | raw.Port>>8
	return netip.AddrPortFrom(netip.AddrFrom16(raw.Addr), port), nil
}
