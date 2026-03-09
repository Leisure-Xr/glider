package kcp

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	kcp "github.com/xtaci/kcp-go/v5"
	"golang.org/x/crypto/pbkdf2"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// KCP 结构体。
type KCP struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string

	key   string
	crypt string
	block kcp.BlockCrypt
	mode  string

	dataShards   int
	parityShards int

	server proxy.Server
}

func init() {
	proxy.RegisterDialer("kcp", NewKCPDialer)
	proxy.RegisterServer("kcp", NewKCPServer)
}

// NewKCP 返回一个 KCP 代理结构体。
func NewKCP(s string, d proxy.Dialer, p proxy.Proxy) (*KCP, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[kcp] parse url err: %s", err)
		return nil, err
	}

	addr := u.Host
	crypt := u.User.Username()
	key, _ := u.User.Password()

	query := u.Query()

	// 数据分片
	dShards := query.Get("dataShards")
	if dShards == "" {
		dShards = "10"
	}

	dataShards, err := strconv.ParseUint(dShards, 10, 32)
	if err != nil {
		log.F("[kcp] parse dataShards err: %s", err)
		return nil, err
	}

	// 奇偶校验分片
	pShards := query.Get("parityShards")
	if pShards == "" {
		pShards = "3"
	}

	parityShards, err := strconv.ParseUint(pShards, 10, 32)
	if err != nil {
		log.F("[kcp] parse parityShards err: %s", err)
		return nil, err
	}

	k := &KCP{
		dialer:       d,
		proxy:        p,
		addr:         addr,
		key:          key,
		crypt:        crypt,
		mode:         query.Get("mode"),
		dataShards:   int(dataShards),
		parityShards: int(parityShards),
	}

	if k.crypt != "" {
		k.block, err = block(k.crypt, k.key)
		if err != nil {
			return nil, fmt.Errorf("[kcp] error: %s", err)
		}
	}

	if k.mode == "" {
		k.mode = "fast"
	}

	return k, nil
}

func block(crypt, key string) (block kcp.BlockCrypt, err error) {
	pass := pbkdf2.Key([]byte(key), []byte("kcp-go"), 4096, 32, sha1.New)
	switch crypt {
	case "sm4":
		block, _ = kcp.NewSM4BlockCrypt(pass[:16])
	case "tea":
		block, _ = kcp.NewTEABlockCrypt(pass[:16])
	case "xor":
		block, _ = kcp.NewSimpleXORBlockCrypt(pass)
	case "none":
		block, _ = kcp.NewNoneBlockCrypt(pass)
	case "aes":
		block, _ = kcp.NewAESBlockCrypt(pass)
	case "aes-128":
		block, _ = kcp.NewAESBlockCrypt(pass[:16])
	case "aes-192":
		block, _ = kcp.NewAESBlockCrypt(pass[:24])
	case "blowfish":
		block, _ = kcp.NewBlowfishBlockCrypt(pass)
	case "twofish":
		block, _ = kcp.NewTwofishBlockCrypt(pass)
	case "cast5":
		block, _ = kcp.NewCast5BlockCrypt(pass[:16])
	case "3des":
		block, _ = kcp.NewTripleDESBlockCrypt(pass[:24])
	case "xtea":
		block, _ = kcp.NewXTEABlockCrypt(pass[:16])
	case "salsa20":
		block, _ = kcp.NewSalsa20BlockCrypt(pass)
	default:
		err = errors.New("unknown crypt type '" + crypt + "'")
	}
	return block, err
}

// NewKCPDialer 返回一个 KCP 代理拨号器。
func NewKCPDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewKCP(s, d, nil)
}

// NewKCPServer 返回一个 KCP 代理服务端。
func NewKCPServer(s string, p proxy.Proxy) (proxy.Server, error) {
	schemes := strings.SplitN(s, ",", 2)
	k, err := NewKCP(schemes[0], nil, p)
	if err != nil {
		return nil, err
	}

	if len(schemes) > 1 {
		k.server, err = proxy.ServerFromURL(schemes[1], p)
		if err != nil {
			return nil, err
		}
	}

	return k, nil
}

// ListenAndServe 在服务端地址上监听并处理连接。
func (s *KCP) ListenAndServe() {
	l, err := kcp.ListenWithOptions(s.addr, s.block, s.dataShards, s.parityShards)
	if err != nil {
		log.Fatalf("[kcp] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[kcp] listening on %s", s.addr)

	for {
		c, err := l.AcceptKCP()
		if err != nil {
			log.F("[kcp] failed to accept: %v", err)
			continue
		}

		s.setParams(c)

		go s.Serve(c)
	}
}

// Serve 处理连接请求。
func (s *KCP) Serve(c net.Conn) {
	if s.server != nil {
		s.server.Serve(c)
		return
	}

	defer c.Close()

	rc, dialer, err := s.proxy.Dial("tcp", "")
	if err != nil {
		log.F("[kcp] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}

	defer rc.Close()

	log.F("[kcp] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[kcp] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// 仅记录远端连接失败
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// Addr 返回转发器的地址。
func (s *KCP) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到网络 net 上的地址 addr。
func (s *KCP) Dial(network, addr string) (net.Conn, error) {
	// 注意：KCP 使用 UDP，此处应直接连接远端服务器
	c, err := kcp.DialWithOptions(s.addr, s.block, s.dataShards, s.parityShards)
	if err != nil {
		log.F("[kcp] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	s.setParams(c)

	c.SetDSCP(0)
	c.SetReadBuffer(4194304)
	c.SetWriteBuffer(4194304)

	return c, err
}

// DialUDP 通过代理连接到给定地址（UDP）。
func (s *KCP) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}

func (s *KCP) setParams(c *kcp.UDPSession) {
	// TODO: 后续改为可自定义配置？
	c.SetStreamMode(true)
	c.SetWriteDelay(false)

	switch s.mode {
	case "normal":
		c.SetNoDelay(0, 40, 2, 1)
	case "fast":
		c.SetNoDelay(0, 30, 2, 1)
	case "fast2":
		c.SetNoDelay(1, 20, 2, 1)
	case "fast3":
		c.SetNoDelay(1, 10, 2, 1)
	default:
		log.F("[kcp] unkonw mode: %s, use fast mode instead", s.mode)
		c.SetNoDelay(0, 30, 2, 1)
	}

	c.SetWindowSize(1024, 1024)
	c.SetMtu(1350)
	c.SetACKNoDelay(true)
}

func init() {
	proxy.AddUsage("kcp", `
KCP 方案：
  kcp://CRYPT:KEY@host:port[?dataShards=NUM&parityShards=NUM&mode=MODE]

KCP 可用加密类型：
  none, sm4, tea, xor, aes, aes-128, aes-192, blowfish, twofish, cast5, 3des, xtea, salsa20

KCP 可用模式：
  fast, fast2, fast3, normal，默认：fast
`)
}
