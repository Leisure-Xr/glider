package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"encoding/binary"
	"errors"

	"github.com/aead/chacha20"
	"github.com/dgryski/go-camellia"
	"github.com/dgryski/go-idea"
	"github.com/dgryski/go-rc2"
	"golang.org/x/crypto/blowfish"
	"golang.org/x/crypto/cast5"
	"golang.org/x/crypto/salsa20/salsa"

	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy/ssr/internal/tools"
)

var errEmptyPassword = errors.New("empty key")

type DecOrEnc int

const (
	Decrypt DecOrEnc = iota
	Encrypt
)

func newCTRStream(block cipher.Block, err error, key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	if err != nil {
		return nil, err
	}
	return cipher.NewCTR(block, iv), nil
}

func newAESCTRStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	return newCTRStream(block, err, key, iv, doe)
}

func newOFBStream(block cipher.Block, err error, key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	if err != nil {
		return nil, err
	}
	return cipher.NewCTR(block, iv), nil
}

func newAESOFBStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	return newOFBStream(block, err, key, iv, doe)
}

func newCFBStream(block cipher.Block, err error, key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	if err != nil {
		return nil, err
	}
	if doe == Encrypt {
		return cipher.NewCFBEncrypter(block, iv), nil
	} else {
		return cipher.NewCFBDecrypter(block, iv), nil
	}
}

func newAESCFBStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	return newCFBStream(block, err, key, iv, doe)
}

func newDESStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := des.NewCipher(key)
	return newCFBStream(block, err, key, iv, doe)
}

func newBlowFishStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := blowfish.NewCipher(key)
	return newCFBStream(block, err, key, iv, doe)
}

func newCast5Stream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := cast5.NewCipher(key)
	return newCFBStream(block, err, key, iv, doe)
}

func newRC4MD5Stream(key, iv []byte, _ DecOrEnc) (cipher.Stream, error) {
	h := md5.New()
	h.Write(key)
	h.Write(iv)
	rc4key := h.Sum(nil)

	return rc4.NewCipher(rc4key)
}

func newChaCha20Stream(key, iv []byte, _ DecOrEnc) (cipher.Stream, error) {
	return chacha20.NewCipher(iv, key)
}

func newChacha20IETFStream(key, iv []byte, _ DecOrEnc) (cipher.Stream, error) {
	return chacha20.NewCipher(iv, key)
}

type salsaStreamCipher struct {
	nonce   [8]byte
	key     [32]byte
	counter int
}

func (c *salsaStreamCipher) XORKeyStream(dst, src []byte) {
	var buf []byte
	padLen := c.counter % 64
	dataSize := len(src) + padLen
	if cap(dst) >= dataSize {
		buf = dst[:dataSize]
		// nadoo: 注释掉此处代码以使用池缓冲区
		// 修改开始 -->
		// } else if leakybuf.GlobalLeakyBufSize >= dataSize {
		// 	buf = leakybuf.GlobalLeakyBuf.Get()
		// 	defer leakybuf.GlobalLeakyBuf.Put(buf)
		// 	buf = buf[:dataSize]
		// } else {
		// 	buf = make([]byte, dataSize)
		// }
	} else {
		buf = pool.GetBuffer(dataSize)
		defer pool.PutBuffer(buf)
	}
	// --> 修改结束

	var subNonce [16]byte
	copy(subNonce[:], c.nonce[:])
	binary.LittleEndian.PutUint64(subNonce[len(c.nonce):], uint64(c.counter/64))

	// 此处难以避免数据拷贝。src 或 dst 可能是来自
	// Conn.Read/Write 的切片，无法添加填充。
	copy(buf[padLen:], src[:])
	salsa.XORKeyStream(buf, buf, &subNonce, &c.key)
	copy(dst, buf[padLen:])

	c.counter += len(src)
}

func newSalsa20Stream(key, iv []byte, _ DecOrEnc) (cipher.Stream, error) {
	var c salsaStreamCipher
	copy(c.nonce[:], iv[:8])
	copy(c.key[:], key[:32])
	return &c, nil
}

func newCamelliaStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := camellia.New(key)
	return newCFBStream(block, err, key, iv, doe)
}

func newIdeaStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := idea.NewCipher(key)
	return newCFBStream(block, err, key, iv, doe)
}

func newRC2Stream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	block, err := rc2.New(key, 16)
	return newCFBStream(block, err, key, iv, doe)
}

func newRC4Stream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	return rc4.NewCipher(key)
}

func newSeedStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	// TODO: 需要实现 SEED 分组加密算法
	block, err := rc2.New(key, 16)
	return newCFBStream(block, err, key, iv, doe)
}

type NoneStream struct {
	cipher.Stream
}

func (*NoneStream) XORKeyStream(dst, src []byte) {
	copy(dst, src)
}

func newNoneStream(key, iv []byte, doe DecOrEnc) (cipher.Stream, error) {
	return new(NoneStream), nil
}

type cipherInfo struct {
	keyLen    int
	ivLen     int
	newStream func(key, iv []byte, doe DecOrEnc) (cipher.Stream, error)
}

