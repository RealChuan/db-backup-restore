package main

import (
	"github.com/RealChuan/db-backup-restore/internal/backup"
)

func main() {
	backup.RegisterAllDrivers()
	Execute()
}
