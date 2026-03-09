package vless

import (
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strconv"

	"github.com/nadoo/glider/pkg/pool"
)

// Atyp 是 vless 地址类型。
type Atyp byte

// Atyp 地址类型常量。
const (
	AtypErr    Atyp = 0
	AtypIP4    Atyp = 1
	AtypDomain Atyp = 2
	AtypIP6    Atyp = 3
)

// Addr 是 vless 地址。
type Addr []byte

// MaxHostLen 是主机名的最大字节长度。
const MaxHostLen = 255

// Port 是 vless 地址端口。
type Port uint16

// ParseAddr 解析字符串 s 中的地址。
func ParseAddr(s string) (Atyp, Addr, Port, error) {
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return 0, nil, 0, err
	}

	var addr Addr
	var atyp Atyp = AtypIP4
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Is6() {
			atyp = AtypIP6
		}
		addr = ip.AsSlice()
	} else {
		if len(host) > MaxHostLen {
			return 0, nil, 0, err
		}
		addr = make([]byte, 1+len(host))
		atyp = AtypDomain
		addr[0] = byte(len(host))
		copy(addr[1:], host)
	}

	portnum, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0, nil, 0, err
	}

	return atyp, addr, Port(portnum), err
}

// ReadAddr 从 r 中读取足够的字节以获取地址。
func ReadAddr(r io.Reader) (atyp Atyp, host Addr, port Port, err error) {
	buf := pool.GetBuffer(2)
	defer pool.PutBuffer(buf)

	// 端口
	_, err = io.ReadFull(r, buf[:2])
	if err != nil {
		return
	}
	port = Port(binary.BigEndian.Uint16(buf[:2]))

	// 地址类型
	_, err = io.ReadFull(r, buf[:1])
	if err != nil {
		return
	}
	atyp = Atyp(buf[0])

	switch atyp {
	case AtypIP4:
		host = make([]byte, net.IPv4len)
		_, err = io.ReadFull(r, host)
		return
	case AtypIP6:
		host = make([]byte, net.IPv6len)
		_, err = io.ReadFull(r, host)
		return
	case AtypDomain:
		_, err = io.ReadFull(r, buf[:1])
		if err != nil {
			return
		}
		host = make([]byte, int(buf[0]))
		_, err = io.ReadFull(r, host)
		return
	}

	return
}

// ReadAddrString 从 r 中读取足够的字节以获取地址字符串。
func ReadAddrString(r io.Reader) (string, error) {
	atyp, host, port, err := ReadAddr(r)
	if err != nil {
		return "", err
	}
	return AddrString(atyp, host, port), nil
}

// AddrString 返回格式为 "host:port" 的地址字符串。
func AddrString(atyp Atyp, addr Addr, port Port) string {
	var host string

	switch atyp {
	case AtypIP4, AtypIP6:
		host = net.IP(addr).String()
	case AtypDomain:
		host = string(addr)
	}

	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}
