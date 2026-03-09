// Package reject 实现了一个始终拒绝所有请求的虚拟代理。
package reject

import (
	"errors"
	"net"

	"github.com/nadoo/glider/proxy"
)

// Reject 是 reject 代理的基础结构体。
type Reject struct{}

func init() {
	proxy.RegisterDialer("reject", NewRejectDialer)
}

// NewReject 返回一个 reject 代理，格式：reject://。
func NewReject(s string, d proxy.Dialer) (*Reject, error) {
	return &Reject{}, nil
}

// NewRejectDialer 返回一个 reject 代理拨号器。
func NewRejectDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewReject(s, d)
}

// Addr 返回转发器地址。
func (s *Reject) Addr() string { return "REJECT" }

// Dial 通过代理连接到指定网络地址。
func (s *Reject) Dial(network, addr string) (net.Conn, error) {
	return nil, errors.New("REJECT")
}

// DialUDP 通过代理连接到指定地址（UDP）。
func (s *Reject) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, errors.New("REJECT")
}

func init() {
	proxy.AddUsage("reject", `
Reject（拒绝）方案：
  reject://
`)
}
