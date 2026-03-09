//go:build !linux

package ipset

import (
	"errors"
	"net/netip"

	"github.com/nadoo/glider/rule"
)

// Manager 是 ipset 管理器的结构体。
type Manager struct{}

// NewManager 返回一个新的 Manager 实例。
func NewManager(rules []*rule.Config) (*Manager, error) {
	return nil, errors.New("ipset not supported on this os")
}

// AddDomainIP 实现了 DNSAnswerHandler 接口。
func (m *Manager) AddDomainIP(domain string, ip netip.Addr) error {
	return errors.New("ipset not supported on this os")
}
