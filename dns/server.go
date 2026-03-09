package dns

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

// 连接超时时间，单位：秒。
const timeout = 30

// Server 是 DNS 服务器的结构体。
type Server struct {
	addr string
	// Client 用于与上游 DNS 服务器通信
	*Client
}

// NewServer 返回一个新的 DNS 服务器。
func NewServer(addr string, p proxy.Proxy, config *Config) (*Server, error) {
	c, err := NewClient(p, config)
	if err != nil {
		return nil, err
	}

	s := &Server{
		addr:   addr,
		Client: c,
	}
	return s, nil
}

// Start 启动 DNS 转发服务器。
// 此处使用 WaitGroup 以确保 UDP 和 TCP 服务器均已完全启动，
// 从而可以在之后启动其他可能依赖 DNS 服务的服务。
func (s *Server) Start() {
	var wg sync.WaitGroup
	wg.Add(2)
	go s.ListenAndServeTCP(&wg)
	go s.ListenAndServeUDP(&wg)
	wg.Wait()
}

// ListenAndServeUDP 在 UDP 端口上监听并处理请求。
func (s *Server) ListenAndServeUDP(wg *sync.WaitGroup) {
	pc, err := net.ListenPacket("udp", s.addr)
	wg.Done()
	if err != nil {
		log.F("[dns] failed to listen on %s, error: %v", s.addr, err)
		return
	}
	defer pc.Close()

	log.F("[dns] listening UDP on %s", s.addr)

	for {
		reqBytes := pool.GetBuffer(UDPMaxLen)
		n, caddr, err := pc.ReadFrom(reqBytes)
		if err != nil {
			log.F("[dns] local read error: %v", err)
			pool.PutBuffer(reqBytes)
			continue
		}
		go s.ServePacket(pc, caddr, reqBytes[:n])
	}
}

// ServePacket 处理 DNS 数据包连接。
func (s *Server) ServePacket(pc net.PacketConn, caddr net.Addr, reqBytes []byte) {
	respBytes, err := s.Exchange(reqBytes, caddr.String(), false)
	defer func() {
		pool.PutBuffer(reqBytes)
		pool.PutBuffer(respBytes)
	}()

	if err != nil {
		log.F("[dns] error in exchange for %s: %s", caddr, err)
		return
	}

	_, err = pc.WriteTo(respBytes, caddr)
	if err != nil {
		log.F("[dns] error in local write to %s: %s", caddr, err)
		return
	}
}

// ListenAndServeTCP 在 TCP 端口上监听并处理请求。
func (s *Server) ListenAndServeTCP(wg *sync.WaitGroup) {
	l, err := net.Listen("tcp", s.addr)
	wg.Done()
	if err != nil {
		log.F("[dns-tcp] error: %v", err)
		return
	}
	defer l.Close()

	log.F("[dns-tcp] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[dns-tcp] error: failed to accept: %v", err)
			continue
		}
		go s.ServeTCP(c)
	}
}

// ServeTCP 处理一个 DNS TCP 连接。
func (s *Server) ServeTCP(c net.Conn) {
	defer c.Close()

	c.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))

	var reqLen uint16
	if err := binary.Read(c, binary.BigEndian, &reqLen); err != nil {
		log.F("[dns-tcp] failed to get request length: %v", err)
		return
	}

	reqBytes := pool.GetBuffer(int(reqLen))
	defer pool.PutBuffer(reqBytes)

	_, err := io.ReadFull(c, reqBytes)
	if err != nil {
		log.F("[dns-tcp] error in read reqBytes %s", err)
		return
	}

	respBytes, err := s.Exchange(reqBytes, c.RemoteAddr().String(), true)
	defer pool.PutBuffer(respBytes)
	if err != nil {
		log.F("[dns-tcp] error in exchange: %s", err)
		return
	}

	lenBuf := pool.GetBuffer(2)
	defer pool.PutBuffer(lenBuf)

	binary.BigEndian.PutUint16(lenBuf, uint16(len(respBytes)))
	if _, err := (&net.Buffers{lenBuf, respBytes}).WriteTo(c); err != nil {
		log.F("[dns-tcp] error in write respBytes: %s", err)
	}
}
