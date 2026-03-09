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
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// Config 用于调整 Smux 会话的参数
type Config struct {
	// SMUX 协议版本，支持 1、2
	Version int

	// 禁用心跳保活
	KeepAliveDisabled bool

	// KeepAliveInterval 是向远端发送 NOP 命令的间隔时间
	KeepAliveInterval time.Duration

	// KeepAliveTimeout 是在没有数据到达时
	// 关闭会话的超时时间
	KeepAliveTimeout time.Duration

	// MaxFrameSize 用于控制发送到远端的最大
	// 帧大小
	MaxFrameSize int

	// MaxReceiveBuffer 用于控制缓冲区池中
	// 数据的最大数量
	MaxReceiveBuffer int

	// MaxStreamBuffer 用于控制每个流的
	// 最大数据量
	MaxStreamBuffer int
}

// DefaultConfig 用于返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Version:           1,
		KeepAliveInterval: 10 * time.Second,
		KeepAliveTimeout:  30 * time.Second,
		MaxFrameSize:      32768,
		MaxReceiveBuffer:  4194304,
		MaxStreamBuffer:   65536,
	}
}

// VerifyConfig 用于验证配置的合法性
func VerifyConfig(config *Config) error {
	if !(config.Version == 1 || config.Version == 2) {
		return errors.New("unsupported protocol version")
	}
	if !config.KeepAliveDisabled {
		if config.KeepAliveInterval == 0 {
			return errors.New("keep-alive interval must be positive")
		}
		if config.KeepAliveTimeout < config.KeepAliveInterval {
			return fmt.Errorf("keep-alive timeout must be larger than keep-alive interval")
		}
	}
	if config.MaxFrameSize <= 0 {
		return errors.New("max frame size must be positive")
	}
	if config.MaxFrameSize > 65535 {
		return errors.New("max frame size must not be larger than 65535")
	}
	if config.MaxReceiveBuffer <= 0 {
		return errors.New("max receive buffer must be positive")
	}
	if config.MaxStreamBuffer <= 0 {
		return errors.New("max stream buffer must be positive")
	}
	if config.MaxStreamBuffer > config.MaxReceiveBuffer {
		return errors.New("max stream buffer must not be larger than max receive buffer")
	}
	if config.MaxStreamBuffer > math.MaxInt32 {
		return errors.New("max stream buffer cannot be larger than 2147483647")
	}
	return nil
}

// Server 用于初始化一个新的服务端连接。
func Server(conn io.ReadWriteCloser, config *Config) (*Session, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := VerifyConfig(config); err != nil {
		return nil, err
	}
	return newSession(config, conn, false), nil
}

// Client 用于初始化一个新的客户端连接。
func Client(conn io.ReadWriteCloser, config *Config) (*Session, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := VerifyConfig(config); err != nil {
		return nil, err
	}
	return newSession(config, conn, true), nil
}
