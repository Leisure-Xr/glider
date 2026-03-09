package dns

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

// AnswerHandler 函数用于处理 DNS TypeA 或 TypeAAAA 类型的应答。
type AnswerHandler func(domain string, ip netip.Addr) error

// Config 是 DNS 的配置结构体。
type Config struct {
	Servers   []string
	Timeout   int
	MaxTTL    int
	MinTTL    int
	Records   []string
	AlwaysTCP bool
	CacheSize int
	CacheLog  bool
	NoAAAA    bool
}

// Client 是 DNS 客户端的结构体。
type Client struct {
	proxy       proxy.Proxy
	cache       *LruCache
	config      *Config
	upStream    *UPStream
	upStreamMap map[string]*UPStream
	handlers    []AnswerHandler
}

// NewClient 返回一个新的 DNS 客户端。
func NewClient(proxy proxy.Proxy, config *Config) (*Client, error) {
	c := &Client{
		proxy:       proxy,
		cache:       NewLruCache(config.CacheSize),
		config:      config,
		upStream:    NewUPStream(config.Servers),
		upStreamMap: make(map[string]*UPStream),
	}

	// 自定义记录
	for _, record := range config.Records {
		if err := c.AddRecord(record); err != nil {
			log.F("[dns] add record '%s' error: %s", record, err)
		}
	}

	return c, nil
}

// Exchange 处理请求消息并返回响应消息。
// TODO: 待优化
func (c *Client) Exchange(reqBytes []byte, clientAddr string, preferTCP bool) ([]byte, error) {
	req, err := UnmarshalMessage(reqBytes)
	if err != nil {
		return nil, err
	}

	if c.config.NoAAAA && req.Question.QTYPE == QTypeAAAA {
		respBytes := valCopy(reqBytes)
		respBytes[2] |= uint8(ResponseMsg) << 7
		return respBytes, nil
	}

	if req.Question.QTYPE == QTypeA || req.Question.QTYPE == QTypeAAAA {
		if v, expired := c.cache.Get(qKey(req.Question)); len(v) > 2 {
			v = valCopy(v)
			binary.BigEndian.PutUint16(v[:2], req.ID)

			if c.config.CacheLog {
				log.F("[dns] %s <-> cache, type: %d, %s",
					clientAddr, req.Question.QTYPE, req.Question.QNAME)
			}

			if expired { // 更新缓存
				go func(qname string, reqBytes []byte, preferTCP bool) {
					defer pool.PutBuffer(reqBytes)
					if dnsServer, network, dialerAddr, respBytes, err := c.exchange(qname, reqBytes, preferTCP); err == nil {
						c.handleAnswer(respBytes, "cache", dnsServer, network, dialerAddr)
					}
				}(req.Question.QNAME, valCopy(reqBytes), preferTCP)
			}
			return v, nil
		}
	}

	dnsServer, network, dialerAddr, respBytes, err := c.exchange(req.Question.QNAME, reqBytes, preferTCP)
	if err != nil {
		return nil, err
	}

	if req.Question.QTYPE != QTypeA && req.Question.QTYPE != QTypeAAAA {
		log.F("[dns] %s <-> %s(%s) via %s, type: %d, %s",
			clientAddr, dnsServer, network, dialerAddr, req.Question.QTYPE, req.Question.QNAME)
		return respBytes, nil
	}

	err = c.handleAnswer(respBytes, clientAddr, dnsServer, network, dialerAddr)
	return respBytes, err
}

func (c *Client) handleAnswer(respBytes []byte, clientAddr, dnsServer, network, dialerAddr string) error {
	resp, err := UnmarshalMessage(respBytes)
	if err != nil {
		return err
	}

	ips, ttl := c.extractAnswer(resp)
	if ttl > c.config.MaxTTL {
		ttl = c.config.MaxTTL
	} else if ttl < c.config.MinTTL {
		ttl = c.config.MinTTL
	}

	if ttl <= 0 { // 得到了空结果
		ttl = 1800
	}

	c.cache.Set(qKey(resp.Question), valCopy(respBytes), ttl)
	log.F("[dns] %s <-> %s(%s) via %s, %s/%d: %s, ttl: %ds",
		clientAddr, dnsServer, network, dialerAddr, resp.Question.QNAME, resp.Question.QTYPE, strings.Join(ips, ","), ttl)

	return nil
}

func (c *Client) extractAnswer(resp *Message) ([]string, int) {
	var ips []string
	ttl := c.config.MinTTL
	for _, answer := range resp.Answers {
		if answer.TYPE == QTypeA || answer.TYPE == QTypeAAAA {
			if answer.IP.IsValid() && !answer.IP.IsUnspecified() {
				for _, h := range c.handlers {
					h(resp.Question.QNAME, answer.IP)
				}
				ips = append(ips, answer.IP.String())
			}
			if answer.TTL != 0 {
				ttl = int(answer.TTL)
			}
		}
	}

	return ips, ttl
}

