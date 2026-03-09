package rule

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// StatusHandler 是转发器状态变更时调用的回调函数类型。
type StatusHandler func(*Forwarder)

// Forwarder 对应一个 `-forward` 参数，通常是一个拨号器或代理链。
type Forwarder struct {
	proxy.Dialer
	url         string
	addr        string
	priority    uint32
	maxFailures uint32 // 达到此失败次数后禁用转发器
	disabled    uint32
	failures    uint32
	latency     int64
	intface     string // 本地网络接口或 IP 地址
	handlers    []StatusHandler
}

// ForwarderFromURL 解析 `forward=` 参数值并返回一个新的转发器。
func ForwarderFromURL(s, intface string, dialTimeout, relayTimeout time.Duration) (f *Forwarder, err error) {
	f = &Forwarder{url: s}

	ss := strings.Split(s, "#")
	if len(ss) > 1 {
		err = f.parseOption(ss[1])
	}

	iface := intface
	if f.intface != "" && f.intface != intface {
		iface = f.intface
	}

	var d proxy.Dialer
	d, err = proxy.NewDirect(iface, dialTimeout, relayTimeout)
	if err != nil {
		return nil, err
	}

	var addrs []string
	for _, url := range strings.Split(ss[0], ",") {
		d, err = proxy.DialerFromURL(url, d)
		if err != nil {
			return nil, err
		}
		cnt := len(addrs)
		if cnt == 0 ||
			(cnt > 0 && d.Addr() != addrs[cnt-1]) {
			addrs = append(addrs, d.Addr())
		}
	}

	f.Dialer = d
	f.addr = d.Addr()

	if len(addrs) > 0 {
		f.addr = strings.Join(addrs, ",")
	}

	// 默认将转发器设为禁用状态
	f.Disable()

	return f, err
}

// DirectForwarder 返回一个直连转发器。
func DirectForwarder(intface string, dialTimeout, relayTimeout time.Duration) (*Forwarder, error) {
	d, err := proxy.NewDirect(intface, dialTimeout, relayTimeout)
	if err != nil {
		return nil, err
	}
	return &Forwarder{Dialer: d, addr: d.Addr()}, nil
}

func (f *Forwarder) parseOption(option string) error {
	query, err := url.ParseQuery(option)
	if err != nil {
		return err
	}

	var priority uint64
	p := query.Get("priority")
	if p != "" {
		priority, err = strconv.ParseUint(p, 10, 32)
	}
	f.SetPriority(uint32(priority))

	f.intface = query.Get("interface")

	return err
}

// Addr 返回转发器的地址。
// 注意：代理链的地址格式为：dialer1Addr,dialer2Addr,...
func (f *Forwarder) Addr() string {
	return f.addr
}

// URL 返回转发器的完整 URL。
func (f *Forwarder) URL() string {
	return f.url
}

// Dial 拨号连接到 addr 并返回连接。
func (f *Forwarder) Dial(network, addr string) (c net.Conn, err error) {
	c, err = f.Dialer.Dial(network, addr)
	if err != nil {
		f.IncFailures()
	}
	return c, err
}

// Failures 返回转发器的失败次数。
func (f *Forwarder) Failures() uint32 {
	return atomic.LoadUint32(&f.failures)
}

// IncFailures 将失败次数加 1。
func (f *Forwarder) IncFailures() {
	failures := atomic.AddUint32(&f.failures, 1)
	if f.MaxFailures() == 0 {
		return
	}

	// log.F("[forwarder] %s(%d) recorded %d failures, maxfailures: %d", f.addr, f.Priority(), failures, f.MaxFailures())

	if failures == f.MaxFailures() && f.Enabled() {
		log.F("[forwarder] %s(%d) reaches maxfailures: %d", f.addr, f.Priority(), f.MaxFailures())
		f.Disable()
	}
}

// AddHandler 添加自定义处理器以处理状态变更事件。
func (f *Forwarder) AddHandler(h StatusHandler) {
	f.handlers = append(f.handlers, h)
}

// Enable 启用转发器。
func (f *Forwarder) Enable() {
	if atomic.CompareAndSwapUint32(&f.disabled, 1, 0) {
		for _, h := range f.handlers {
			h(f)
		}
	}
	atomic.StoreUint32(&f.failures, 0)
}

// Disable 禁用转发器。
func (f *Forwarder) Disable() {
	if atomic.CompareAndSwapUint32(&f.disabled, 0, 1) {
		for _, h := range f.handlers {
			h(f)
		}
	}
}

// Enabled 返回转发器的启用状态。
func (f *Forwarder) Enabled() bool {
	return !isTrue(atomic.LoadUint32(&f.disabled))
}

func isTrue(n uint32) bool {
	return n&1 == 1
}

// Priority 返回转发器的优先级。
func (f *Forwarder) Priority() uint32 {
	return atomic.LoadUint32(&f.priority)
}

// SetPriority 设置转发器的优先级。
func (f *Forwarder) SetPriority(l uint32) {
	atomic.StoreUint32(&f.priority, l)
}

// MaxFailures 返回转发器的最大失败次数。
func (f *Forwarder) MaxFailures() uint32 {
	return atomic.LoadUint32(&f.maxFailures)
}

// SetMaxFailures 设置转发器的最大失败次数。
func (f *Forwarder) SetMaxFailures(l uint32) {
	atomic.StoreUint32(&f.maxFailures, l)
}

// Latency 返回转发器的延迟。
func (f *Forwarder) Latency() int64 {
	return atomic.LoadInt64(&f.latency)
}

// SetLatency 设置转发器的延迟。
func (f *Forwarder) SetLatency(l int64) {
	atomic.StoreInt64(&f.latency, l)
}
