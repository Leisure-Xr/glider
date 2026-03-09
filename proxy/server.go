package proxy

import (
	"errors"
	"net"
	"sort"
	"strings"
)

// Server 接口。
type Server interface {
	// ListenAndServe 建立监听并开始服务
	ListenAndServe()

	// Serve 处理一个连接
	Serve(c net.Conn)
}

// PacketServer 接口。
type PacketServer interface {
	ServePacket(pc net.PacketConn)
}

// ServerCreator 是用于创建代理服务器的函数类型。
type ServerCreator func(s string, proxy Proxy) (Server, error)

var (
	serverCreators = make(map[string]ServerCreator)
)

// RegisterServer 用于注册一个代理服务器。
func RegisterServer(name string, c ServerCreator) {
	serverCreators[strings.ToLower(name)] = c
}

// ServerFromURL 调用已注册的创建函数来创建代理服务器。
// proxy 不能为 nil。
func ServerFromURL(s string, proxy Proxy) (Server, error) {
	if proxy == nil {
		return nil, errors.New("ServerFromURL: dialer cannot be nil")
	}

	if !strings.Contains(s, "://") {
		s = "mixed://" + s
	}

	scheme := s[:strings.Index(s, ":")]
	c, ok := serverCreators[strings.ToLower(scheme)]
	if ok {
		return c(s, proxy)
	}

	return nil, errors.New("unknown scheme '" + scheme + "'")
}

// ServerSchemes 返回已注册的服务器协议方案列表。
func ServerSchemes() string {
	s := make([]string, 0, len(serverCreators))
	for name := range serverCreators {
		s = append(s, name)
	}
	sort.Strings(s)
	return strings.Join(s, " ")
}
