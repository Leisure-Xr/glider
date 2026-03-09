// MIT 许可证
//
// 版权所有 (c) 2016-2017 xtaci
//
// 特此免费授予任何获得本软件及相关文档文件（"软件"）副本的人不受限制地处理
// 本软件的权利，包括但不限于使用、复制、修改、合并、发布、分发、再许可和/或出售
// 本软件副本的权利，以及允许获得本软件的人员这样做，但须符合以下条件：
//
// 上述版权声明和本许可声明应包含在本软件的所有
// 副本或主要部分中。
//
// 本软件按"原样"提供，不提供任何形式的明示或
// 暗示担保，包括但不限于适销性、特定用途适用性和非侵权性担保。在任何情况下，
// 作者或版权持有人均不对任何索赔、损害或其他
// 责任负责，无论是在合同诉讼、侵权诉讼或其他诉讼中，均由软件或软件的使用或其他
// 软件交易引起或与之相关。

package smux

import (
	"container/heap"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/pool"
)

const (
	defaultAcceptBacklog = 1024
	maxShaperSize        = 1024
	openCloseTimeout     = 30 * time.Second // 打开/关闭流的超时时间
)

// CLASSID 表示帧的分类
type CLASSID int

const (
	CLSCTRL CLASSID = iota // 优先级更高的控制信号
	CLSDATA
)

// timeoutError 表示 accept、read、write 等操作的超时错误
//
// 为了更好地与标准库协作，timeoutError 应实现标准库的 `net.Error` 接口。
//
// 例如，使用 smux 实现 net.Listener 并与 http.Server 配合时，心跳保活连接（*smux.Stream）会被意外关闭。
// 详情请参见 https://github.com/xtaci/smux/pull/99。
type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Temporary() bool { return true }
func (timeoutError) Timeout() bool   { return true }

var (
	ErrInvalidProtocol           = errors.New("invalid protocol")
	ErrConsumed                  = errors.New("peer consumed more than sent")
	ErrGoAway                    = errors.New("stream id overflows, should start a new connection")
	ErrTimeout         net.Error = &timeoutError{}
	ErrWouldBlock                = errors.New("operation would block on IO")
)

// writeRequest 表示一个写入帧的请求
type writeRequest struct {
	class  CLASSID
	frame  Frame
	seq    uint32
	result chan writeResult
}

// writeResult represents the result of a write request
type writeResult struct {
	n   int
	err error
}

// Session 定义了一个用于多路复用流的连接
type Session struct {
	conn io.ReadWriteCloser

	config           *Config
	nextStreamID     uint32 // next stream identifier
	nextStreamIDLock sync.Mutex

	bucket       int32         // token bucket
	bucketNotify chan struct{} // used for waiting for tokens

	streams    map[uint32]*stream // all streams in this session
	streamLock sync.Mutex         // locks streams

	die     chan struct{} // flag session has died
	dieOnce sync.Once

	// socket error handling
	socketReadError      atomic.Value
	socketWriteError     atomic.Value
	chSocketReadError    chan struct{}
	chSocketWriteError   chan struct{}
	socketReadErrorOnce  sync.Once
	socketWriteErrorOnce sync.Once

	// smux protocol errors
	protoError     atomic.Value
	chProtoError   chan struct{}
	protoErrorOnce sync.Once

	chAccepts chan *stream

	dataReady int32 // flag data has arrived

	goAway int32 // flag id exhausted

	deadline atomic.Value

	requestID uint32            // Monotonic increasing write request ID
	shaper    chan writeRequest // a shaper for writing
	writes    chan writeRequest
}

func newSession(config *Config, conn io.ReadWriteCloser, client bool) *Session {
	s := new(Session)
	s.die = make(chan struct{})
	s.conn = conn
	s.config = config
	s.streams = make(map[uint32]*stream)
	s.chAccepts = make(chan *stream, defaultAcceptBacklog)
	s.bucket = int32(config.MaxReceiveBuffer)
	s.bucketNotify = make(chan struct{}, 1)
	s.shaper = make(chan writeRequest)
	s.writes = make(chan writeRequest)
	s.chSocketReadError = make(chan struct{})
	s.chSocketWriteError = make(chan struct{})
	s.chProtoError = make(chan struct{})

	if client {
		s.nextStreamID = 1
	} else {
		s.nextStreamID = 0
	}

	go s.shaperLoop()
	go s.recvLoop()
	go s.sendLoop()
	if !config.KeepAliveDisabled {
		go s.keepalive()
	}
	return s
}

