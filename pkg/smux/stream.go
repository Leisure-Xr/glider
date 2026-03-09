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
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/pool"
)

// wrapper for GC
type Stream struct {
	*stream
}

// Stream 实现 net.Conn 接口
type stream struct {
	id   uint32 // Stream identifier
	sess *Session

	buffers [][]byte // the sequential buffers of stream
	heads   [][]byte // slice heads of the buffers above, kept for recycle

	bufferLock sync.Mutex // Mutex to protect access to buffers
	frameSize  int        // Maximum frame size for the stream

	// notify a read event
	chReadEvent chan struct{}

	// flag the stream has closed
	die     chan struct{}
	dieOnce sync.Once // Ensures die channel is closed only once

	// FIN command
	chFinEvent   chan struct{}
	finEventOnce sync.Once // Ensures chFinEvent is closed only once

	// deadlines
	readDeadline  atomic.Value
	writeDeadline atomic.Value

	// per stream sliding window control
	numRead    uint32 // count num of bytes read
	numWritten uint32 // count num of bytes written
	incr       uint32 // bytes sent since last window update

	// UPD command
	peerConsumed uint32        // num of bytes the peer has consumed
	peerWindow   uint32        // peer window, initialized to 256KB, updated by peer
	chUpdate     chan struct{} // notify of remote data consuming and window update
}

// newStream initializes and returns a new Stream.
func newStream(id uint32, frameSize int, sess *Session) *stream {
	s := new(stream)
	s.id = id
	s.chReadEvent = make(chan struct{}, 1)
	s.chUpdate = make(chan struct{}, 1)
	s.frameSize = frameSize
	s.sess = sess
	s.die = make(chan struct{})
	s.chFinEvent = make(chan struct{})
	s.peerWindow = initialPeerWindow // set to initial window size

	return s
}

// ID 返回流的唯一标识符。
func (s *stream) ID() uint32 {
	return s.id
}

// Read 从流中读取数据到提供的缓冲区。
func (s *stream) Read(b []byte) (n int, err error) {
	for {
		n, err = s.tryRead(b)
		if err == ErrWouldBlock {
			if ew := s.waitRead(); ew != nil {
				return 0, ew
			}
		} else {
			return n, err
		}
	}
}

// tryRead attempts to read data from the stream without blocking.
func (s *stream) tryRead(b []byte) (n int, err error) {
	if s.sess.config.Version == 2 {
		return s.tryReadv2(b)
	}

	if len(b) == 0 {
		return 0, nil
	}

	// A critical section to copy data from buffers to
	s.bufferLock.Lock()
	if len(s.buffers) > 0 {
		n = copy(b, s.buffers[0])
		s.buffers[0] = s.buffers[0][n:]
		if len(s.buffers[0]) == 0 {
			s.buffers[0] = nil
			s.buffers = s.buffers[1:]
			// full recycle
			pool.PutBuffer(s.heads[0])
			s.heads = s.heads[1:]
		}
	}
	s.bufferLock.Unlock()

	if n > 0 {
		s.sess.returnTokens(n)
		return n, nil
	}

	select {
	case <-s.die:
		return 0, io.EOF
	default:
		return 0, ErrWouldBlock
	}
}

// tryReadv2 is the non-blocking version of Read for version 2 streams.
func (s *stream) tryReadv2(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	var notifyConsumed uint32
	s.bufferLock.Lock()
	if len(s.buffers) > 0 {
		n = copy(b, s.buffers[0])
		s.buffers[0] = s.buffers[0][n:]
		if len(s.buffers[0]) == 0 {
			s.buffers[0] = nil
			s.buffers = s.buffers[1:]
			// full recycle
			pool.PutBuffer(s.heads[0])
			s.heads = s.heads[1:]
		}
	}

	// in an ideal environment:
	// if more than half of buffer has consumed, send read ack to peer
	// based on round-trip time of ACK, continous flowing data
	// won't slow down due to waiting for ACK, as long as the
	// consumer keeps on reading data.
	//
	// s.numRead == n implies that it's the initial reading
	s.numRead += uint32(n)
	s.incr += uint32(n)

	// for initial reading, send window update
	if s.incr >= uint32(s.sess.config.MaxStreamBuffer/2) || s.numRead == uint32(n) {
		notifyConsumed = s.numRead
		s.incr = 0 // reset couting for next window update
	}
	s.bufferLock.Unlock()

	if n > 0 {
		s.sess.returnTokens(n)

		// send window update if necessary
		if notifyConsumed > 0 {
			err := s.sendWindowUpdate(notifyConsumed)
			return n, err
		} else {
			return n, nil
		}
	}

	select {
	case <-s.die:
		return 0, io.EOF
	default:
		return 0, ErrWouldBlock
	}
}

