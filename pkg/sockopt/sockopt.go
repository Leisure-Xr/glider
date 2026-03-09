package sockopt

import (
	"net"
	"syscall"
)

// Options 是套接字选项结构体。
type Options struct {
	bindIface *net.Interface
	reuseAddr bool
}

// Option 是选项函数参数类型。
type Option func(opts *Options)

// Bind 设置绑定网络接口选项。
func Bind(intf *net.Interface) Option { return func(opts *Options) { opts.bindIface = intf } }

// ReuseAddr 设置地址复用选项。
func ReuseAddr() Option { return func(opts *Options) { opts.reuseAddr = true } }

// Control 返回用于 net.Dialer 和 net.ListenConfig 的控制函数。
func Control(opts ...Option) func(network, address string, c syscall.RawConn) error {
	option := &Options{}
	for _, opt := range opts {
		opt(option)
	}

	return control(option)
}
