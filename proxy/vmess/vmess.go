package vmess

import (
	"net"
	"net/url"
	"strconv"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// VMess 结构体。
type VMess struct {
	dialer proxy.Dialer
	addr   string

	uuid     string
	aead     bool
	alterID  int
	security string

	client *Client
}

func init() {
	proxy.RegisterDialer("vmess", NewVMessDialer)
}

// NewVMess 返回一个 VMess 代理。
func NewVMess(s string, d proxy.Dialer) (*VMess, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host
	security := u.User.Username()
	uuid, ok := u.User.Password()
	if !ok {
		// 未指定加密类型，格式为 vmess://uuid@server
		uuid = security
		security = ""
	}

	query := u.Query()
	aid := query.Get("alterID")
	if aid == "" {
		aid = "0"
	}

	alterID, err := strconv.ParseUint(aid, 10, 32)
	if err != nil {
		log.F("parse alterID err: %s", err)
		return nil, err
	}

	aead := alterID == 0
	client, err := NewClient(uuid, security, int(alterID), aead)
	if err != nil {
		log.F("create vmess client err: %s", err)
		return nil, err
	}

	p := &VMess{
		dialer:   d,
		addr:     addr,
		uuid:     uuid,
		alterID:  int(alterID),
		security: security,
		client:   client,
	}

	return p, nil
}

// NewVMessDialer 返回一个 VMess 代理拨号器。
func NewVMessDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewVMess(s, dialer)
}

// Addr 返回转发器的地址。
func (s *VMess) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial 通过代理连接到网络 net 上的地址 addr。
func (s *VMess) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	return s.client.NewConn(rc, addr, CmdTCP)
}

// DialUDP 通过代理连接到给定地址（UDP）。
func (s *VMess) DialUDP(network, addr string) (net.PacketConn, error) {
	tgtAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.F("[vmess] error in ResolveUDPAddr: %v", err)
		return nil, err
	}

	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	rc, err = s.client.NewConn(rc, addr, CmdUDP)
	if err != nil {
		return nil, err
	}

	return NewPktConn(rc, tgtAddr), err
}

func init() {
	proxy.AddUsage("vmess", `
VMess 方案：
  vmess://[security:]uuid@host:port[?alterID=num]
    若 alterID=0 或未设置，将启用 VMessAEAD

  VMess 可用加密类型：
    zero, none, aes-128-gcm, chacha20-poly1305
`)
}