// WriteTo 实现 io.WriteTo 接口
// WriteTo 将数据写入 w，直到没有更多数据或发生错误。
// 返回值 n 为写入的字节数，同时返回写入过程中遇到的任何错误。
// WriteTo 循环调用 Write 直到没有更多数据可写或发生错误。
// 若底层流为 v2 流，必要时会向对端发送窗口更新。
// 若底层流为 v1 流，则不会向对端发送窗口更新。
func (s *stream) WriteTo(w io.Writer) (n int64, err error) {
	if s.sess.config.Version == 2 {
		return s.writeTov2(w)
	}

	for {
		var buf []byte
		s.bufferLock.Lock()
		if len(s.buffers) > 0 {
			buf = s.buffers[0]
			s.buffers = s.buffers[1:]
			s.heads = s.heads[1:]
		}
		s.bufferLock.Unlock()

		if buf != nil {
			nw, ew := w.Write(buf)
			// NOTE: WriteTo is a reader, so we need to return tokens here
			s.sess.returnTokens(len(buf))
			pool.PutBuffer(buf)
			if nw > 0 {
				n += int64(nw)
			}

			if ew != nil {
				return n, ew
			}
		} else if ew := s.waitRead(); ew != nil {
			return n, ew
		}
	}
}

// check comments in WriteTo
func (s *stream) writeTov2(w io.Writer) (n int64, err error) {
	for {
		var notifyConsumed uint32
		var buf []byte
		s.bufferLock.Lock()
		if len(s.buffers) > 0 {
			buf = s.buffers[0]
			s.buffers = s.buffers[1:]
			s.heads = s.heads[1:]
		}
		s.numRead += uint32(len(buf))
		s.incr += uint32(len(buf))
		if s.incr >= uint32(s.sess.config.MaxStreamBuffer/2) || s.numRead == uint32(len(buf)) {
			notifyConsumed = s.numRead
			s.incr = 0
		}
		s.bufferLock.Unlock()

		if buf != nil {
			nw, ew := w.Write(buf)
			// NOTE: WriteTo is a reader, so we need to return tokens here
			s.sess.returnTokens(len(buf))
			pool.PutBuffer(buf)
			if nw > 0 {
				n += int64(nw)
			}

			if ew != nil {
				return n, ew
			}

			if notifyConsumed > 0 {
				if err := s.sendWindowUpdate(notifyConsumed); err != nil {
					return n, err
				}
			}
		} else if ew := s.waitRead(); ew != nil {
			return n, ew
		}
	}
}

// sendWindowUpdate sends a window update frame to the peer.
func (s *stream) sendWindowUpdate(consumed uint32) error {
	var timer *time.Timer
	var deadline <-chan time.Time
	if d, ok := s.readDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer = time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	frame := newFrame(byte(s.sess.config.Version), cmdUPD, s.id)
	var hdr updHeader
	binary.LittleEndian.PutUint32(hdr[:], consumed)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(s.sess.config.MaxStreamBuffer))
	frame.data = hdr[:]
	_, err := s.sess.writeFrameInternal(frame, deadline, CLSCTRL)
	return err
}

