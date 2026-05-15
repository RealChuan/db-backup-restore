package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"db-backup-restore/pkg/utils"
)

type FileSystemBackupManager struct {
	backupDir string
	dbType    string
	systemDbs []string
}

func NewFileSystemBackupManager(backupDir, dbType string, systemDbs []string) *FileSystemBackupManager {
	return &FileSystemBackupManager{
		backupDir: backupDir,
		dbType:    dbType,
		systemDbs: systemDbs,
	}
}

func (f *FileSystemBackupManager) ListBackups(ctx context.Context, targetPath string) ([]BackupInfo, error) {
	backupDir := targetPath
	if backupDir == "" {
		backupDir = f.backupDir
	}
	if backupDir == "" {
		return nil, errors.New("必须指定备份目录")
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			backupPath := filepath.Join(backupDir, entry.Name())
			info, err := os.Stat(backupPath)
			if err != nil {
				continue
			}

			backups = append(backups, BackupInfo{
				BackupID:       entry.Name(),
				BackupPath:     backupPath,
				Size:           utils.GetDirSize(backupPath),
				CompletionTime: info.ModTime(),
				BackupType:     "FULL",
			})
		}
	}

	for i := 0; i < len(backups)-1; i++ {
		for j := i + 1; j < len(backups); j++ {
			if backups[j].CompletionTime.After(backups[i].CompletionTime) {
				backups[i], backups[j] = backups[j], backups[i]
			}
		}
	}

	return backups, nil
}

func (f *FileSystemBackupManager) DeleteBackup(ctx context.Context, identifier string, targetPath string) error {
	var backupPath string
	if filepath.IsAbs(identifier) {
		cleanPath, err := sanitizeBackupPath(identifier)
		if err != nil {
			return fmt.Errorf("invalid backup path: %w", err)
		}
		backupPath = cleanPath
	} else {
		backupDir := targetPath
		if backupDir == "" {
			backupDir = f.backupDir
		}
		if backupDir == "" {
			return errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
		if strings.ContainsAny(identifier, `/\`) {
			return fmt.Errorf("backup identifier cannot contain path separators: %q", identifier)
		}
		backupPath = filepath.Join(backupDir, identifier)
		if err := mustBeUnderBackupDir(backupPath, backupDir); err != nil {
			return err
		}
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("备份不存在: %w", err)
	}

	if info.IsDir() {
		return os.RemoveAll(backupPath)
	}

	return os.Remove(backupPath)
}

func (f *FileSystemBackupManager) GetBackupInfo(ctx context.Context, backupID string, targetPath string) (map[string]string, error) {
	if backupID == "" {
		return nil, errors.New("必须指定备份文件路径")
	}

	var backupPath string
	if filepath.IsAbs(backupID) {
		cleanPath, err := sanitizeBackupPath(backupID)
		if err != nil {
			return nil, fmt.Errorf("invalid backup path: %w", err)
		}
		backupPath = cleanPath
	} else {
		backupDir := targetPath
		if backupDir == "" {
			backupDir = f.backupDir
		}
		if backupDir == "" {
			return nil, errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
		backupPath = filepath.Join(backupDir, backupID)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("备份文件不存在: %w", err)
	}

	backupInfo := make(map[string]string)
	backupInfo["path"] = backupPath
	backupInfo["name"] = filepath.Base(backupPath)
	backupInfo["size"] = fmt.Sprintf("%d", info.Size())
	backupInfo["mod_time"] = info.ModTime().Format(time.RFC3339)

	if info.IsDir() {
		backupInfo["type"] = "directory"
		backupInfo["size"] = fmt.Sprintf("%d", utils.GetDirSize(backupPath))
	} else {
		backupInfo["type"] = "file"
	}

	return backupInfo, nil
}

func (f *FileSystemBackupManager) getAllUserDatabases(ctx context.Context) ([]string, error) {
	return []string{}, nil
}
