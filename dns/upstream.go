package dns

import (
	"net"
	"sync/atomic"
)

// UPStream 是 DNS 上游服务器的结构体。
type UPStream struct {
	index   uint32
	servers []string
}

// NewUPStream 返回一个新的 UPStream 实例。
func NewUPStream(servers []string) *UPStream {
	// DNS 上游服务器的默认端口
	for i, server := range servers {
		if _, port, _ := net.SplitHostPort(server); port == "" {
			servers[i] = net.JoinHostPort(server, "53")
		}
	}
	return &UPStream{servers: servers}
}

// Server 返回当前使用的 DNS 服务器地址。
func (u *UPStream) Server() string {
	return u.servers[atomic.LoadUint32(&u.index)%uint32(len(u.servers))]
}

// Switch 切换到下一个 DNS 服务器。
func (u *UPStream) Switch() string {
	return u.servers[atomic.AddUint32(&u.index, 1)%uint32(len(u.servers))]
}

// SwitchIf 在需要时切换到下一个 DNS 服务器。
func (u *UPStream) SwitchIf(server string) string {
	if u.Server() == server {
		return u.Switch()
	}
	return u.Server()
}

// Len 返回 DNS 服务器的数量。
func (u *UPStream) Len() int {
	return len(u.servers)
}
