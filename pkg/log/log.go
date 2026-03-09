package log

import (
	"fmt"
	stdlog "log"
)

var enable = false

// Set 设置日志器的详细模式和输出标志。
func Set(verbose bool, flag int) {
	enable = verbose
	stdlog.SetFlags(flag)
}

// F 打印调试日志。
func F(f string, v ...any) {
	if enable {
		stdlog.Output(2, fmt.Sprintf(f, v...))
	}
}

// Print 打印日志。
func Print(v ...any) {
	stdlog.Print(v...)
}

// Printf 打印日志。
func Printf(f string, v ...any) {
	stdlog.Printf(f, v...)
}

// Fatal 记录日志并退出。
func Fatal(v ...any) {
	stdlog.Fatal(v...)
}

// Fatalf 记录日志并退出。
func Fatalf(f string, v ...any) {
	stdlog.Fatalf(f, v...)
}
