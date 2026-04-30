//go:build windows

package utils

import (
	"syscall"
	"unsafe"
)

// TOKEN_ELEVATION 是 Windows API 中的常量
const TOKEN_ELEVATION = 20

// checkAdmin 检查当前进程是否以管理员身份运行（Windows）
func checkAdmin() bool {
	var hToken syscall.Token
	processHandle, err := syscall.GetCurrentProcess()
	if err != nil {
		return false
	}
	err = syscall.OpenProcessToken(processHandle, syscall.TOKEN_QUERY, &hToken)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(syscall.Handle(hToken))

	var tokenInfo struct {
		TokenIsAdmin uint32
	}
	tokenInfoSize := uint32(unsafe.Sizeof(tokenInfo))

	err = syscall.GetTokenInformation(
		hToken,
		TOKEN_ELEVATION,
		(*byte)(unsafe.Pointer(&tokenInfo)),
		tokenInfoSize,
		&tokenInfoSize,
	)
	if err != nil {
		return false
	}

	return tokenInfo.TokenIsAdmin != 0
}
