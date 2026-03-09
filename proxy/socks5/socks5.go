// https://www.rfc-editor.org/rfc/rfc1928

// socks5 客户端：
// https://github.com/golang/net/tree/master/proxy
// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package socks5 实现了一个 SOCKS5 代理。
package socks5

import (
	"net/url"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// Version 是 SOCKS5 的版本号。
const Version = 5

// Socks5 是 SOCKS5 的基础结构体。
type Socks5 struct {
	dialer   proxy.Dialer
	proxy    proxy.Proxy
	addr     string
	user     string
	password string
}

// NewSocks5 返回一个代理，该代理通过 SOCKS v5 协议连接到给定地址，
// 支持可选的用户名和密码。（RFC 1928）
func NewSocks5(s string, d proxy.Dialer, p proxy.Proxy) (*Socks5, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	user := u.User.Username()
	pass, _ := u.User.Password()

	h := &Socks5{
		dialer:   d,
		proxy:    p,
		addr:     addr,
		user:     user,
		password: pass,
	}

	return h, nil
}

func init() {
	proxy.AddUsage("socks5", `
SOCKS5 方案：
  socks5://[user:pass@]host:port
`)
}
