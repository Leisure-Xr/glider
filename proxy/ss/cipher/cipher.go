package cipher

import (
	"crypto/md5"
	"errors"
	"net"
	"strings"

	"github.com/nadoo/glider/proxy/ss/cipher/shadowaead"
	"github.com/nadoo/glider/proxy/ss/cipher/shadowstream"
)

// Cipher 加密算法接口。
type Cipher interface {
	StreamConnCipher
	PacketConnCipher
}

// StreamConnCipher 是流连接加密算法。
type StreamConnCipher interface {
	StreamConn(net.Conn) net.Conn
}

// PacketConnCipher 是数据包连接加密算法。
type PacketConnCipher interface {
	PacketConn(net.PacketConn) net.PacketConn
}

// ErrCipherNotSupported 当加密算法不受支持时返回此错误（通常出于安全考虑）。
var ErrCipherNotSupported = errors.New("cipher not supported")

// AEAD 加密算法列表：密钥长度（字节）及构造函数
var aeadList = map[string]struct {
	KeySize int
	New     func([]byte) (shadowaead.Cipher, error)
}{
	"AEAD_AES_128_GCM":       {16, shadowaead.AESGCM},
	"AEAD_AES_192_GCM":       {24, shadowaead.AESGCM},
	"AEAD_AES_256_GCM":       {32, shadowaead.AESGCM},
	"AEAD_CHACHA20_POLY1305": {32, shadowaead.Chacha20Poly1305},

	// http://shadowsocks.org/en/spec/AEAD-Ciphers.html
	// 规范中未列出
	"AEAD_XCHACHA20_POLY1305": {32, shadowaead.XChacha20Poly1305},
}

// 流式加密算法列表：密钥长度（字节）及构造函数
var streamList = map[string]struct {
	KeySize int
	New     func(key []byte) (shadowstream.Cipher, error)
}{
	"AES-128-CTR":   {16, shadowstream.AESCTR},
	"AES-192-CTR":   {24, shadowstream.AESCTR},
	"AES-256-CTR":   {32, shadowstream.AESCTR},
	"AES-128-CFB":   {16, shadowstream.AESCFB},
	"AES-192-CFB":   {24, shadowstream.AESCFB},
	"AES-256-CFB":   {32, shadowstream.AESCFB},
	"CHACHA20-IETF": {32, shadowstream.Chacha20IETF},
	"XCHACHA20":     {32, shadowstream.Xchacha20},

	// http://shadowsocks.org/en/spec/Stream-Ciphers.html
	// 标记为"请勿使用"
	"CHACHA20": {32, shadowstream.ChaCha20},
	"RC4-MD5":  {16, shadowstream.RC4MD5},
}

// PickCipher 返回指定名称的加密算法。若密钥为空，则从密码派生密钥。
func PickCipher(name string, key []byte, password string) (Cipher, error) {
	name = strings.ToUpper(name)

	switch name {
	case "DUMMY", "NONE":
		return &dummy{}, nil
	case "CHACHA20-IETF-POLY1305":
		name = "AEAD_CHACHA20_POLY1305"
	case "XCHACHA20-IETF-POLY1305":
		name = "AEAD_XCHACHA20_POLY1305"
	case "AES-128-GCM":
		name = "AEAD_AES_128_GCM"
	case "AES-192-GCM":
		name = "AEAD_AES_192_GCM"
	case "AES-256-GCM":
		name = "AEAD_AES_256_GCM"
	}

	if choice, ok := aeadList[name]; ok {
		if len(key) == 0 {
			key = kdf(password, choice.KeySize)
		}
		if len(key) != choice.KeySize {
			return nil, shadowaead.KeySizeError(choice.KeySize)
		}
		aead, err := choice.New(key)
		return &aeadCipher{aead}, err
	}

	if choice, ok := streamList[name]; ok {
		if len(key) == 0 {
			key = kdf(password, choice.KeySize)
		}
		if len(key) != choice.KeySize {
			return nil, shadowstream.KeySizeError(choice.KeySize)
		}
		ciph, err := choice.New(key)
		return &streamCipher{ciph}, err
	}

	return nil, ErrCipherNotSupported
}

type aeadCipher struct{ shadowaead.Cipher }

func (aead *aeadCipher) StreamConn(c net.Conn) net.Conn { return shadowaead.NewConn(c, aead) }
func (aead *aeadCipher) PacketConn(c net.PacketConn) net.PacketConn {
	return shadowaead.NewPacketConn(c, aead)
}

type streamCipher struct{ shadowstream.Cipher }

func (ciph *streamCipher) StreamConn(c net.Conn) net.Conn { return shadowstream.NewConn(c, ciph) }
func (ciph *streamCipher) PacketConn(c net.PacketConn) net.PacketConn {
	return shadowstream.NewPacketConn(c, ciph)
}

// dummy 加密算法不进行加密
type dummy struct{}

func (dummy) StreamConn(c net.Conn) net.Conn             { return c }
func (dummy) PacketConn(c net.PacketConn) net.PacketConn { return c }

// kdf 是原始 Shadowsocks 的密钥派生函数
func kdf(password string, keyLen int) []byte {
	var b, prev []byte
	h := md5.New()
	for len(b) < keyLen {
		h.Write(prev)
		h.Write([]byte(password))
		b = h.Sum(b)
		prev = b[len(b)-h.Size():]
		h.Reset()
	}
	return b[:keyLen]
}
