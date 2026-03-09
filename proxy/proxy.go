package proxy

import (
	"net"
	"strings"
)

// Proxy 是代理拨号管理器接口。
type Proxy interface {
	// Dial 通过代理连接到指定地址。
	Dial(network, addr string) (c net.Conn, dialer Dialer, err error)

	// DialUDP 通过代理连接到指定 UDP 地址。
	DialUDP(network, addr string) (pc net.PacketConn, dialer UDPDialer, err error)

	// NextDialer 根据目标地址获取拨号器。
	NextDialer(dstAddr string) Dialer

	// Record 记录使用该拨号器的结果。
	Record(dialer Dialer, success bool)
}

var (
	msg    strings.Builder
	usages = make(map[string]string)
)

// AddUsage 为指定代理添加帮助说明。
func AddUsage(name, usage string) {
	usages[name] = usage
	msg.WriteString(usage)
	msg.WriteString("\n--")
}

// Usage 返回指定代理的帮助说明。
func Usage(name string) string {
	if name == "all" {
		return msg.String()
	}

	if usage, ok := usages[name]; ok {
		return usage
	}

	return "未找到方案说明：" + name
}
