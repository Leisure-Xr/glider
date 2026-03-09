package ss

import (
	"net/url"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/ss/cipher"
)

// SS 是 Shadowsocks 的基础结构体。
type SS struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string

	cipher.Cipher
}

func init() {
	proxy.RegisterDialer("ss", NewSSDialer)
	proxy.RegisterServer("ss", NewSSServer)
}

// NewSS 返回一个 Shadowsocks 代理。
func NewSS(s string, d proxy.Dialer, p proxy.Proxy) (*SS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[ss] parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	method := u.User.Username()
	pass, _ := u.User.Password()

	ciph, err := cipher.PickCipher(method, nil, pass)
	if err != nil {
		log.Fatalf("[ss] PickCipher for '%s', error: %s", method, err)
	}

	ss := &SS{
		dialer: d,
		proxy:  p,
		addr:   addr,
		Cipher: ciph,
	}

	return ss, nil
}

func init() {
	proxy.AddUsage("ss", `
SS（Shadowsocks）方案：
  ss://method:pass@host:port

  SS 可用加密方法：
    AEAD 加密：
      AEAD_AES_128_GCM AEAD_AES_192_GCM AEAD_AES_256_GCM AEAD_CHACHA20_POLY1305 AEAD_XCHACHA20_POLY1305
    流式加密：
      AES-128-CFB AES-128-CTR AES-192-CFB AES-192-CTR AES-256-CFB AES-256-CTR CHACHA20-IETF XCHACHA20 CHACHA20 RC4-MD5
    别名：
	  chacha20-ietf-poly1305 = AEAD_CHACHA20_POLY1305, xchacha20-ietf-poly1305 = AEAD_XCHACHA20_POLY1305
    明文：NONE
`)
}
