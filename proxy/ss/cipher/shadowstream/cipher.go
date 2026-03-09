package shadowstream

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"strconv"

	"github.com/aead/chacha20"
	"github.com/aead/chacha20/chacha"
)

// Cipher 生成一对用于加密和解密的流式加密算法。
type Cipher interface {
	IVSize() int
	Encrypter(iv []byte) cipher.Stream
	Decrypter(iv []byte) cipher.Stream
}

// KeySizeError 是关于密钥长度的错误。
type KeySizeError int

func (e KeySizeError) Error() string {
	return "key size error: need " + strconv.Itoa(int(e)) + " bytes"
}

// CTR 模式
type ctrStream struct{ cipher.Block }

func (b *ctrStream) IVSize() int                       { return b.BlockSize() }
func (b *ctrStream) Decrypter(iv []byte) cipher.Stream { return b.Encrypter(iv) }
func (b *ctrStream) Encrypter(iv []byte) cipher.Stream { return cipher.NewCTR(b, iv) }

// AESCTR 返回一个 aesctr 加密算法。
func AESCTR(key []byte) (Cipher, error) {
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &ctrStream{blk}, nil
}

// CFB 模式
type cfbStream struct{ cipher.Block }

func (b *cfbStream) IVSize() int                       { return b.BlockSize() }
func (b *cfbStream) Decrypter(iv []byte) cipher.Stream { return cipher.NewCFBDecrypter(b, iv) }
func (b *cfbStream) Encrypter(iv []byte) cipher.Stream { return cipher.NewCFBEncrypter(b, iv) }

// AESCFB 返回一个 aescfb 加密算法。
func AESCFB(key []byte) (Cipher, error) {
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &cfbStream{blk}, nil
}

// chacha20 的 IETF 变体
type chacha20ietfkey []byte

func (k chacha20ietfkey) IVSize() int                       { return chacha.INonceSize }
func (k chacha20ietfkey) Decrypter(iv []byte) cipher.Stream { return k.Encrypter(iv) }
func (k chacha20ietfkey) Encrypter(iv []byte) cipher.Stream {
	ciph, err := chacha20.NewCipher(iv, k)
	if err != nil {
		panic(err) // 不应发生
	}
	return ciph
}

// Chacha20IETF 返回一个 Chacha20IETF 加密算法。
func Chacha20IETF(key []byte) (Cipher, error) {
	if len(key) != chacha.KeySize {
		return nil, KeySizeError(chacha.KeySize)
	}
	return chacha20ietfkey(key), nil
}

// xchacha20
type xchacha20key []byte

func (k xchacha20key) IVSize() int                       { return chacha.XNonceSize }
func (k xchacha20key) Decrypter(iv []byte) cipher.Stream { return k.Encrypter(iv) }
func (k xchacha20key) Encrypter(iv []byte) cipher.Stream {
	ciph, err := chacha20.NewCipher(iv, k)
	if err != nil {
		panic(err) // 不应发生
	}
	return ciph
}

// Xchacha20 返回一个 Xchacha20 加密算法。
func Xchacha20(key []byte) (Cipher, error) {
	if len(key) != chacha.KeySize {
		return nil, KeySizeError(chacha.KeySize)
	}
	return xchacha20key(key), nil
}

// chacah20
type chacha20key []byte

func (k chacha20key) IVSize() int                       { return chacha.NonceSize }
func (k chacha20key) Decrypter(iv []byte) cipher.Stream { return k.Encrypter(iv) }
func (k chacha20key) Encrypter(iv []byte) cipher.Stream {
	ciph, err := chacha20.NewCipher(iv, k)
	if err != nil {
		panic(err) // 不应发生
	}
	return ciph
}

// ChaCha20 返回一个 ChaCha20 加密算法。
func ChaCha20(key []byte) (Cipher, error) {
	if len(key) != chacha.KeySize {
		return nil, KeySizeError(chacha.KeySize)
	}
	return chacha20key(key), nil
}

// rc4md5
type rc4Md5Key []byte

func (k rc4Md5Key) IVSize() int                       { return 16 }
func (k rc4Md5Key) Decrypter(iv []byte) cipher.Stream { return k.Encrypter(iv) }
func (k rc4Md5Key) Encrypter(iv []byte) cipher.Stream {
	h := md5.New()
	h.Write([]byte(k))
	h.Write(iv)
	rc4key := h.Sum(nil)
	ciph, err := rc4.NewCipher(rc4key)
	if err != nil {
		panic(err) // 不应发生
	}
	return ciph
}

// RC4MD5 返回一个 RC4MD5 加密算法。
func RC4MD5(key []byte) (Cipher, error) {
	return rc4Md5Key(key), nil
}
