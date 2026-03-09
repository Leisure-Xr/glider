//go:build linux

package dhcpd

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Pool 是一个 DHCP 地址池。
type Pool struct {
	items []*item
	mutex sync.RWMutex
	lease time.Duration
}

type item struct {
	ip     netip.Addr
	mac    net.HardwareAddr
	expire time.Time
}

// NewPool 返回一个新的 DHCP IP 地址池。
func NewPool(lease time.Duration, start, end netip.Addr) (*Pool, error) {
	if start.IsUnspecified() || end.IsUnspecified() || start.Is6() || end.Is6() {
		return nil, errors.New("start ip or end ip is wrong/nil, please check your config, note only ipv4 is supported")
	}

	s, e := ipv4ToNum(start), ipv4ToNum(end)
	if e < s {
		return nil, errors.New("start ip larger than end ip")
	}

	items := make([]*item, 0, e-s+1)
	for n := s; n <= e; n++ {
		items = append(items, &item{ip: numToIPv4(n)})
	}

	p := &Pool{items: items, lease: lease}
	go func() {
		for now := range time.Tick(time.Second) {
			p.mutex.Lock()
			for i := range len(items) {
				if !items[i].expire.IsZero() && now.After(items[i].expire) {
					items[i].mac = nil
					items[i].expire = time.Time{}
				}
			}
			p.mutex.Unlock()
		}
	}()

	return p, nil
}

// LeaseIP 从 DHCP 地址池中将一个 IP 地址租给指定 MAC 地址。
func (p *Pool) LeaseIP(mac net.HardwareAddr, ip netip.Addr) (netip.Addr, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 静态 IP 及已租用的 IP
	for _, item := range p.items {
		if bytes.Equal(mac, item.mac) {
			if !item.expire.IsZero() {
				item.expire = time.Now().Add(p.lease)
			}
			return item.ip, nil
		}
	}

	// 请求指定的 IP
	for _, item := range p.items {
		if item.ip == ip && item.mac == nil {
			item.mac = mac
			item.expire = time.Now().Add(p.lease)
			return item.ip, nil
		}
	}

	// 租用新的 IP
	idx := rand.IntN(len(p.items))
	for _, item := range p.items[idx:] {
		if item.mac == nil {
			item.mac = mac
			item.expire = time.Now().Add(p.lease)
			return item.ip, nil
		}
	}

	for _, item := range p.items {
		if item.mac == nil {
			item.mac = mac
			item.expire = time.Now().Add(p.lease)
			return item.ip, nil
		}
	}

	return netip.Addr{}, errors.New("no more ip can be leased")
}

// LeaseStaticIP 根据给定的 MAC 地址从地址池中租用静态 IP。
func (p *Pool) LeaseStaticIP(mac net.HardwareAddr, ip netip.Addr) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, item := range p.items {
		if item.ip == ip {
			item.mac = mac
			item.expire = time.Time{}
		}
	}
}

// ReleaseIP 根据给定的 MAC 地址将 IP 地址释放回地址池。
func (p *Pool) ReleaseIP(mac net.HardwareAddr) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, item := range p.items {
		// 非静态 IP
		if !item.expire.IsZero() && bytes.Equal(mac, item.mac) {
			item.mac = nil
			item.expire = time.Time{}
		}
	}
}

func ipv4ToNum(addr netip.Addr) uint32 {
	ip := addr.AsSlice()
	n := uint32(ip[0])<<24 + uint32(ip[1])<<16
	return n + uint32(ip[2])<<8 + uint32(ip[3])
}

func numToIPv4(n uint32) netip.Addr {
	ip := [4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	return netip.AddrFrom4(ip)
}