// OpenStream 用于创建一个新的流
func (s *Session) OpenStream() (*Stream, error) {
	if s.IsClosed() {
		return nil, io.ErrClosedPipe
	}

	// generate stream id
	s.nextStreamIDLock.Lock()
	if s.goAway > 0 {
		s.nextStreamIDLock.Unlock()
		return nil, ErrGoAway
	}

	s.nextStreamID += 2
	sid := s.nextStreamID
	if sid == sid%2 { // stream-id overflows
		s.goAway = 1
		s.nextStreamIDLock.Unlock()
		return nil, ErrGoAway
	}
	s.nextStreamIDLock.Unlock()

	stream := newStream(sid, s.config.MaxFrameSize, s)

	if _, err := s.writeControlFrame(newFrame(byte(s.config.Version), cmdSYN, sid)); err != nil {
		return nil, err
	}

	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	select {
	case <-s.chSocketReadError:
		return nil, s.socketReadError.Load().(error)
	case <-s.chSocketWriteError:
		return nil, s.socketWriteError.Load().(error)
	case <-s.die:
		return nil, io.ErrClosedPipe
	default:
		s.streams[sid] = stream
		wrapper := &Stream{stream: stream}
		// NOTE(x): disabled finalizer for issue #997
		/*
			runtime.SetFinalizer(wrapper, func(s *Stream) {
				s.Close()
			})
		*/
		return wrapper, nil
	}
}

// Open 返回一个通用的 ReadWriteCloser
func (s *Session) Open() (io.ReadWriteCloser, error) {
	return s.OpenStream()
}

// AcceptStream 用于阻塞直到下一个可用的流
// is ready to be accepted.
func (s *Session) AcceptStream() (*Stream, error) {
	var deadline <-chan time.Time
	if d, ok := s.deadline.Load().(time.Time); ok && !d.IsZero() {
		timer := time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case stream := <-s.chAccepts:
		wrapper := &Stream{stream: stream}
		runtime.SetFinalizer(wrapper, func(s *Stream) {
			s.Close()
		})
		return wrapper, nil
	case <-deadline:
		return nil, ErrTimeout
	case <-s.chSocketReadError:
		return nil, s.socketReadError.Load().(error)
	case <-s.chProtoError:
		return nil, s.protoError.Load().(error)
	case <-s.die:
		return nil, io.ErrClosedPipe
	}
}

// Accept 返回通用的 ReadWriteCloser 而非 smux.Stream
func (s *Session) Accept() (io.ReadWriteCloser, error) {
	return s.AcceptStream()
}

// Close 用于关闭会话及其所有流。
func (s *Session) Close() error {
	var once bool
	s.dieOnce.Do(func() {
		close(s.die)
		once = true
	})

	if once {
		s.streamLock.Lock()
		for k := range s.streams {
			s.streams[k].sessionClose()
		}
		s.streamLock.Unlock()
		return s.conn.Close()
	} else {
		return io.ErrClosedPipe
	}
}

// CloseChan 可供需要在会话关闭时立即收到通知的调用方使用
// session is closed
func (s *Session) CloseChan() <-chan struct{} {
	return s.die
}

// notifyBucket notifies recvLoop that bucket is available
func (s *Session) notifyBucket() {
	select {
	case s.bucketNotify <- struct{}{}:
	default:
	}
}

func (s *Session) notifyReadError(err error) {
	s.socketReadErrorOnce.Do(func() {
		s.socketReadError.Store(err)
		close(s.chSocketReadError)
	})
}

func (s *Session) notifyWriteError(err error) {
	s.socketWriteErrorOnce.Do(func() {
		s.socketWriteError.Store(err)
		close(s.chSocketWriteError)
	})
}

func (s *Session) notifyProtoError(err error) {
	s.protoErrorOnce.Do(func() {
		s.protoError.Store(err)
		close(s.chProtoError)
	})
}

// IsClosed 安全检查会话是否已关闭
func (s *Session) IsClosed() bool {
	select {
	case <-s.die:
		return true
	default:
		return false
	}
}