var streamCipherMethod = map[string]*cipherInfo{
	"aes-128-cfb":      {16, 16, newAESCFBStream},
	"aes-192-cfb":      {24, 16, newAESCFBStream},
	"aes-256-cfb":      {32, 16, newAESCFBStream},
	"aes-128-ctr":      {16, 16, newAESCTRStream},
	"aes-192-ctr":      {24, 16, newAESCTRStream},
	"aes-256-ctr":      {32, 16, newAESCTRStream},
	"aes-128-ofb":      {16, 16, newAESOFBStream},
	"aes-192-ofb":      {24, 16, newAESOFBStream},
	"aes-256-ofb":      {32, 16, newAESOFBStream},
	"des-cfb":          {8, 8, newDESStream},
	"bf-cfb":           {16, 8, newBlowFishStream},
	"cast5-cfb":        {16, 8, newCast5Stream},
	"rc4-md5":          {16, 16, newRC4MD5Stream},
	"rc4-md5-6":        {16, 6, newRC4MD5Stream},
	"chacha20":         {32, 8, newChaCha20Stream},
	"chacha20-ietf":    {32, 12, newChacha20IETFStream},
	"salsa20":          {32, 8, newSalsa20Stream},
	"camellia-128-cfb": {16, 16, newCamelliaStream},
	"camellia-192-cfb": {24, 16, newCamelliaStream},
	"camellia-256-cfb": {32, 16, newCamelliaStream},
	"idea-cfb":         {16, 8, newIdeaStream},
	"rc2-cfb":          {16, 8, newRC2Stream},
	"seed-cfb":         {16, 8, newSeedStream},
	"rc4":              {16, 0, newRC4Stream},
	"none":             {16, 0, newNoneStream},
}

func CheckCipherMethod(method string) error {
	if method == "" {
		method = "rc4-md5"
	}
	_, ok := streamCipherMethod[method]
	if !ok {
		return errors.New("Unsupported encryption method: " + method)
	}
	return nil
}

type StreamCipher struct {
	enc  cipher.Stream
	dec  cipher.Stream
	key  []byte
	info *cipherInfo
	iv   []byte
}

// NewStreamCipher 创建一个可用于 Dial() 等场景的加密算法实例。
// 使用 cipher.Copy() 以相同方法和密码创建新的加密算法实例，
// 以避免重复初始化加密算法的开销。
func NewStreamCipher(method, password string) (c *StreamCipher, err error) {
	if password == "" {
		return nil, errEmptyPassword
	}
	if method == "" {
		method = "rc4-md5"
	}
	mi, ok := streamCipherMethod[method]
	if !ok {
		return nil, errors.New("Unsupported encryption method: " + method)
	}

	key := tools.EVPBytesToKey(password, mi.keyLen)

	c = &StreamCipher{key: key, info: mi}

	return c, nil
}

func (c *StreamCipher) EncryptInited() bool {
	return c.enc != nil
}

func (c *StreamCipher) DecryptInited() bool {
	return c.dec != nil
}

// 以 CFB 模式初始化分组加密算法，返回初始向量（IV）。
func (c *StreamCipher) InitEncrypt() (iv []byte, err error) {
	if c.iv == nil {
		iv = make([]byte, c.info.ivLen)
		rand.Read(iv)
		c.iv = iv
	} else {
		iv = c.iv
	}
	c.enc, err = c.info.newStream(c.key, iv, Encrypt)
	return
}

func (c *StreamCipher) InitDecrypt(iv []byte) (err error) {
	c.dec, err = c.info.newStream(c.key, iv, Decrypt)
	return
}

func (c *StreamCipher) Encrypt(dst, src []byte) {
	c.enc.XORKeyStream(dst, src)
}

func (c *StreamCipher) Decrypt(dst, src []byte) {
	c.dec.XORKeyStream(dst, src)
}

// Copy 创建一个处于初始状态的新加密算法实例。
func (c *StreamCipher) Copy() *StreamCipher {
	// 此优化可能并非必要。但若没有此函数，则需要为 newTableCipher
	// 维护一个表缓存，并使用锁来保护对该缓存的并发访问。

	// AES 和 DES 加密算法不返回具体类型，因此难以创建副本。
	// 但在我的 2.26 GHz Intel Core 2 Duo 处理器上，其初始化时间不足 4000ns，
	// 因此无需担心。

	// 目前，blowfish 和 cast5 的初始化开销比其他加密算法高出一个数量级。
	// （不确定这是因为当前实现未充分优化，还是算法本身的特性所致。）

	nc := *c
	nc.enc = nil
	nc.dec = nil
	return &nc
}

func (c *StreamCipher) Key() []byte {
	return c.key
}

func (c *StreamCipher) IV() []byte {
	return c.iv
}

func (c *StreamCipher) SetIV(iv []byte) {
	c.iv = iv
}

func (c *StreamCipher) SetKey(key []byte) {
	c.key = key
}

func (c *StreamCipher) InfoIVLen() int {
	return c.info.ivLen
}

func (c *StreamCipher) InfoKeyLen() int {
	return c.info.keyLen
}
