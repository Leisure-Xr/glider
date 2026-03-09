package proxy

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/nadoo/glider/pkg/sockopt"
)

// Direct 直连代理。
type Direct struct {
	iface        *net.Interface // 用户指定的网络接口
	ip           net.IP
	dialTimeout  time.Duration
	relayTimeout time.Duration
}

func init() {
	RegisterDialer("direct", NewDirectDialer)
}

// NewDirect 返回一个 Direct 拨号器。
func NewDirect(intface string, dialTimeout, relayTimeout time.Duration) (*Direct, error) {
	d := &Direct{dialTimeout: dialTimeout, relayTimeout: relayTimeout}

	if intface != "" {
		if addr, err := netip.ParseAddr(intface); err == nil {
			d.ip = addr.AsSlice()
		} else {
			iface, err := net.InterfaceByName(intface)
			if err != nil {
				return nil, errors.New(err.Error() + ": " + intface)
			}
			d.iface = iface
		}
	}

	return d, nil
}

// NewDirectDialer 返回一个直连拨号器。
func NewDirectDialer(s string, d Dialer) (Dialer, error) {
	if d == nil {
		return NewDirect("", time.Duration(3)*time.Second, time.Duration(3)*time.Second)
	}
	return d, nil
}

// Addr 返回转发器的地址。
func (d *Direct) Addr() string { return "DIRECT" }

// Dial 连接到网络 net 上的地址 addr
func (d *Direct) Dial(network, addr string) (c net.Conn, err error) {
	if d.iface == nil || d.ip != nil {
		c, err = d.dial(network, addr, d.ip)
		if err == nil {
			return
		}
	}

	for _, ip := range d.IFaceIPs() {
		c, err = d.dial(network, addr, ip)
		if err == nil {
			d.ip = ip
			break
		}
	}

	// 无可用 IP（未发起任何拨号），可能是网络接口链路已断开
	if c == nil && err == nil {
		err = errors.New("dial failed, maybe the interface link is down, please check it")
	}

	return c, err
}

func (d *Direct) dial(network, addr string, localIP net.IP) (net.Conn, error) {
	var la net.Addr
	switch network {
	case "tcp":
		la = &net.TCPAddr{IP: localIP}
	case "udp":
		la = &net.UDPAddr{IP: localIP}
	}

	dialer := &net.Dialer{LocalAddr: la, Timeout: d.dialTimeout}
	if d.iface != nil {
		dialer.Control = sockopt.Control(sockopt.Bind(d.iface))
	}

	c, err := dialer.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	if d.relayTimeout > 0 {
		c.SetDeadline(time.Now().Add(d.relayTimeout))
	}

	return c, err
}

// DialUDP 连接到给定地址（UDP）。
func (d *Direct) DialUDP(network, addr string) (net.PacketConn, error) {
	var la string
	if d.ip != nil {
		la = net.JoinHostPort(d.ip.String(), "0")
	}

	lc := &net.ListenConfig{}
	if d.iface != nil {
		lc.Control = sockopt.Control(sockopt.Bind(d.iface))
	}

	return lc.ListenPacket(context.Background(), network, la)
}

// IFaceIPs 返回指定网络接口的 IP 地址列表。
func (d *Direct) IFaceIPs() (ips []net.IP) {
	ipNets, err := d.iface.Addrs()
	if err != nil {
		return
	}
	for _, ipNet := range ipNets {
		ips = append(ips, ipNet.(*net.IPNet).IP) //!ip.IsLinkLocalUnicast()
	}
	return
}

func init() {
	AddUsage("direct", `
Direct（直连）方案：
  direct://

仅在需要指定出口网络接口时使用：
  glider -verbose -listen :8443 -forward direct://#interface=eth0

或直接对多个接口进行负载均衡：
  glider -verbose -listen :8443 -forward direct://#interface=eth0 -forward direct://#interface=eth1 -strategy rr

或使用高可用模式：
  glider -verbose -listen :8443 -forward direct://#interface=eth0&priority=100 -forward direct://#interface=eth1&priority=200 -strategy ha
`)
}
