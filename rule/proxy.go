package rule

import (
	"net"
	"net/netip"
	"strings"
	"sync"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// Proxy 实现了带规则支持的 proxy.Proxy 接口。
type Proxy struct {
	main      *FwdrGroup
	all       []*FwdrGroup
	domainMap sync.Map
	ipMap     sync.Map
	cidrMap   sync.Map
}

// NewProxy 返回一个新的规则代理。
func NewProxy(mainForwarders []string, mainStrategy *Strategy, rules []*Config) *Proxy {
	rd := &Proxy{main: NewFwdrGroup("main", mainForwarders, mainStrategy)}

	for _, r := range rules {
		group := NewFwdrGroup(r.RulePath, r.Forward, &r.Strategy)
		rd.all = append(rd.all, group)

		for _, domain := range r.Domain {
			rd.domainMap.Store(strings.ToLower(domain), group)
		}

		for _, s := range r.IP {
			ip, err := netip.ParseAddr(s)
			if err != nil {
				log.F("[rule] parse ip error: %s", err)
				continue
			}
			rd.ipMap.Store(ip, group)
		}

		for _, s := range r.CIDR {
			cidr, err := netip.ParsePrefix(s)
			if err != nil {
				log.F("[rule] parse cidr error: %s", err)
				continue
			}
			rd.cidrMap.Store(cidr, group)
		}
	}

	direct := NewFwdrGroup("", nil, mainStrategy)
	rd.domainMap.Store("direct", direct)

	// 如果主配置中定义了任何转发器，确保它们可以被直连访问。
	if len(mainForwarders) > 0 {
		for _, f := range rd.main.fwdrs {
			addr := strings.Split(f.addr, ",")[0]
			host, _, _ := net.SplitHostPort(addr)
			if _, err := netip.ParseAddr(host); err != nil {
				rd.domainMap.Store(strings.ToLower(host), direct)
			}
		}
	}

	return rd
}

// Dial 拨号到目标地址并返回一个连接。
func (p *Proxy) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	return p.findDialer(addr).Dial(network, addr)
}

// DialUDP 通过代理连接到指定地址。
func (p *Proxy) DialUDP(network, addr string) (pc net.PacketConn, dialer proxy.UDPDialer, err error) {
	return p.findDialer(addr).DialUDP(network, addr)
}

// findDialer 根据规则通过目标地址返回一个拨号器。
func (p *Proxy) findDialer(dstAddr string) *FwdrGroup {
	host, _, err := net.SplitHostPort(dstAddr)
	if err != nil {
		return p.main
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		// 检查 IP
		if proxy, ok := p.ipMap.Load(ip); ok {
			return proxy.(*FwdrGroup)
		}

		// 检查 CIDR
		var ret *FwdrGroup
		p.cidrMap.Range(func(key, value any) bool {
			if key.(netip.Prefix).Contains(ip) {
				ret = value.(*FwdrGroup)
				return false
			}
			return true
		})

		if ret != nil {
			return ret
		}
	}

	// 检查主机名
	host = strings.ToLower(host)
	for i := len(host); i != -1; {
		i = strings.LastIndexByte(host[:i], '.')
		if proxy, ok := p.domainMap.Load(host[i+1:]); ok {
			return proxy.(*FwdrGroup)
		}
	}

	return p.main
}

// NextDialer 根据规则返回下一个拨号器。
func (p *Proxy) NextDialer(dstAddr string) proxy.Dialer {
	return p.findDialer(dstAddr).NextDialer(dstAddr)
}

// Record 记录使用代理中拨号器的结果。
func (p *Proxy) Record(dialer proxy.Dialer, success bool) {
	if fwdr, ok := dialer.(*Forwarder); ok {
		if !success {
			fwdr.IncFailures()
			return
		}
		fwdr.Enable()
	}
}

// AddDomainIP 用于根据 domainMap 规则更新 ipMap 规则。
func (p *Proxy) AddDomainIP(domain string, ip netip.Addr) error {
	domain = strings.ToLower(domain)
	for i := len(domain); i != -1; {
		i = strings.LastIndexByte(domain[:i], '.')
		if dialer, ok := p.domainMap.Load(domain[i+1:]); ok {
			p.ipMap.Store(ip, dialer)
			// log.F("[rule] update map: %s/%s based on rule: domain=%s\n", domain, ip, domain[i+1:])
		}
	}
	return nil
}

// Check 检查代理内转发器的可用性。
func (p *Proxy) Check() {
	p.main.Check()

	for _, fwdrGroup := range p.all {
		fwdrGroup.Check()
	}
}