// waitRead blocks until a read event occurs or a deadline is reached.
func (s *stream) waitRead() error {
	var timer *time.Timer
	var deadline <-chan time.Time
	if d, ok := s.readDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer = time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case <-s.chReadEvent: // notify some data has arrived, or closed
		return nil
	case <-s.chFinEvent:
		// BUGFIX(xtaci): Fix for https://github.com/xtaci/smux/issues/82
		s.bufferLock.Lock()
		defer s.bufferLock.Unlock()
		if len(s.buffers) > 0 {
			return nil
		}
		return io.EOF
	case <-s.sess.chSocketReadError:
		return s.sess.socketReadError.Load().(error)
	case <-s.sess.chProtoError:
		return s.sess.protoError.Load().(error)
	case <-deadline:
		return ErrTimeout
	case <-s.die:
		return io.ErrClosedPipe
	}

}

// Write 实现 net.Conn 接口
//
// 注意：多个 goroutine 并发写入时的行为是不确定的，
// frames may interleave in random way.
func (s *stream) Write(b []byte) (n int, err error) {
	if s.sess.config.Version == 2 {
		return s.writeV2(b)
	}

	var deadline <-chan time.Time
	if d, ok := s.writeDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer := time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	// check if stream has closed
	select {
	case <-s.chFinEvent: // passive closing
		return 0, io.EOF
	case <-s.die:
		return 0, io.ErrClosedPipe
	default:
	}

	// frame split and transmit
	sent := 0
	frame := newFrame(byte(s.sess.config.Version), cmdPSH, s.id)
	bts := b
	for len(bts) > 0 {
		sz := len(bts)
		if sz > s.frameSize {
			sz = s.frameSize
		}
		frame.data = bts[:sz]
		bts = bts[sz:]
		n, err := s.sess.writeFrameInternal(frame, deadline, CLSDATA)
		s.numWritten++
		sent += n
		if err != nil {
			return sent, err
		}
	}

	return sent, nil
}

// writeV2 writes data to the stream for version 2 streams.
func (s *stream) writeV2(b []byte) (n int, err error) {
	// check empty input
	if len(b) == 0 {
		return 0, nil
	}

	// check if stream has closed
	select {
	case <-s.chFinEvent:
		return 0, io.EOF
	case <-s.die:
		return 0, io.ErrClosedPipe
	default:
	}

	// create write deadline timer
	var deadline <-chan time.Time
	if d, ok := s.writeDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer := time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	// frame split and transmit process
	sent := 0
	frame := newFrame(byte(s.sess.config.Version), cmdPSH, s.id)

	for {
		// per stream sliding window control
		// [.... [consumed... numWritten] ... win... ]
		// [.... [consumed...................+rmtwnd]]
		var bts []byte
		// note:
		// even if uint32 overflow, this math still works:
		// eg1: uint32(0) - uint32(math.MaxUint32) = 1
		// eg2: int32(uint32(0) - uint32(1)) = -1
		//
		// basicially, you can take it as a MODULAR ARITHMETIC
		inflight := int32(atomic.LoadUint32(&s.numWritten) - atomic.LoadUint32(&s.peerConsumed))
		if inflight < 0 { // security check for malformed data
			return 0, ErrConsumed
		}

		// make sure you understand 'win' is calculated in modular arithmetic(2^32(4GB))
		win := int32(atomic.LoadUint32(&s.peerWindow)) - inflight

		if win > 0 {
			// determine how many bytes to send
			if win > int32(len(b)) {
				bts = b
				b = nil
			} else {
				bts = b[:win]
				b = b[win:]
			}

			// frame split and transmit
			for len(bts) > 0 {
				// splitting frame
				sz := len(bts)
				if sz > s.frameSize {
					sz = s.frameSize
				}
				frame.data = bts[:sz]
				bts = bts[sz:]

				// transmit of frame
				n, err := s.sess.writeFrameInternal(frame, deadline, CLSDATA)
				atomic.AddUint32(&s.numWritten, uint32(sz))
				sent += n
				if err != nil {
					return sent, err
				}
			}
		}

		// if there is any data left to be sent,
		// wait until stream closes, window changes or deadline reached
		// this blocking behavior will back propagate flow control to upper layer.
		if len(b) > 0 {
			select {
			case <-s.chFinEvent:
				return 0, io.EOF
			case <-s.die:
				return sent, io.ErrClosedPipe
			case <-deadline:
				return sent, ErrTimeout
			case <-s.sess.chSocketWriteError:
				return sent, s.sess.socketWriteError.Load().(error)
			case <-s.chUpdate: // notify of remote data consuming and window update
				continue
			}
		} else {
			return sent, nil
		}
	}
}