// NumStreams 返回当前打开的流数量
func (s *Session) NumStreams() int {
	if s.IsClosed() {
		return 0
	}
	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	return len(s.streams)
}

// SetDeadline 设置 Accept* 调用使用的截止时间。
// 零值表示禁用截止时间。
func (s *Session) SetDeadline(t time.Time) error {
	s.deadline.Store(t)
	return nil
}

// LocalAddr 实现 net.Conn 接口
func (s *Session) LocalAddr() net.Addr {
	if ts, ok := s.conn.(interface {
		LocalAddr() net.Addr
	}); ok {
		return ts.LocalAddr()
	}
	return nil
}

// RemoteAddr 实现 net.Conn 接口
func (s *Session) RemoteAddr() net.Addr {
	if ts, ok := s.conn.(interface {
		RemoteAddr() net.Addr
	}); ok {
		return ts.RemoteAddr()
	}
	return nil
}

// notify the session that a stream has closed
func (s *Session) streamClosed(sid uint32) {
	s.streamLock.Lock()
	if stream, ok := s.streams[sid]; ok {
		n := stream.recycleTokens()
		if n > 0 { // return remaining tokens to the bucket
			if atomic.AddInt32(&s.bucket, int32(n)) > 0 {
				s.notifyBucket()
			}
		}
		delete(s.streams, sid)
	}
	s.streamLock.Unlock()
}

// returnTokens is called by stream to return token after read
func (s *Session) returnTokens(n int) {
	if atomic.AddInt32(&s.bucket, int32(n)) > 0 {
		s.notifyBucket()
	}
}

