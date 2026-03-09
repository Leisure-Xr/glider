package proxy

import (
	"errors"
	"net"
	"sort"
	"strings"
)

var (
	// ErrNotSupported 表示该操作不受支持
	ErrNotSupported = errors.New("not supported")
)

// Dialer 用于创建连接。
type Dialer interface {
	TCPDialer
	UDPDialer
}

// TCPDialer 用于创建 TCP 连接。
type TCPDialer interface {
	// Addr 是拨号器的地址
	Addr() string

	// Dial 连接到给定地址
	Dial(network, addr string) (c net.Conn, err error)
}

// UDPDialer 用于创建 UDP PacketConn。
type UDPDialer interface {
	// Addr 是拨号器的地址
	Addr() string

	// DialUDP 连接到给定地址
	DialUDP(network, addr string) (pc net.PacketConn, err error)
}

// DialerCreator 是用于创建拨号器的函数类型。
type DialerCreator func(s string, dialer Dialer) (Dialer, error)

var (
	dialerCreators = make(map[string]DialerCreator)
)

// RegisterDialer 用于注册一个拨号器。
func RegisterDialer(name string, c DialerCreator) {
	dialerCreators[strings.ToLower(name)] = c
}

// DialerFromURL 调用已注册的创建函数来创建拨号器。
// dialer 是默认的上游拨号器，不能为 nil，调用此函数时可使用 Default。
func DialerFromURL(s string, dialer Dialer) (Dialer, error) {
	if dialer == nil {
		return nil, errors.New("DialerFromURL: dialer cannot be nil")
	}

	if !strings.Contains(s, "://") {
		s = s + "://"
	}

	scheme := s[:strings.Index(s, ":")]
	c, ok := dialerCreators[strings.ToLower(scheme)]
	if ok {
		return c(s, dialer)
	}

	return nil, errors.New("unknown scheme '" + scheme + "'")
}

// DialerSchemes 返回已注册的拨号器协议方案列表。
func DialerSchemes() string {
	s := make([]string, 0, len(dialerCreators))
	for name := range dialerCreators {
		s = append(s, name)
	}
	sort.Strings(s)
	return strings.Join(s, " ")
}
