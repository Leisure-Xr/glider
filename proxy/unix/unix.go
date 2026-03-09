package unix

import (
	"net"
	"net/url"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// Unix 是 Unix 域套接字结构体。
type Unix struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	server proxy.Server

	addr  string // TCP 地址
	uaddr *net.UnixAddr

	addru  string // UDP 地址（数据报）
	uaddru *net.UnixAddr
}

// NewUnix 返回 Unix 域套接字代理。
func NewUnix(s string, d proxy.Dialer, p proxy.Proxy) (*Unix, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[unix] parse url err: %s", err)
		return nil, err
	}

	unix := &Unix{
		dialer: d,
		proxy:  p,
		addr:   u.Path,
		addru:  u.Path + "u",
	}

	unix.uaddr, err = net.ResolveUnixAddr("unixgram", unix.addr)
	if err != nil {
		return nil, err
	}

	unix.uaddru, err = net.ResolveUnixAddr("unixgram", unix.addru)
	if err != nil {
		return nil, err
	}

	return unix, nil
}

func init() {
	proxy.AddUsage("unix", `
Unix 域套接字方案：
  unix://path
`)
}