// exchange 根据 qname 选择上游 DNS 服务器，并通过网络与其通信。
func (c *Client) exchange(qname string, reqBytes []byte, preferTCP bool) (
	server, network, dialerAddr string, respBytes []byte, err error) {

	// 默认使用 TCP 连接上游服务器
	network = "tcp"
	dialer := c.proxy.NextDialer(qname + ":0")

	// 如果正在解析的域名使用了 `REJECT` 转发器，则改用 `DIRECT`，
	// 以确保能够正确解析。
	// TODO: dialer.Addr() == "REJECT", 处理较为特殊
	if dialer.Addr() == "REJECT" {
		dialer = c.proxy.NextDialer("direct:0")
	}

	// 如果客户端使用 UDP 且未指定转发器，则使用 UDP
	// TODO: dialer.Addr() == "DIRECT", 处理较为特殊
	if !preferTCP && !c.config.AlwaysTCP && dialer.Addr() == "DIRECT" {
		network = "udp"
	}

	ups := c.UpStream(qname)
	server = ups.Server()
	for range ups.Len() {
		var rc net.Conn
		rc, err = dialer.Dial(network, server)
		if err != nil {
			newServer := ups.SwitchIf(server)
			log.F("[dns] error in resolving %s, failed to connect to server %v via %s: %v, next server: %s",
				qname, server, dialer.Addr(), err, newServer)
			server = newServer
			continue
		}
		defer rc.Close()

		// TODO: 支持为不同上游服务器单独设置超时时间
		if c.config.Timeout > 0 {
			rc.SetDeadline(time.Now().Add(time.Duration(c.config.Timeout) * time.Second))
		}

		switch network {
		case "tcp":
			respBytes, err = c.exchangeTCP(rc, reqBytes)
		case "udp":
			respBytes, err = c.exchangeUDP(rc, reqBytes)
		}

		if err == nil {
			break
		}

		newServer := ups.SwitchIf(server)
		log.F("[dns] error in resolving %s, failed to exchange with server %v via %s: %v, next server: %s",
			qname, server, dialer.Addr(), err, newServer)

		server = newServer
	}

	// 如果所有 DNS 上游均失败，则可能是转发器不可用。
	if err != nil {
		c.proxy.Record(dialer, false)
	}

	return server, network, dialer.Addr(), respBytes, err
}

// exchangeTCP 通过 TCP 与服务器进行消息交换。
func (c *Client) exchangeTCP(rc net.Conn, reqBytes []byte) ([]byte, error) {
	lenBuf := pool.GetBuffer(2)
	defer pool.PutBuffer(lenBuf)

	binary.BigEndian.PutUint16(lenBuf, uint16(len(reqBytes)))
	if _, err := (&net.Buffers{lenBuf, reqBytes}).WriteTo(rc); err != nil {
		return nil, err
	}

	var respLen uint16
	if err := binary.Read(rc, binary.BigEndian, &respLen); err != nil {
		return nil, err
	}

	respBytes := pool.GetBuffer(int(respLen))
	_, err := io.ReadFull(rc, respBytes)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

// exchangeUDP 通过 UDP 与服务器进行消息交换。
func (c *Client) exchangeUDP(rc net.Conn, reqBytes []byte) ([]byte, error) {
	if _, err := rc.Write(reqBytes); err != nil {
		return nil, err
	}

	respBytes := pool.GetBuffer(UDPMaxLen)
	n, err := rc.Read(respBytes)
	if err != nil {
		return nil, err
	}

	return respBytes[:n], nil
}

// SetServers 为指定域名设置上游 DNS 服务器。
func (c *Client) SetServers(domain string, servers []string) {
	c.upStreamMap[strings.ToLower(domain)] = NewUPStream(servers)
}

// UpStream 返回指定域名对应的上游 DNS 服务器。
func (c *Client) UpStream(domain string) *UPStream {
	domain = strings.ToLower(domain)
	for i := len(domain); i != -1; {
		i = strings.LastIndexByte(domain[:i], '.')
		if upstream, ok := c.upStreamMap[domain[i+1:]]; ok {
			return upstream
		}
	}
	return c.upStream
}

// AddHandler 添加自定义处理器，用于处理解析结果（A 和 AAAA 记录）。
func (c *Client) AddHandler(h AnswerHandler) {
	c.handlers = append(c.handlers, h)
}

// AddRecord 向 DNS 缓存中添加自定义记录，格式为：
// www.example.com/1.2.3.4 或 www.example.com/2606:2800:220:1:248:1893:25c8:1946
func (c *Client) AddRecord(record string) error {
	domain, ip, found := strings.Cut(record, "/")
	if !found {
		return errors.New("wrong record format, must contain '/'")
	}
	m, err := MakeResponse(domain, ip, uint32(c.config.MaxTTL))
	if err != nil {
		log.F("[dns] add custom record error: %s", err)
		return err
	}

	wb := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(wb)

	_, err = m.MarshalTo(wb)
	if err != nil {
		return err
	}

	c.cache.Set(qKey(m.Question), valCopy(wb.Bytes()), 0)

	return nil
}

// MakeResponse 为给定的域名和 IP 地址构造一条 DNS 响应消息。
// 注意：需确保 ttl > 0。
func MakeResponse(domain, ip string, ttl uint32) (*Message, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return nil, err
	}

	var qtype, rdlen uint16 = QTypeA, net.IPv4len
	if addr.Is6() {
		qtype, rdlen = QTypeAAAA, net.IPv6len
	}

	m := NewMessage(0, ResponseMsg)
	m.SetQuestion(NewQuestion(qtype, domain))
	rr := &RR{NAME: domain, TYPE: qtype, CLASS: ClassINET,
		TTL: ttl, RDLENGTH: rdlen, RDATA: addr.AsSlice()}
	m.AddAnswer(rr)

	return m, nil
}

func qKey(q *Question) string {
	return q.QNAME + "/" + strconv.FormatUint(uint64(q.QTYPE), 10)
}

func valCopy(v []byte) (b []byte) {
	if v != nil {
		b = pool.GetBuffer(len(v))
		copy(b, v)
	}
	return
}
