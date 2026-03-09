package smux

import "github.com/nadoo/glider/proxy"

func init() {
	proxy.AddUsage("smux", `
Smux（多路复用）方案：
  smux://host:port
`)
}
