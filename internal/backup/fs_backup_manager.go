package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
)

// FileSystemBackupManager 提供基于文件系统的备份管理通用实现，
// 供 MySQL、PostgreSQL 等驱动组合复用，消除重复代码。
type FileSystemBackupManager struct {
	backupDir       string   // 默认备份目录
	logicalGlob     string   // 逻辑备份文件匹配模式，默认 "*.sql*"
	physicalGlobs   []string // 物理备份目录匹配模式列表，默认 []string{"*_physical"}
	logFileSuffixes []string // 逻辑备份文件的关联日志文件后缀，删除备份时一并清理
}

// FSManagerOption 配置 FileSystemBackupManager 的可选参数。
type FSManagerOption func(*FileSystemBackupManager)

// WithLogicalGlob 设置逻辑备份文件匹配模式。
func WithLogicalGlob(glob string) FSManagerOption {
	return func(m *FileSystemBackupManager) {
		m.logicalGlob = glob
	}
}

// WithPhysicalGlobs 设置物理备份目录匹配模式列表。
func WithPhysicalGlobs(globs ...string) FSManagerOption {
	return func(m *FileSystemBackupManager) {
		m.physicalGlobs = globs
	}
}

// WithLogFileSuffixes 设置逻辑备份文件的关联日志文件后缀。
// 删除备份时，会同时删除 backupFile + suffix 的文件。
// 例如：WithLogFileSuffixes(".log", ".restore.log")，
// 删除 dameng_full.dmp 时会同时删除 dameng_full.dmp.log 和 dameng_full.dmp.restore.log。
func WithLogFileSuffixes(suffixes ...string) FSManagerOption {
	return func(m *FileSystemBackupManager) {
		m.logFileSuffixes = suffixes
	}
}

// NewFileSystemBackupManager 创建文件系统备份管理器实例。
func NewFileSystemBackupManager(backupDir string, opts ...FSManagerOption) *FileSystemBackupManager {
	m := &FileSystemBackupManager{
		backupDir:     backupDir,
		logicalGlob:   "*.sql*",
		physicalGlobs: []string{"*_physical"},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
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
	files, err := filepath.Glob(filepath.Join(backupDir, f.logicalGlob))
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
			BackupType:     string(BackupTypeLogical),
		})
	}

	// 列出物理备份目录
	for _, pglob := range f.physicalGlobs {
		dirs, err := filepath.Glob(filepath.Join(backupDir, pglob))
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
				BackupType:     string(BackupTypePhysical),
			})
		}
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].BackupID < backups[j].BackupID
	})

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

	// 删除备份文件本身
	if err := os.Remove(backupPath); err != nil {
		return err
	}

	// 删除关联的日志文件
	f.deleteAssociatedLogFiles(backupPath)

	return nil
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
		if strings.ContainsAny(backupID, `/\`) {
			return nil, fmt.Errorf("backup identifier cannot contain path separators: %q", backupID)
		}
		backupPath = filepath.Join(backupDir, backupID)
		if err := mustBeUnderBackupDir(backupPath, backupDir); err != nil {
			return nil, err
		}
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
		result["backup_type"] = string(BackupTypePhysical)
		result["size"] = strconv.FormatInt(fileutil.GetDirSize(backupPath), 10)
	} else {
		result["backup_type"] = string(BackupTypeLogical)
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
	files, err := filepath.Glob(filepath.Join(backupDir, f.logicalGlob))
	if err != nil {
		return fmt.Errorf("列出逻辑备份失败: %w", err)
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			logging.Warn("删除逻辑备份失败", "error", err)
			continue
		}
		f.deleteAssociatedLogFiles(file)
	}

	// 删除物理备份目录
	for _, pglob := range f.physicalGlobs {
		dirs, err := filepath.Glob(filepath.Join(backupDir, pglob))
		if err != nil {
			return fmt.Errorf("列出物理备份目录失败: %w", err)
		}

		for _, dir := range dirs {
			if err := os.RemoveAll(dir); err != nil {
				logging.Warn("删除物理备份失败", "error", err)
			}
		}
	}

	return nil
}

// deleteAssociatedLogFiles 删除与逻辑备份文件关联的日志文件。
// 使用 glob 匹配 backupPath + "_*" + suffix 的文件，以支持带时间戳后缀的日志文件名。
// 例如：备份文件 dameng_full_20260710.dmp 对应的日志文件
//
//	dameng_full_20260710.dmp_20260710_120000.log
//	dameng_full_20260710.dmp_20260710_140000.restore.log
//
// 同时也兼容无时间戳的精确后缀匹配（backupPath + suffix）。
// 日志文件不存在时不报错。
func (f *FileSystemBackupManager) deleteAssociatedLogFiles(backupPath string) {
	for _, suffix := range f.logFileSuffixes {
		deleted := false

		// 优先尝试 glob 匹配带时间戳后缀的日志文件：backupPath_*suffix
		pattern := backupPath + "_*" + suffix
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logging.Warn("匹配关联日志文件失败", "pattern", pattern, "error", err)
			continue
		}
		for _, logPath := range matches {
			if err := os.Remove(logPath); err != nil {
				if !os.IsNotExist(err) {
					logging.Warn("删除关联日志文件失败", "log_file", logPath, "error", err)
				}
			} else {
				logging.Info("已删除关联日志文件", "log_file", logPath)
				deleted = true
			}
		}

		// 兼容精确后缀匹配（无时间戳）：backupPath + suffix
		exactPath := backupPath + suffix
		if _, statErr := os.Stat(exactPath); statErr == nil {
			if err := os.Remove(exactPath); err != nil {
				if !os.IsNotExist(err) {
					logging.Warn("删除关联日志文件失败", "log_file", exactPath, "error", err)
				}
			} else {
				logging.Info("已删除关联日志文件", "log_file", exactPath)
				deleted = true
			}
		}

		if !deleted {
			logging.Debug("未找到关联日志文件", "pattern", pattern)
		}
	}
}