// recvLoop keeps on reading from underlying connection if tokens are available
func (s *Session) recvLoop() {
	var hdr rawHeader
	var updHdr updHeader

	for {
		for atomic.LoadInt32(&s.bucket) <= 0 && !s.IsClosed() {
			select {
			case <-s.bucketNotify:
			case <-s.die:
				return
			}
		}

		// read header first
		if _, err := io.ReadFull(s.conn, hdr[:]); err == nil {
			atomic.StoreInt32(&s.dataReady, 1)
			if hdr.Version() != byte(s.config.Version) {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
			sid := hdr.StreamID()
			switch hdr.Cmd() {
			case cmdNOP:
			case cmdSYN: // stream opening
				s.streamLock.Lock()
				if _, ok := s.streams[sid]; !ok {
					stream := newStream(sid, s.config.MaxFrameSize, s)
					s.streams[sid] = stream
					select {
					case s.chAccepts <- stream:
					case <-s.die:
					}
				}
				s.streamLock.Unlock()
			case cmdFIN: // stream closing
				s.streamLock.Lock()
				if stream, ok := s.streams[sid]; ok {
					stream.fin()
					stream.notifyReadEvent()
				}
				s.streamLock.Unlock()
			case cmdPSH: // data frame
				if hdr.Length() > 0 {
					newbuf := pool.GetBuffer(int(hdr.Length()))
					if written, err := io.ReadFull(s.conn, newbuf); err == nil {
						s.streamLock.Lock()
						if stream, ok := s.streams[sid]; ok {
							stream.pushBytes(newbuf)
							// a stream used some token
							atomic.AddInt32(&s.bucket, -int32(written))
							stream.notifyReadEvent()
						} else {
							// data directed to a missing/closed stream, recycle the buffer immediately.
							pool.PutBuffer(newbuf)
						}
						s.streamLock.Unlock()
					} else {
						s.notifyReadError(err)
						return
					}
				}
			case cmdUPD: // a window update signal
				if _, err := io.ReadFull(s.conn, updHdr[:]); err == nil {
					s.streamLock.Lock()
					if stream, ok := s.streams[sid]; ok {
						stream.update(updHdr.Consumed(), updHdr.Window())
					}
					s.streamLock.Unlock()
				} else {
					s.notifyReadError(err)
					return
				}
			default:
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
		} else {
			s.notifyReadError(err)
			return
		}
	}
}

// keepalive sends NOP frame to peer to keep the connection alive, and detect dead peers
func (s *Session) keepalive() {
	tickerPing := time.NewTicker(s.config.KeepAliveInterval)
	tickerTimeout := time.NewTicker(s.config.KeepAliveTimeout)
	defer tickerPing.Stop()
	defer tickerTimeout.Stop()
	for {
		select {
		case <-tickerPing.C:
			s.writeFrameInternal(newFrame(byte(s.config.Version), cmdNOP, 0), tickerPing.C, CLSCTRL)
			s.notifyBucket() // force a signal to the recvLoop
		case <-tickerTimeout.C:
			if !atomic.CompareAndSwapInt32(&s.dataReady, 1, 0) {
				// recvLoop may block while bucket is 0, in this case,
				// session should not be closed.
				if atomic.LoadInt32(&s.bucket) > 0 {
					s.Close()
					return
				}
			}
		case <-s.die:
			return
		}
	}
}

// shaperLoop implements a priority queue for write requests,
// some control messages are prioritized over data messages
func (s *Session) shaperLoop() {
	var reqs shaperHeap
	var next writeRequest
	var chWrite chan writeRequest
	var chShaper chan writeRequest

	for {
		// chWrite is not available until it has packet to send
		if len(reqs) > 0 {
			chWrite = s.writes
			next = heap.Pop(&reqs).(writeRequest)
		} else {
			chWrite = nil
		}

		// control heap size, chShaper is not available until packets are less than maximum allowed
		if len(reqs) >= maxShaperSize {
			chShaper = nil
		} else {
			chShaper = s.shaper
		}

		// assertion on non nil
		if chShaper == nil && chWrite == nil {
			panic("both channel are nil")
		}

		select {
		case <-s.die:
			return
		case r := <-chShaper:
			if chWrite != nil { // next is valid, reshape
				heap.Push(&reqs, next)
			}
			heap.Push(&reqs, r)
		case chWrite <- next:
		}
	}
}

// sendLoop sends frames to the underlying connection
func (s *Session) sendLoop() {
	var buf []byte
	var n int
	var err error
	var vec [][]byte // vector for writeBuffers

	bw, ok := s.conn.(interface {
		WriteBuffers(v [][]byte) (n int, err error)
	})

	if ok {
		buf = make([]byte, headerSize)
		vec = make([][]byte, 2)
	} else {
		buf = make([]byte, (1<<16)+headerSize)
	}

	for {
		select {
		case <-s.die:
			return
		case request := <-s.writes:
			buf[0] = request.frame.ver
			buf[1] = request.frame.cmd
			binary.LittleEndian.PutUint16(buf[2:], uint16(len(request.frame.data)))
			binary.LittleEndian.PutUint32(buf[4:], request.frame.sid)

			// support for scatter-gather I/O
			if len(vec) > 0 {
				vec[0] = buf[:headerSize]
				vec[1] = request.frame.data
				n, err = bw.WriteBuffers(vec)
			} else {
				copy(buf[headerSize:], request.frame.data)
				n, err = s.conn.Write(buf[:headerSize+len(request.frame.data)])
			}

			n -= headerSize
			if n < 0 {
				n = 0
			}

			result := writeResult{
				n:   n,
				err: err,
			}

			request.result <- result
			close(request.result)

			// store conn error
			if err != nil {
				s.notifyWriteError(err)
				return
			}
		}
	}
}

// writeControlFrame writes the control frame to the underlying connection
// and returns the number of bytes written if successful
func (s *Session) writeControlFrame(f Frame) (n int, err error) {
	timer := time.NewTimer(openCloseTimeout)
	defer timer.Stop()

	return s.writeFrameInternal(f, timer.C, CLSCTRL)
}

// internal writeFrame version to support deadline used in keepalive
func (s *Session) writeFrameInternal(f Frame, deadline <-chan time.Time, class CLASSID) (int, error) {
	req := writeRequest{
		class:  class,
		frame:  f,
		seq:    atomic.AddUint32(&s.requestID, 1),
		result: make(chan writeResult, 1),
	}
	select {
	case s.shaper <- req:
	case <-s.die:
		return 0, io.ErrClosedPipe
	case <-s.chSocketWriteError:
		return 0, s.socketWriteError.Load().(error)
	case <-deadline:
		return 0, ErrTimeout
	}

	select {
	case result := <-req.result:
		return result.n, result.err
	case <-s.die:
		return 0, io.ErrClosedPipe
	case <-s.chSocketWriteError:
		return 0, s.socketWriteError.Load().(error)
	case <-deadline:
		return 0, ErrTimeout
	}
}
