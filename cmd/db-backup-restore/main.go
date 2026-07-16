package main

import (
	"fmt"
	"os"

	"github.com/RealChuan/db-backup-restore/internal/backup"
)

func main() {
	if err := backup.RegisterAllDrivers(); err != nil {
		fmt.Fprintf(os.Stderr, "注册驱动失败: %v\n", err)
		os.Exit(1)
	}
	os.Exit(Execute())
}
