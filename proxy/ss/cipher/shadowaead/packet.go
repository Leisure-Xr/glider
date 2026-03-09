package shadowaead

import (
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
)

// ErrShortPacket 表示数据包太短，不是有效的加密数据包。
var ErrShortPacket = errors.New("short packet")

var _zerononce [128]byte // 只读。128 字节已绰绰有余。

// Pack 使用加密算法和随机生成的盐值加密明文，
// 返回包含加密后数据包的 dst 切片及任何发生的错误。
// 请确保 len(dst) >= ciph.SaltSize() + len(plaintext) + aead.Overhead()。
func Pack(dst, plaintext []byte, ciph Cipher) ([]byte, error) {
	saltSize := ciph.SaltSize()
	salt := dst[:saltSize]
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	aead, err := ciph.Encrypter(salt)
	if err != nil {
		return nil, err
	}

	if len(dst) < saltSize+len(plaintext)+aead.Overhead() {
		return nil, io.ErrShortBuffer
	}
	b := aead.Seal(dst[saltSize:saltSize], _zerononce[:aead.NonceSize()], plaintext, nil)
	return dst[:saltSize+len(b)], nil
}

// Unpack 使用加密算法解密 pkt，返回包含解密后载荷的 dst 切片及任何发生的错误。
// 请确保 len(dst) >= len(pkt) - aead.SaltSize() - aead.Overhead()。
func Unpack(dst, pkt []byte, ciph Cipher) ([]byte, error) {
	saltSize := ciph.SaltSize()
	if len(pkt) < saltSize {
		return nil, ErrShortPacket
	}
	salt := pkt[:saltSize]
	aead, err := ciph.Decrypter(salt)
	if err != nil {
		return nil, err
	}
	if len(pkt) < saltSize+aead.Overhead() {
		return nil, ErrShortPacket
	}
	if saltSize+len(dst)+aead.Overhead() < len(pkt) {
		return nil, io.ErrShortBuffer
	}
	b, err := aead.Open(dst[:0], _zerononce[:aead.NonceSize()], pkt[saltSize:], nil)
	return b, err
}

type packetConn struct {
	net.PacketConn
	Cipher
	sync.Mutex
	buf []byte // 写锁
}

// NewPacketConn 使用加密算法封装一个 net.PacketConn
func NewPacketConn(c net.PacketConn, ciph Cipher) net.PacketConn {
	const maxPacketSize = 64 * 1024
	return &packetConn{PacketConn: c, Cipher: ciph, buf: make([]byte, maxPacketSize)}
}

// WriteTo 加密 b 并使用内嵌的 PacketConn 写入 addr。
func (c *packetConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.Lock()
	defer c.Unlock()
	buf, err := Pack(c.buf, b, c)
	if err != nil {
		return 0, err
	}
	_, err = c.PacketConn.WriteTo(buf, addr)
	return len(b), err
}

// ReadFrom 从内嵌的 PacketConn 读取数据并解密到 b 中。
func (c *packetConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(b)
	if err != nil {
		return n, addr, err
	}
	bb, err := Unpack(b[c.Cipher.SaltSize():], b[:n], c)
	if err != nil {
		return n, addr, err
	}
	copy(b, bb)
	return len(bb), addr, err
}
