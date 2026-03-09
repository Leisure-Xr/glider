package udp

import (
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

var nm sync.Map

func init() {
	proxy.RegisterDialer("udp", NewUDPDialer)
	proxy.RegisterServer("udp", NewUDPServer)
}

// UDP 结构体。
type UDP struct {
	addr   string
	uaddr  *net.UDPAddr
	dialer proxy.Dialer
	proxy  proxy.Proxy
}

// NewUDP 返回一个 udp 结构体。
func NewUDP(s string, d proxy.Dialer, p proxy.Proxy) (*UDP, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[udp] parse url err: %s", err)
		return nil, err
	}

	t := &UDP{
		dialer: d,
		proxy:  p,
		addr:   u.Host,
	}

	t.uaddr, err = net.ResolveUDPAddr("udp", t.addr)
	return t, err
}

// NewUDPDialer 返回一个 udp 拨号器。
func NewUDPDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewUDP(s, d, nil)
}

// NewUDPServer 返回真实服务器前的 udp 传输层。
func NewUDPServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewUDP(s, nil, p)
}

// ListenAndServe 在服务器地址上监听并处理连接。
func (s *UDP) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.Fatalf("[udp] failed to listen on UDP %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("[udp] listening UDP on %s", s.addr)

	for {
		buf := pool.GetBuffer(proxy.UDPBufSize)
		n, srcAddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[udp] read error: %v", err)
			continue
		}

		var sess *session
		sessKey := srcAddr.String()

		v, ok := nm.Load(sessKey)
		if !ok || v == nil {
			sess = newSession(sessKey, srcAddr, c)
			nm.Store(sessKey, sess)
			go s.serveSession(sess)
		} else {
			sess = v.(*session)
		}

		sess.msgCh <- buf[:n]
	}
}

func (s *UDP) serveSession(session *session) {
	// 我们知道这是一个 udp 隧道，所以拨号地址无意义，
	// 这里使用 srcAddr 帮助 unix 客户端识别源套接字。
	dstPC, dialer, err := s.proxy.DialUDP("udp", session.src.String())
	if err != nil {
		log.F("[udp] remote dial error: %v", err)
		nm.Delete(session.key)
		return
	}
	defer dstPC.Close()

	go func() {
		proxy.CopyUDP(session, session.src, dstPC, 2*time.Minute, 5*time.Second)
		nm.Delete(session.key)
		close(session.finCh)
	}()

	log.F("[udp] %s <-> %s", session.src, dialer.Addr())

	for {
		select {
		case p := <-session.msgCh:
			_, err = dstPC.WriteTo(p, nil) // 我们知道这是隧道，所以目标地址可以为 nil
			if err != nil {
				log.F("[udp] writeTo error: %v", err)
			}
			pool.PutBuffer(p)
		case <-session.finCh:
			return
		}
	}
}

type session struct {
	key string
	src *net.UDPAddr
	net.PacketConn
	msgCh chan []byte
	finCh chan struct{}
}

func newSession(key string, src net.Addr, srcPC net.PacketConn) *session {
	srcAddr, _ := net.ResolveUDPAddr("udp", src.String())
	return &session{key, srcAddr, srcPC, make(chan []byte, 32), make(chan struct{})}
}

// Serve 处理一个连接。
func (s *UDP) Serve(c net.Conn) {
	log.F("[udp] func Serve: can not be called directly")
}

// Addr 返回转发器的地址。
func (s *UDP) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到网络 net 上的地址 addr。
func (s *UDP) Dial(network, addr string) (net.Conn, error) {
	return nil, proxy.ErrNotSupported
}

// DialUDP 通过代理连接到给定地址。
func (s *UDP) DialUDP(network, addr string) (net.PacketConn, error) {
	// return s.dialer.DialUDP(network, s.addr)
	pc, err := s.dialer.DialUDP(network, s.addr)
	return &PktConn{pc, s.uaddr}, err
}

// PktConn 数据包连接。
type PktConn struct {
	net.PacketConn
	uaddr *net.UDPAddr
}

// WriteTo 覆盖 net.PacketConn 中的原始函数。
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return pc.PacketConn.WriteTo(b, pc.uaddr)
}
