package vmess

import (
	"net"
	"net/netip"
	"strconv"
)

// Atyp 是 vmess 地址类型。
type Atyp byte

// Atyp 地址类型常量。
const (
	AtypErr    Atyp = 0
	AtypIP4    Atyp = 1
	AtypDomain Atyp = 2
	AtypIP6    Atyp = 3
)

// Addr 是 vmess 地址。
type Addr []byte

// MaxHostLen 是主机名的最大字节长度。
const MaxHostLen = 255

// Port 是 vmess 地址端口。
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
