package vmess

import (
	"encoding/binary"

	"golang.org/x/crypto/sha3"
)

// ChunkSizeEncoder 是将大小值编码为字节的工具类。
type ChunkSizeEncoder interface {
	SizeBytes() int32
	Encode(uint16, []byte) []byte
}

// ChunkSizeDecoder 是从字节解码大小值的工具类。
type ChunkSizeDecoder interface {
	SizeBytes() int32
	Decode([]byte) (uint16, error)
}

// ShakeSizeParser 实现了 ChunkSizeEncoder 和 ChunkSizeDecoder。
type ShakeSizeParser struct {
	shake  sha3.ShakeHash
	buffer [2]byte
}

// NewShakeSizeParser 返回一个新的 ShakeSizeParser。
func NewShakeSizeParser(nonce []byte) *ShakeSizeParser {
	shake := sha3.NewShake128()
	shake.Write(nonce)
	return &ShakeSizeParser{
		shake: shake,
	}
}

// SizeBytes 实现 ChunkSizeEncoder 方法。
func (*ShakeSizeParser) SizeBytes() int32 {
	return 2
}

func (s *ShakeSizeParser) next() uint16 {
	s.shake.Read(s.buffer[:])
	return binary.BigEndian.Uint16(s.buffer[:])
}

// Decode 实现 ChunkSizeDecoder 方法。
func (s *ShakeSizeParser) Decode(b []byte) (uint16, error) {
	mask := s.next()
	size := binary.BigEndian.Uint16(b)
	return mask ^ size, nil
}

// Encode 实现 ChunkSizeEncoder 方法。
func (s *ShakeSizeParser) Encode(size uint16, b []byte) []byte {
	mask := s.next()
	binary.BigEndian.PutUint16(b, mask^size)
	return b[:2]
}
