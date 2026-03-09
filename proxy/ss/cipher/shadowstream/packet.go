package shadowstream

import (
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
)

// ErrShortPacket 表示数据包太短，不是有效的加密数据包。
var ErrShortPacket = errors.New("short packet")

// Pack 使用流式加密算法 s 和随机 IV 加密明文。
// 返回包含随机 IV 和密文的 dst 切片。
// 请确保 len(dst) >= s.IVSize() + len(plaintext)。
func Pack(dst, plaintext []byte, s Cipher) ([]byte, error) {
	if len(dst) < s.IVSize()+len(plaintext) {
		return nil, io.ErrShortBuffer
	}
	iv := dst[:s.IVSize()]
	_, err := io.ReadFull(rand.Reader, iv)
	if err != nil {
		return nil, err
	}

	s.Encrypter(iv).XORKeyStream(dst[len(iv):], plaintext)
	return dst[:len(iv)+len(plaintext)], nil
}

// Unpack 使用流式加密算法 s 解密数据包 pkt。
// 返回包含解密后明文的 dst 切片。
func Unpack(dst, pkt []byte, s Cipher) ([]byte, error) {
	if len(pkt) < s.IVSize() {
		return nil, ErrShortPacket
	}

	if len(dst) < len(pkt)-s.IVSize() {
		return nil, io.ErrShortBuffer
	}
	iv := pkt[:s.IVSize()]
	s.Decrypter(iv).XORKeyStream(dst, pkt[len(iv):])
	return dst[:len(pkt)-len(iv)], nil
}

type packetConn struct {
	net.PacketConn
	Cipher
	buf        []byte
	sync.Mutex // 写锁
}

// NewPacketConn 使用流式加密算法封装一个 net.PacketConn，提供加密/解密功能。
func NewPacketConn(c net.PacketConn, ciph Cipher) net.PacketConn {
	return &packetConn{PacketConn: c, Cipher: ciph, buf: make([]byte, 64*1024)}
}

func (c *packetConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.Lock()
	defer c.Unlock()
	buf, err := Pack(c.buf, b, c.Cipher)
	if err != nil {
		return 0, err
	}
	_, err = c.PacketConn.WriteTo(buf, addr)
	return len(b), err
}

func (c *packetConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(b)
	if err != nil {
		return n, addr, err
	}
	bb, err := Unpack(b[c.IVSize():], b[:n], c.Cipher)
	if err != nil {
		return n, addr, err
	}
	copy(b, bb)
	return len(bb), addr, err
}