// Close 实现 net.Conn 接口
func (s *stream) Close() error {
	var once bool
	var err error
	s.dieOnce.Do(func() {
		close(s.die)
		once = true
	})

	if once {
		// send FIN in order
		f := newFrame(byte(s.sess.config.Version), cmdFIN, s.id)

		timer := time.NewTimer(openCloseTimeout)
		defer timer.Stop()

		_, err = s.sess.writeFrameInternal(f, timer.C, CLSDATA)
		s.sess.streamClosed(s.id)
		return err
	} else {
		return io.ErrClosedPipe
	}
}

// GetDieCh 返回一个只读 channel，可在流关闭时读取
// when the stream is to be closed.
func (s *stream) GetDieCh() <-chan struct{} {
	return s.die
}

// SetReadDeadline 设置读取截止时间，定义见
// net.Conn.SetReadDeadline.
// 零值表示禁用截止时间。
func (s *stream) SetReadDeadline(t time.Time) error {
	s.readDeadline.Store(t)
	s.notifyReadEvent()
	return nil
}

// SetWriteDeadline 设置写入截止时间，定义见
// net.Conn.SetWriteDeadline.
// 零值表示禁用截止时间。
func (s *stream) SetWriteDeadline(t time.Time) error {
	s.writeDeadline.Store(t)
	return nil
}

// SetDeadline 同时设置读写截止时间，定义见
// net.Conn.SetDeadline.
// 零值表示禁用截止时间。.
func (s *stream) SetDeadline(t time.Time) error {
	if err := s.SetReadDeadline(t); err != nil {
		return err
	}
	if err := s.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

// session closes
func (s *stream) sessionClose() { s.dieOnce.Do(func() { close(s.die) }) }

// LocalAddr 实现 net.Conn 接口
func (s *stream) LocalAddr() net.Addr {
	if ts, ok := s.sess.conn.(interface {
		LocalAddr() net.Addr
	}); ok {
		return ts.LocalAddr()
	}
	return nil
}

// RemoteAddr 实现 net.Conn 接口
func (s *stream) RemoteAddr() net.Addr {
	if ts, ok := s.sess.conn.(interface {
		RemoteAddr() net.Addr
	}); ok {
		return ts.RemoteAddr()
	}
	return nil
}

// pushBytes append buf to buffers
func (s *stream) pushBytes(buf []byte) (written int, err error) {
	s.bufferLock.Lock()
	s.buffers = append(s.buffers, buf)
	s.heads = append(s.heads, buf)
	s.bufferLock.Unlock()
	return
}

// recycleTokens transform remaining bytes to tokens(will truncate buffer)
func (s *stream) recycleTokens() (n int) {
	s.bufferLock.Lock()
	for k := range s.buffers {
		n += len(s.buffers[k])
		pool.PutBuffer(s.heads[k])
	}
	s.buffers = nil
	s.heads = nil
	s.bufferLock.Unlock()
	return
}

// notify read event
func (s *stream) notifyReadEvent() {
	select {
	case s.chReadEvent <- struct{}{}:
	default:
	}
}

// update command
func (s *stream) update(consumed uint32, window uint32) {
	atomic.StoreUint32(&s.peerConsumed, consumed)
	atomic.StoreUint32(&s.peerWindow, window)
	select {
	case s.chUpdate <- struct{}{}:
	default:
	}
}

// mark this stream has been closed in protocol
func (s *stream) fin() {
	s.finEventOnce.Do(func() {
		close(s.chFinEvent)
	})
}
