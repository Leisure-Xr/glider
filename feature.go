package main

import (
	// 注释掉不需要的服务，可以减小编译后的二进制文件体积。
	// _ "github.com/nadoo/glider/service/xxx"

	// 注释掉不需要的协议，可以减小编译后的二进制文件体积。
	_ "github.com/nadoo/glider/proxy/http"
	_ "github.com/nadoo/glider/proxy/kcp"
	_ "github.com/nadoo/glider/proxy/mixed"
	_ "github.com/nadoo/glider/proxy/obfs"
	_ "github.com/nadoo/glider/proxy/pxyproto"
	_ "github.com/nadoo/glider/proxy/reject"
	_ "github.com/nadoo/glider/proxy/smux"
	_ "github.com/nadoo/glider/proxy/socks4"
	_ "github.com/nadoo/glider/proxy/socks5"
	_ "github.com/nadoo/glider/proxy/ss"
	_ "github.com/nadoo/glider/proxy/ssh"
	_ "github.com/nadoo/glider/proxy/ssr"
	_ "github.com/nadoo/glider/proxy/tcp"
	_ "github.com/nadoo/glider/proxy/tls"
	_ "github.com/nadoo/glider/proxy/trojan"
	_ "github.com/nadoo/glider/proxy/udp"
	_ "github.com/nadoo/glider/proxy/vless"
	_ "github.com/nadoo/glider/proxy/vmess"
	_ "github.com/nadoo/glider/proxy/ws"
)
