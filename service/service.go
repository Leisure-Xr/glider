package service

import (
	"errors"
	"strings"
)

var creators = make(map[string]Creator)

// Service 是可运行的服务接口。
type Service interface{ Run() }

// Creator 是创建服务的函数类型。
type Creator func(args ...string) (Service, error)

// Register 用于注册服务。
func Register(name string, c Creator) {
	creators[strings.ToLower(name)] = c
}

// New 调用已注册的创建函数来创建服务。
func New(s string) (Service, error) {
	args := strings.Split(s, ",")
	c, ok := creators[strings.ToLower(args[0])]
	if ok {
		return c(args[1:]...)
	}
	return nil, errors.New("未知的服务名：'" + args[0] + "'")
}
