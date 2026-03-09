package dns

import (
	"sync"
	"time"
)

// LruCache 是 LRU 缓存的结构体。
type LruCache struct {
	mu    sync.Mutex
	size  int
	head  *item
	tail  *item
	cache map[string]*item
	store map[string][]byte
}

// item 是缓存条目的结构体。
type item struct {
	key  string
	val  []byte
	exp  int64
	prev *item
	next *item
}

// NewLruCache 返回一个新的 LruCache 实例。
func NewLruCache(size int) *LruCache {
	// 此处初始化 2 个条目，缓存满时它们会被删除，所以没有影响
	head, tail := &item{key: "head"}, &item{key: "tail"}
	head.next, tail.prev = tail, head
	c := &LruCache{
		size:  size,
		head:  head,
		tail:  tail,
		cache: make(map[string]*item, size),
		store: make(map[string][]byte),
	}
	c.cache[head.key], c.cache[tail.key] = head, tail
	return c
}

// Get 从缓存中获取一个条目。
func (c *LruCache) Get(k string) (v []byte, expired bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := c.store[k]; ok {
		return v, false
	}

	if it, ok := c.cache[k]; ok {
		v = it.val
		if it.exp < time.Now().Unix() {
			expired = true
		}
		c.moveToHead(it)
	}
	return
}

// Set 以键、值和 TTL（秒）设置一个缓存条目。
// 若 TTL 为零，该条目将被永久保留，不会被删除。
// 若键已存在，则更新其值和过期时间，并将其移至链表头部。
// 若键不存在，则在链表头部插入一个新条目。
// 最后，若缓存已满，则移除链表尾部的条目。
func (c *LruCache) Set(k string, v []byte, ttl int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ttl == 0 {
		c.store[k] = v
		return
	}

	exp := time.Now().Add(time.Second * time.Duration(ttl)).Unix()
	if it, ok := c.cache[k]; ok {
		it.val = v
		it.exp = exp
		c.moveToHead(it)
		return
	}

	c.putToHead(k, v, exp)

	// 注意：缓存大小始终 >= 2，
	// 但在本使用场景中不影响正确性。
	if len(c.cache) > c.size {
		c.removeTail()
	}
}

// putToHead 将新条目插入到缓存链表的头部。
func (c *LruCache) putToHead(k string, v []byte, exp int64) {
	it := &item{key: k, val: v, exp: exp, prev: nil, next: c.head}
	it.prev = nil
	it.next = c.head
	c.head.prev = it
	c.head = it

	c.cache[k] = it
}

// moveToHead 将已有条目移动到缓存链表的头部。
func (c *LruCache) moveToHead(it *item) {
	if it != c.head {
		if c.tail == it {
			c.tail = it.prev
			c.tail.next = nil
		} else {
			it.prev.next = it.next
			it.next.prev = it.prev
		}
		it.prev = nil
		it.next = c.head
		c.head.prev = it
		c.head = it
	}
}

// removeTail 从缓存中移除链表尾部的条目。
func (c *LruCache) removeTail() {
	delete(c.cache, c.tail.key)

	c.tail.prev.next = nil
	c.tail = c.tail.prev
}
