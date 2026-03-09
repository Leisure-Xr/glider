package rule

import (
	"errors"
	"hash/fnv"
	"net"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// 按优先级排序的转发器切片。
type priSlice []*Forwarder

func (p priSlice) Len() int           { return len(p) }
func (p priSlice) Less(i, j int) bool { return p[i].Priority() > p[j].Priority() }
func (p priSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// FwdrGroup 是一个转发器组。
type FwdrGroup struct {
	name     string
	config   *Strategy
	fwdrs    priSlice
	avail    []*Forwarder // 可用的转发器
	mu       sync.RWMutex
	index    uint32
	priority uint32
	next     func(addr string) *Forwarder
}

// NewFwdrGroup 返回一个新的转发器组。
func NewFwdrGroup(rulePath string, s []string, c *Strategy) *FwdrGroup {
	var fwdrs []*Forwarder
	for _, chain := range s {
		fwdr, err := ForwarderFromURL(chain, c.IntFace,
			time.Duration(c.DialTimeout)*time.Second, time.Duration(c.RelayTimeout)*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		fwdr.SetMaxFailures(uint32(c.MaxFailures))
		fwdrs = append(fwdrs, fwdr)
	}

	if len(fwdrs) == 0 {
		// 直连转发器
		direct, err := DirectForwarder(c.IntFace,
			time.Duration(c.DialTimeout)*time.Second, time.Duration(c.RelayTimeout)*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		fwdrs = append(fwdrs, direct)
		c.Strategy = "rr"
	}

	name := strings.TrimSuffix(filepath.Base(rulePath), filepath.Ext(rulePath))
	return newFwdrGroup(name, fwdrs, c)
}

// newFwdrGroup 返回一个新的 FwdrGroup。
func newFwdrGroup(name string, fwdrs []*Forwarder, c *Strategy) *FwdrGroup {
	p := &FwdrGroup{name: name, fwdrs: fwdrs, config: c}
	sort.Sort(p.fwdrs)

	p.init()

	// 默认调度器
	p.next = p.scheduleRR

	// 如果有超过1个转发器，则需要考虑策略。
	if count := len(fwdrs); count > 1 {
		switch c.Strategy {
		case "rr":
			p.next = p.scheduleRR
			log.F("[strategy] %s: %d forwarders forward in round robin mode.", name, count)
		case "ha":
			p.next = p.scheduleHA
			log.F("[strategy] %s: %d forwarders forward in high availability mode.", name, count)
		case "lha":
			p.next = p.scheduleLHA
			log.F("[strategy] %s: %d forwarders forward in latency based high availability mode.", name, count)
		case "dh":
			p.next = p.scheduleDH
			log.F("[strategy] %s: %d forwarders forward in destination hashing mode.", name, count)
		default:
			p.next = p.scheduleRR
			log.F("[strategy] %s: not supported forward mode '%s', use round robin mode for %d forwarders.", name, c.Strategy, count)
		}
	}

	for _, f := range fwdrs {
		f.AddHandler(p.onStatusChanged)
	}

	return p
}

// Dial 连接到网络 net 上的地址 addr。
func (p *FwdrGroup) Dial(network, addr string) (net.Conn, proxy.Dialer, error) {
	nd := p.NextDialer(addr)
	c, err := nd.Dial(network, addr)
	return c, nd, err
}

// DialUDP 连接到指定地址。
func (p *FwdrGroup) DialUDP(network, addr string) (pc net.PacketConn, dialer proxy.UDPDialer, err error) {
	nd := p.NextDialer(addr)
	pc, err = nd.DialUDP(network, addr)
	return pc, nd, err
}

// NextDialer 返回下一个拨号器。
func (p *FwdrGroup) NextDialer(dstAddr string) proxy.Dialer {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.avail) == 0 {
		return p.fwdrs[atomic.AddUint32(&p.index, 1)%uint32(len(p.fwdrs))]
	}

	return p.next(dstAddr)
}

// Priority 返回拨号器的当前优先级。
func (p *FwdrGroup) Priority() uint32 { return atomic.LoadUint32(&p.priority) }

// SetPriority 设置拨号器的当前优先级。
func (p *FwdrGroup) SetPriority(pri uint32) { atomic.StoreUint32(&p.priority, pri) }

// init 遍历 d.fwdrs 并初始化可用转发器切片。
func (p *FwdrGroup) init() {
	for _, f := range p.fwdrs {
		if f.Enabled() {
			p.SetPriority(f.Priority())
			break
		}
	}

	p.avail = nil
	for _, f := range p.fwdrs {
		if f.Enabled() && f.Priority() >= p.Priority() {
			p.avail = append(p.avail, f)
		}
	}

	if len(p.avail) == 0 {
		// 没有可用的转发器，将优先级设为0以在检查函数中检查所有转发器
		p.SetPriority(0)
		// log.F("[group] no available forwarders, please check your config file or network settings")
	}
}

// onStatusChanged 当转发器状态改变时被调用。
func (p *FwdrGroup) onStatusChanged(fwdr *Forwarder) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if fwdr.Enabled() {
		if fwdr.Priority() == p.Priority() {
			p.avail = append(p.avail, fwdr)
		} else if fwdr.Priority() > p.Priority() {
			p.init()
		}
		log.F("[group] %s: %s(%d) changed status from DISABLED to ENABLED (%d of %d currently enabled)",
			p.name, fwdr.Addr(), fwdr.Priority(), len(p.avail), len(p.fwdrs))
	} else {
		for i, f := range p.avail {
			if f == fwdr {
				p.avail[i], p.avail = p.avail[len(p.avail)-1], p.avail[:len(p.avail)-1]
				break
			}
		}
		log.F("[group] %s: %s(%d) changed status from ENABLED to DISABLED (%d of %d currently enabled)",
			p.name, fwdr.Addr(), fwdr.Priority(), len(p.avail), len(p.fwdrs))
	}

	if len(p.avail) == 0 {
		p.init()
	}
}

// Check 运行转发器的健康检查。
func (p *FwdrGroup) Check() {
	if len(p.fwdrs) == 1 {
		log.F("[group] %s: only 1 forwarder found, disable health checking", p.name)
		return
	}

	if !strings.Contains(p.config.Check, "://") {
		p.config.Check += "://"
	}

	u, err := url.Parse(p.config.Check)
	if err != nil {
		log.F("[group] %s: parse check config error: %s, disable health checking", p.name, err)
		return
	}

	addr := u.Host
	timeout := time.Duration(p.config.CheckTimeout) * time.Second

	var checker Checker
	switch u.Scheme {
	case "tcp":
		checker = newTcpChecker(addr, timeout)
	case "http", "https":
		expect := "HTTP" // 默认：检查响应的前4个字符
		params, _ := url.ParseQuery(u.Fragment)
		if ex := params.Get("expect"); ex != "" {
			expect = ex
		}
		checker = newHttpChecker(addr, u.RequestURI(), expect, timeout, u.Scheme == "https")
	case "file":
		checker = newFileChecker(u.Host + u.Path)
	default:
		log.F("[group] %s: unknown scheme in check config `%s`, disable health checking", p.name, p.config.Check)
		return
	}

	log.F("[group] %s: using check config: %s", p.name, p.config.Check)

	for i := range p.fwdrs {
		go p.check(p.fwdrs[i], checker)
	}
}

func (p *FwdrGroup) check(fwdr *Forwarder, checker Checker) {
	wait := uint8(0)
	intval := time.Duration(p.config.CheckInterval) * time.Second

	for {
		time.Sleep(intval * time.Duration(wait))

		// 至少检查所有转发器一次
		if wait > 0 && (fwdr.Priority() < p.Priority()) {
			continue
		}

		if fwdr.Enabled() && p.config.CheckDisabledOnly {
			continue
		}

		elapsed, err := checker.Check(fwdr)
		if err != nil {
			if errors.Is(err, proxy.ErrNotSupported) {
				fwdr.SetMaxFailures(0)
				log.F("[check] %s: %s(%d), %s, stop checking", p.name, fwdr.Addr(), fwdr.Priority(), err)
				fwdr.Enable()
				break
			}

			wait++
			if wait > 16 {
				wait = 16
			}

			log.F("[check] %s: %s(%d), FAILED. error: %s", p.name, fwdr.Addr(), fwdr.Priority(), err)
			fwdr.Disable()
			continue
		}

		wait = 1
		p.setLatency(fwdr, elapsed)
		log.F("[check] %s: %s(%d), SUCCESS. Elapsed: %dms, Latency: %dms.",
			p.name, fwdr.Addr(), fwdr.Priority(), elapsed.Milliseconds(), time.Duration(fwdr.Latency()).Milliseconds())
		fwdr.Enable()
	}
}

func (p *FwdrGroup) setLatency(fwdr *Forwarder, elapsed time.Duration) {
	newLatency := int64(elapsed)
	if cnt := p.config.CheckLatencySamples; cnt > 1 {
		if lastLatency := fwdr.Latency(); lastLatency > 0 {
			newLatency = (lastLatency*(int64(cnt)-1) + int64(elapsed)) / int64(cnt)
		}
	}
	fwdr.SetLatency(newLatency)
}

// 轮询。
func (p *FwdrGroup) scheduleRR(dstAddr string) *Forwarder {
	return p.avail[atomic.AddUint32(&p.index, 1)%uint32(len(p.avail))]
}

// 高可用。
func (p *FwdrGroup) scheduleHA(dstAddr string) *Forwarder {
	return p.avail[0]
}

// 基于延迟的高可用。
func (p *FwdrGroup) scheduleLHA(dstAddr string) *Forwarder {
	oldfwdr, newfwdr := p.avail[0], p.avail[0]
	lowest := oldfwdr.Latency()
	for _, f := range p.avail {
		if f.Latency() < lowest {
			lowest = f.Latency()
			newfwdr = f
		}
	}
	tolerance := int64(p.config.CheckTolerance) * int64(time.Millisecond)
	if newfwdr.Latency() < (oldfwdr.Latency() - tolerance) {
		return newfwdr
	}
	return oldfwdr
}

// 目标哈希。
func (p *FwdrGroup) scheduleDH(dstAddr string) *Forwarder {
	fnv1a := fnv.New32a()
	fnv1a.Write([]byte(dstAddr))
	return p.avail[fnv1a.Sum32()%uint32(len(p.avail))]
}
