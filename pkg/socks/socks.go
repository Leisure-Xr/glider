package socks

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
)

// SOCKS 认证类型
const (
	AuthNone     = 0
	AuthPassword = 2
)

// SOCKS 请求命令，定义于 RFC 1928 第 4 节
const (
	CmdError        byte = 0
	CmdConnect      byte = 1
	CmdBind         byte = 2
	CmdUDPAssociate byte = 3
)

// SOCKS 地址类型，定义于 RFC 1928 第 5 节
const (
	ATypIP4    = 1
	ATypDomain = 3
	ATypIP6    = 4
)

// MaxAddrLen 是 SOCKS 地址的最大字节长度
const MaxAddrLen = 1 + 1 + 255 + 2

// Errors 是 socks5 错误列表
var Errors = []error{
	errors.New(""),
	errors.New("general failure"),
	errors.New("connection forbidden"),
	errors.New("network unreachable"),
	errors.New("host unreachable"),
	errors.New("connection refused"),
	errors.New("TTL expired"),
	errors.New("command not supported"),
	errors.New("address type not supported"),
	errors.New("socks5UDPAssociate"),
}

// Addr 表示 RFC 1928 第 5 节中定义的 SOCKS 地址。
type Addr []byte

// String 将 SOCKS 地址 a 序列化为字符串形式。
func (a Addr) String() string {
	var host, port string

	switch a[0] { // 地址类型
	case ATypDomain:
		host = string(a[2 : 2+int(a[1])])
		port = strconv.Itoa((int(a[2+int(a[1])]) << 8) | int(a[2+int(a[1])+1]))
	case ATypIP4:
		host = net.IP(a[1 : 1+net.IPv4len]).String()
		port = strconv.Itoa((int(a[1+net.IPv4len]) << 8) | int(a[1+net.IPv4len+1]))
	case ATypIP6:
		host = net.IP(a[1 : 1+net.IPv6len]).String()
		port = strconv.Itoa((int(a[1+net.IPv6len]) << 8) | int(a[1+net.IPv6len+1]))
	}

	return net.JoinHostPort(host, port)
}

// Network 返回网络名称。实现 net.Addr 接口。
func (a Addr) Network() string { return "socks" }

// ReadAddr 从 r 中读取恰好足够的字节以获得一个有效的 Addr。
func ReadAddr(r io.Reader) (Addr, error) {
	b := make([]byte, MaxAddrLen)
	_, err := io.ReadFull(r, b[:1]) // 读取第 1 个字节以获取地址类型
	if err != nil {
		return nil, err
	}

	switch b[0] {
	case ATypDomain:
		_, err = io.ReadFull(r, b[1:2]) // 读取第 2 个字节以获取域名长度
		if err != nil {
			return nil, err
		}
		_, err = io.ReadFull(r, b[2:2+int(b[1])+2])
		return b[:1+1+int(b[1])+2], err
	case ATypIP4:
		_, err = io.ReadFull(r, b[1:1+net.IPv4len+2])
		return b[:1+net.IPv4len+2], err
	case ATypIP6:
		_, err = io.ReadFull(r, b[1:1+net.IPv6len+2])
		return b[:1+net.IPv6len+2], err
	}

	return nil, Errors[8]
}

// SplitAddr 从 b 的开头截取一个 SOCKS 地址。失败时返回 nil。
func SplitAddr(b []byte) Addr {
	addrLen := 1
	if len(b) < addrLen {
		return nil
	}

	switch b[0] {
	case ATypDomain:
		if len(b) < 2 {
			return nil
		}
		addrLen = 1 + 1 + int(b[1]) + 2
	case ATypIP4:
		addrLen = 1 + net.IPv4len + 2
	case ATypIP6:
		addrLen = 1 + net.IPv6len + 2
	default:
		return nil
	}

	if len(b) < addrLen {
		return nil
	}

	return b[:addrLen]
}

// ParseAddr 解析字符串 s 中的地址。失败时返回 nil。
func ParseAddr(s string) Addr {
	var addr Addr
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return nil
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Is4() {
			addr = make([]byte, 1+net.IPv4len+2)
			addr[0] = ATypIP4
		} else {
			addr = make([]byte, 1+net.IPv6len+2)
			addr[0] = ATypIP6
		}
		copy(addr[1:], ip.AsSlice())
	} else {
		if len(host) > 255 {
			return nil
		}
		addr = make([]byte, 1+1+len(host)+2)
		addr[0] = ATypDomain
		addr[1] = byte(len(host))
		copy(addr[2:], host)
	}

	portnum, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil
	}

	addr[len(addr)-2], addr[len(addr)-1] = byte(portnum>>8), byte(portnum)

	return addr
}
