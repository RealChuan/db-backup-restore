package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
)

// FileSystemBackupManager 提供基于文件系统的备份管理通用实现，
// 供 MySQL、PostgreSQL 等驱动组合复用，消除重复代码。
type FileSystemBackupManager struct {
	backupDir string
	dbType    string
	systemDbs []string
}

// NewFileSystemBackupManager 创建文件系统备份管理器实例。
func NewFileSystemBackupManager(backupDir, dbType string, systemDbs []string) *FileSystemBackupManager {
	return &FileSystemBackupManager{
		backupDir: backupDir,
		dbType:    dbType,
		systemDbs: systemDbs,
	}
}

// ListBackups 列出所有备份（逻辑备份 *.sql* 文件 + 物理备份 *_physical 目录）。
func (f *FileSystemBackupManager) ListBackups(_ context.Context, targetPath string) ([]BackupInfo, error) {
	backupDir := targetPath
	if backupDir == "" {
		backupDir = f.backupDir
	}
	if backupDir == "" {
		return nil, errors.New("必须指定备份目录")
	}

	var backups []BackupInfo

	// 列出逻辑备份文件
	files, err := filepath.Glob(filepath.Join(backupDir, "*.sql*"))
	if err != nil {
		return nil, fmt.Errorf("列出逻辑备份失败: %w", err)
	}

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			BackupID:       filepath.Base(file),
			CompletionTime: info.ModTime(),
			Size:           info.Size(),
			BackupPath:     file,
			BackupType:     "LOGICAL",
		})
	}

	// 列出物理备份目录
	dirs, err := filepath.Glob(filepath.Join(backupDir, "*_physical"))
	if err != nil {
		return nil, fmt.Errorf("列出物理备份失败: %w", err)
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}

		backups = append(backups, BackupInfo{
			BackupID:       filepath.Base(dir),
			CompletionTime: info.ModTime(),
			Size:           fileutil.GetDirSize(dir),
			BackupPath:     dir,
			BackupType:     "PHYSICAL",
		})
	}

	return backups, nil
}

// DeleteBackup 删除指定备份（支持绝对路径和相对标识符）。
func (f *FileSystemBackupManager) DeleteBackup(_ context.Context, identifier string, targetPath string) error {
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

// GetBackupInfo 获取指定备份的详细信息。
func (f *FileSystemBackupManager) GetBackupInfo(_ context.Context, backupID string, targetPath string) (map[string]string, error) {
	if backupID == "" {
		return nil, errors.New("必须指定备份文件路径")
	}

	var backupPath string
	backupDir := targetPath
	if backupDir == "" {
		backupDir = f.backupDir
	}

	if filepath.IsAbs(backupID) {
		cleanPath, err := sanitizeBackupPath(backupID)
		if err != nil {
			return nil, fmt.Errorf("invalid backup path: %w", err)
		}
		if err := mustBeUnderBackupDir(cleanPath, backupDir); err != nil {
			return nil, fmt.Errorf("backup path not in allowed directory: %w", err)
		}
		backupPath = cleanPath
	} else {
		if backupDir == "" {
			return nil, errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
		backupPath = filepath.Join(backupDir, backupID)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("获取备份信息失败: %w", err)
	}

	result := make(map[string]string)
	result["path"] = backupPath
	result["size"] = strconv.FormatInt(info.Size(), 10)
	result["mod_time"] = info.ModTime().Format(time.RFC3339)

	if info.IsDir() {
		result["backup_type"] = "PHYSICAL"
		result["size"] = strconv.FormatInt(fileutil.GetDirSize(backupPath), 10)
	} else {
		result["backup_type"] = "LOGICAL"
	}

	return result, nil
}

// DeleteAllBackups 删除所有备份（逻辑备份文件 + 物理备份目录）。
func (f *FileSystemBackupManager) DeleteAllBackups(_ context.Context, targetPath string) error {
	backupDir := targetPath
	if backupDir == "" {
		backupDir = f.backupDir
	}
	if backupDir == "" {
		return errors.New("必须指定备份目录")
	}

	// 删除逻辑备份文件
	files, err := filepath.Glob(filepath.Join(backupDir, "*.sql*"))
	if err != nil {
		return fmt.Errorf("列出逻辑备份失败: %w", err)
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			logging.Warn(fmt.Sprintf("删除逻辑备份失败: %v", err))
		}
	}

	// 删除物理备份目录
	dirs, err := filepath.Glob(filepath.Join(backupDir, "*_physical"))
	if err != nil {
		return fmt.Errorf("列出物理备份目录失败: %w", err)
	}

	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			logging.Warn(fmt.Sprintf("删除物理备份失败: %v", err))
		}
	}

	return nil
}
