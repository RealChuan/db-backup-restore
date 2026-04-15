package utils

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

var Logger *logrus.Logger

// CustomFormatter 自定义日志格式化器
type CustomFormatter struct {
	logrus.TextFormatter
}

// Format 自定义日志格式
func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// 获取 caller 字段
	caller, ok := entry.Data["caller"]
	if ok {
		// 从 entry.Data 中删除 caller 字段，避免在消息后重复显示
		delete(entry.Data, "caller")
		// 构建自定义格式：时间 级别 caller 消息
		formatted := fmt.Sprintf("%s %s %s %s\n",
			entry.Time.Format("2006-01-02 15:04:05.000"),
			strings.ToUpper(entry.Level.String()),
			caller,
			entry.Message,
		)
		return []byte(formatted), nil
	}
	// 如果没有 caller 字段，使用默认格式
	return f.TextFormatter.Format(entry)
}

func init() {
	Logger = logrus.New()
	Logger.SetFormatter(&CustomFormatter{
		TextFormatter: logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05.000",
			DisableQuote:    true,
			ForceColors:     true,
		},
	})
	Logger.SetReportCaller(false)
}

// getCallerInfo 获取调用者信息
func getCallerInfo() (string, int, string) {
	pc, file, line, _ := runtime.Caller(3) // 3 表示获取调用者的调用者的调用者信息
	funcName := runtime.FuncForPC(pc).Name()
	if lastSlash := strings.LastIndex(funcName, "/"); lastSlash >= 0 {
		funcName = funcName[lastSlash+1:]
	}
	if lastSlash := strings.LastIndex(file, "/"); lastSlash >= 0 {
		file = file[lastSlash+1:]
	}
	return file, line, funcName
}

// logWithCaller 通用日志函数，添加调用者信息
func logWithCaller(level logrus.Level, args ...interface{}) {
	file, line, funcName := getCallerInfo()
	callerInfo := fmt.Sprintf("%s:%d %s", file, line, funcName)
	Logger.WithField("caller", callerInfo).Log(level, args...)
}

// logfWithCaller 通用格式化日志函数，添加调用者信息
func logfWithCaller(level logrus.Level, format string, args ...interface{}) {
	file, line, funcName := getCallerInfo()
	callerInfo := fmt.Sprintf("%s:%d %s", file, line, funcName)
	message := fmt.Sprintf(format, args...)
	Logger.WithField("caller", callerInfo).Log(level, message)
}

// Info 记录信息级别的日志
func Info(args ...interface{}) {
	logWithCaller(logrus.InfoLevel, args...)
}

// Infof 记录格式化的信息级别的日志
func Infof(format string, args ...interface{}) {
	logfWithCaller(logrus.InfoLevel, format, args...)
}

// Warn 记录警告级别的日志
func Warn(args ...interface{}) {
	logWithCaller(logrus.WarnLevel, args...)
}

// Warnf 记录格式化的警告级别的日志
func Warnf(format string, args ...interface{}) {
	logfWithCaller(logrus.WarnLevel, format, args...)
}

// Error 记录错误级别的日志
func Error(args ...interface{}) {
	logWithCaller(logrus.ErrorLevel, args...)
}

// Errorf 记录格式化的错误级别的日志
func Errorf(format string, args ...interface{}) {
	logfWithCaller(logrus.ErrorLevel, format, args...)
}

// Fatal 记录致命级别的日志并退出程序
func Fatal(args ...interface{}) {
	logWithCaller(logrus.FatalLevel, args...)
}

// Fatalf 记录格式化的致命级别的日志并退出程序
func Fatalf(format string, args ...interface{}) {
	logfWithCaller(logrus.FatalLevel, format, args...)
}

// Debug 记录调试级别的日志
func Debug(args ...interface{}) {
	logWithCaller(logrus.DebugLevel, args...)
}

// Debugf 记录格式化的调试级别的日志
func Debugf(format string, args ...interface{}) {
	logfWithCaller(logrus.DebugLevel, format, args...)
}
