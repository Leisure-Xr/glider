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
	"fmt"
)

const ( // 命令
	// 协议版本 1：
	cmdSYN byte = iota // 流 打开
	cmdFIN             // 流 关闭，即 EOF 标记
	cmdPSH             // 数据推送
	cmdNOP             // 无操作

	// 协议版本 2 额外命令
	// 通知远端对等方已消费的字节数
	cmdUPD
)

const (
	// cmdUPD 的数据大小，格式：
	// |4字节 已消费数据(ACK)| 4字节 窗口大小(WINDOW) |
	szCmdUPD = 8
)

const (
	// 初始对端窗口估算值，采用慢启动策略
	initialPeerWindow = 262144
)

const (
	sizeOfVer    = 1
	sizeOfCmd    = 1
	sizeOfLength = 2
	sizeOfSid    = 4
	headerSize   = sizeOfVer + sizeOfCmd + sizeOfSid + sizeOfLength
)

// Frame 定义了一个将被多路复用到单一连接中的数据包
type Frame struct {
	ver  byte   // 版本
	cmd  byte   // 命令
	sid  uint32 // 流 ID
	data []byte // 载荷
}

// newFrame 使用给定的版本、命令和流 ID 创建一个新帧
func newFrame(version byte, cmd byte, sid uint32) Frame {
	return Frame{ver: version, cmd: cmd, sid: sid}
}

// rawHeader 是帧头的字节数组表示
type rawHeader [headerSize]byte

func (h rawHeader) Version() byte {
	return h[0]
}

func (h rawHeader) Cmd() byte {
	return h[1]
}

func (h rawHeader) Length() uint16 {
	return binary.LittleEndian.Uint16(h[2:])
}

func (h rawHeader) StreamID() uint32 {
	return binary.LittleEndian.Uint32(h[4:])
}

func (h rawHeader) String() string {
	return fmt.Sprintf("Version:%d Cmd:%d StreamID:%d Length:%d",
		h.Version(), h.Cmd(), h.StreamID(), h.Length())
}

// updHeader 是 cmdUPD 命令的字节数组表示
type updHeader [szCmdUPD]byte

func (h updHeader) Consumed() uint32 {
	return binary.LittleEndian.Uint32(h[:])
}
func (h updHeader) Window() uint32 {
	return binary.LittleEndian.Uint32(h[4:])
}
