//go:build linux

// Source code from:
// https://github.com/linuxkit/virtsock/tree/master/pkg/vsock
package vsock

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Addr 表示 vsock 端点的地址。
type Addr struct {
	CID  uint32
	Port uint32
}

// Network 返回 Addr 的网络类型。
func (a Addr) Network() string {
	return "vsock"
}

// String 返回 Addr 的字符串表示。
func (a Addr) String() string {
	return fmt.Sprintf("%d:%d", a.CID, a.Port)
}

// Conn 是支持半关闭的 vsock 连接。
type Conn interface {
	net.Conn
	CloseRead() error
	CloseWrite() error
	File() (*os.File, error)
}

// SocketMode 在 Linux 上是空操作。
func SocketMode(m string) {}

// 将通用 unix.Sockaddr 转换为 Addr。
func sockaddrToVsock(sa unix.Sockaddr) *Addr {
	switch sa := sa.(type) {
	case *unix.SockaddrVM:
		return &Addr{CID: sa.CID, Port: sa.Port}
	}
	return nil
}

// 关闭 fd，遇到 EINTR 时重试
func closeFD(fd int) error {
	for {
		if err := unix.Close(fd); err != nil {
			if errno, ok := err.(syscall.Errno); ok && errno == syscall.EINTR {
				continue
			}
			return fmt.Errorf("failed to close() fd %d: %w", fd, err)
		}
		break
	}
	return nil
}

// Dial 通过 virtio 套接字连接到 CID.Port。
func Dial(cid, port uint32) (Conn, error) {
	fd, err := syscall.Socket(unix.AF_VSOCK, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("Failed to create AF_VSOCK socket: %w", err)
	}
	sa := &unix.SockaddrVM{CID: cid, Port: port}
	// 如果遇到 EINTR，在循环中重试 connect。
	for {
		if err := unix.Connect(fd, sa); err != nil {
			if errno, ok := err.(syscall.Errno); ok && errno == syscall.EINTR {
				continue
			}
			// 尝试避免 fd 泄漏
			_ = closeFD(fd)
			return nil, fmt.Errorf("failed connect() to %d:%d: %w", cid, port, err)
		}
		break
	}
	return newVsockConn(uintptr(fd), nil, &Addr{cid, port}), nil
}

// Listen 返回一个 net.Listener，可在给定的 cid 和 port 上接受连接。
func Listen(cid, port uint32) (net.Listener, error) {
	fd, err := syscall.Socket(unix.AF_VSOCK, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}

	sa := &unix.SockaddrVM{CID: cid, Port: port}
	if err = unix.Bind(fd, sa); err != nil {
		return nil, fmt.Errorf("bind() to %d:%d failed: %w", cid, port, err)
	}

	err = syscall.Listen(fd, syscall.SOMAXCONN)
	if err != nil {
		return nil, fmt.Errorf("listen() on %d:%d failed: %w", cid, port, err)
	}
	return &vsockListener{fd, Addr{cid, port}}, nil
}

// ContextID 获取本系统的本地上下文 ID。
func ContextID() (uint32, error) {
	f, err := os.Open("/dev/vsock")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return unix.IoctlGetUint32(int(f.Fd()), unix.IOCTL_VM_SOCKETS_GET_LOCAL_CID)
}

type vsockListener struct {
	fd    int
	local Addr
}

// Accept 接受传入连接并返回新连接。
func (v *vsockListener) Accept() (net.Conn, error) {
	fd, sa, err := unix.Accept(v.fd)
	if err != nil {
		return nil, err
	}
	return newVsockConn(uintptr(fd), &v.local, sockaddrToVsock(sa)), nil
}

// Close 关闭监听连接。
func (v *vsockListener) Close() error {
	// 注意：这不会导致 Accept 解除阻塞。
	return unix.Close(v.fd)
}

// Addr 返回监听器正在监听的地址。
func (v *vsockListener) Addr() net.Addr {
	return v.local
}

// 封装 FileConn 以支持 CloseRead 和 CloseWrite。
type vsockConn struct {
	vsock  *os.File
	fd     uintptr
	local  *Addr
	remote *Addr
}

func newVsockConn(fd uintptr, local, remote *Addr) *vsockConn {
	vsock := os.NewFile(fd, fmt.Sprintf("vsock:%d", fd))
	return &vsockConn{vsock: vsock, fd: fd, local: local, remote: remote}
}

// LocalAddr 返回连接的本地地址。
func (v *vsockConn) LocalAddr() net.Addr {
	return v.local
}

// RemoteAddr 返回连接的远程地址。
func (v *vsockConn) RemoteAddr() net.Addr {
	return v.remote
}

// Close 关闭连接。
func (v *vsockConn) Close() error {
	return v.vsock.Close()
}

// CloseRead 关闭 vsock 连接的读取端。
func (v *vsockConn) CloseRead() error {
	return syscall.Shutdown(int(v.fd), syscall.SHUT_RD)
}

// CloseWrite 关闭 vsock 连接的写入端。
func (v *vsockConn) CloseWrite() error {
	return syscall.Shutdown(int(v.fd), syscall.SHUT_WR)
}

// Read 从连接读取数据。
func (v *vsockConn) Read(buf []byte) (int, error) {
	return v.vsock.Read(buf)
}

// Write 通过连接写入数据。
func (v *vsockConn) Write(buf []byte) (int, error) {
	return v.vsock.Write(buf)
}

// SetDeadline 设置与连接关联的读写超时时间。
func (v *vsockConn) SetDeadline(t time.Time) error {
	return nil // FIXME
}

// SetReadDeadline 设置未来 Read 调用的超时时间。
func (v *vsockConn) SetReadDeadline(t time.Time) error {
	return nil // FIXME
}

// SetWriteDeadline 设置未来 Write 调用的超时时间。
func (v *vsockConn) SetWriteDeadline(t time.Time) error {
	return nil // FIXME
}

// File 复制底层套接字描述符并返回它。
func (v *vsockConn) File() (*os.File, error) {
	// 等价于 dup(2)，但创建新 fd 时已设置 CLOEXEC。
	r0, _, e1 := syscall.Syscall(syscall.SYS_FCNTL, uintptr(v.vsock.Fd()), syscall.F_DUPFD_CLOEXEC, 0)
	if e1 != 0 {
		return nil, os.NewSyscallError("fcntl", e1)
	}
	return os.NewFile(r0, v.vsock.Name()), nil
}
