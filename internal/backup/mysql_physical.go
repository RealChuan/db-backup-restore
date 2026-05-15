package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"db-backup-restore/pkg/utils"
)

// isAdmin 检查当前进程是否以管理员身份运行
func (m *MySQLBackup) isAdmin() bool {
	return utils.IsAdmin()
}

// backupPhysical 执行 MySQL 物理备份（备份整个数据库实例）
func (m *MySQLBackup) backupPhysical(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	if !m.isAdmin() {
		return nil, errors.New("物理备份需要管理员权限，请以管理员身份运行程序")
	}

	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if callback != nil {
		callback(0, "开始物理备份 MySQL 实例...")
	}

	backupPath := filepath.Join(backupDir, fmt.Sprintf("mysql_%s_physical", time.Now().Format("20060102_150405")))

	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("创建备份目录失败: %w", err)
	}

	if err := m.executePhysicalBackup(ctx, backupPath, callback); err != nil {
		return nil, fmt.Errorf("物理备份失败: %w", err)
	}

	if callback != nil {
		callback(100, "物理备份完成")
	}

	result.BackupFile = backupPath
	result.BackupSize = utils.GetDirSize(backupPath)
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Success = true

	return result, nil
}

// executePhysicalBackup 执行物理备份
func (m *MySQLBackup) executePhysicalBackup(ctx context.Context, backupPath string, callback ProgressCallback) error {
	xtrabackupPath, err := m.getXtrabackupPathOrError()
	if err != nil {
		return err
	}

	utils.Infof("检测到 xtrabackup，使用 Percona XtraBackup 进行物理备份")
	return m.execXtrabackup(ctx, xtrabackupPath, backupPath, callback)
}

// getXtrabackupPath 获取 xtrabackup 命令路径
func (m *MySQLBackup) getXtrabackupPath() string {
	if val, ok := m.config.Extra["XTRABACKUP_BIN_PATH"]; ok && val != "" {
		return utils.AddExeExt(filepath.Join(val, "xtrabackup"))
	}

	if path, err := exec.LookPath("xtrabackup"); err == nil {
		return path
	}
	if path, err := exec.LookPath("innobackupex"); err == nil {
		return path
	}

	return ""
}

// getXtrabackupPathOrError 获取 xtrabackup 命令路径，如果未找到则返回错误
func (m *MySQLBackup) getXtrabackupPathOrError() (string, error) {
	xtrabackupPath := m.getXtrabackupPath()
	if xtrabackupPath == "" {
		return "", errors.New("未检测到 xtrabackup，请安装 Percona XtraBackup")
	}
	return xtrabackupPath, nil
}

// execXtrabackup 使用 Percona XtraBackup 执行物理备份
func (m *MySQLBackup) execXtrabackup(ctx context.Context, xtrabackupPath, backupPath string, callback ProgressCallback) error {
	utils.Infof("使用 Percona XtraBackup 进行物理备份")

	args := []string{
		"--backup",
		"--target-dir=" + backupPath,
		fmt.Sprintf("--host=%s", m.config.Host),
		fmt.Sprintf("--port=%d", m.config.Port),
		fmt.Sprintf("--user=%s", m.config.User),
	}

	if m.config.Password != "" {
		args = append(args, fmt.Sprintf("--password=%s", m.config.Password))
	}

	if callback != nil {
		callback(20, "正在执行 xtrabackup 备份...")
	}

	cmdStr := xtrabackupPath + " " + strings.Join(args, " ")
	utils.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, xtrabackupPath, args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stderrBytes, _ := io.ReadAll(stderr)
	stderrOutput := string(stderrBytes)

	if err := cmd.Wait(); err != nil {
		utils.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("xtrabackup 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		utils.LogCommand(cmdStr, stderrOutput, false)
	}

	if callback != nil {
		callback(80, "xtrabackup 备份完成，准备 prepare...")
	}

	prepareArgs := []string{
		"--prepare",
		"--target-dir=" + backupPath,
	}

	prepareCmdStr := xtrabackupPath + " " + strings.Join(prepareArgs, " ")
	utils.LogCommandInfo(prepareCmdStr)

	prepareCmd := exec.CommandContext(ctx, xtrabackupPath, prepareArgs...)
	prepareOutput, err := utils.ExecCommand(ctx, prepareCmd)

	if err != nil {
		utils.LogCommand(prepareCmdStr, prepareOutput, true)
		return fmt.Errorf("xtrabackup prepare 失败: %w", err)
	}
	utils.LogCommand(prepareCmdStr, prepareOutput, false)

	utils.Infof("XtraBackup 物理备份完成，备份目录: %s", backupPath)
	return nil
}

// restorePhysical 执行 MySQL 物理还原（还原整个数据库实例）
//
// 安全设计原则：
//  1. 先还原到临时目录，验证完整性后再切换
//  2. 保留旧数据目录作为备份，不自动删除
//  3. 任何步骤失败都进行回滚，保证数据安全
//
// 执行流程（原子操作保证数据安全）：
//  1. 权限检查 → 2. 参数校验 → 3. 创建临时目录 → 4. 还原到临时目录
//  5. 验证临时目录 → 6. 停止服务 → 7. 重命名旧目录 → 8. 切换新目录
//  9. 设置权限 → 10. 启动服务 → 11. 输出清理提示
//
// 关键设计说明：
//   - 临时目录策略：先将备份还原到 {datadir}_new_{timestamp}，验证通过后再切换
//   - 旧目录保留：原数据目录重命名为 {datadir}_old_{timestamp}，由用户手动清理
//   - 回滚机制：任何步骤失败都清理临时目录，必要时恢复原数据目录
//   - 最小停机时间：仅在切换目录阶段停止服务
func (m *MySQLBackup) restorePhysical(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	// 步骤1：权限检查 - 物理还原需要管理员权限（涉及系统服务管理和文件操作）
	if !m.isAdmin() {
		return nil, errors.New("物理还原需要管理员权限，请以管理员身份运行程序")
	}

	startTime := time.Now()
	result := &RestoreResult{}

	if callback != nil {
		callback(0, "开始执行物理还原...")
	}

	// 步骤2：获取 xtrabackup 工具路径
	xtrabackupPath, err := m.getXtrabackupPathOrError()
	if err != nil {
		return nil, fmt.Errorf("物理还原失败: %w", err)
	}

	// 获取备份目录路径（从参数中提取）
	var backupDir string
	if opts.BackupIdentifier != "" {
		backupDir = opts.BackupIdentifier
	}

	if backupDir == "" {
		return nil, errors.New("必须通过 --backup-identifier 参数指定备份目录路径")
	}

	// 验证备份目录存在
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("备份目录不存在: %s", backupDir)
	}

	// 物理还原会还原整个实例，目标数据库名参数将被忽略（给出警告）
	if opts.TargetDatabaseName != "" {
		utils.Warnf("物理还原将还原整个 MySQL 实例，指定的目标数据库 [%s] 将被忽略", opts.TargetDatabaseName)
	}

	// 获取数据目录配置（必须在配置文件中设置）
	datadir := m.config.Extra["DATA_DIR"]
	if datadir == "" {
		return nil, errors.New("未配置 MySQL 数据目录，请在配置文件中设置 Extra[\"DATA_DIR\"]")
	}

	// 安全校验：验证数据目录合法性（防止路径遍历攻击和误操作）
	if err := validateDataDir(datadir, "mysql"); err != nil {
		return nil, fmt.Errorf("DATA_DIR validation failed: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	tempDir := datadir + "_new_" + timestamp
	oldDir := datadir + "_old_" + timestamp

	if callback != nil {
		callback(20, "创建临时目录...")
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}

	if callback != nil {
		callback(30, "执行 xtrabackup --copy-back 到临时目录...")
	}

	if err := m.execXtrabackupRestore(ctx, xtrabackupPath, backupDir, tempDir, callback); err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}

	if callback != nil {
		callback(50, "验证临时目录...")
	}

	if err := validateDataDir(tempDir, "mysql"); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("临时目录验证失败: %w", err)
	}

	if callback != nil {
		callback(60, "停止 MySQL 服务...")
	}

	if err := m.stopMySQLService(ctx); err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}

	if callback != nil {
		callback(70, "重命名旧数据目录...")
	}

	utils.Infof("正在重命名旧数据目录 %s -> %s", datadir, oldDir)
	if err := os.Rename(datadir, oldDir); err != nil {
		m.startMySQLService(ctx)
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to rename old data dir: %w", err)
	}

	if callback != nil {
		callback(80, "切换到新数据目录...")
	}

	utils.Infof("正在重命名临时目录 %s -> %s", tempDir, datadir)
	if err := os.Rename(tempDir, datadir); err != nil {
		os.Rename(oldDir, datadir)
		m.startMySQLService(ctx)
		return nil, fmt.Errorf("failed to rename temp dir to datadir: %w", err)
	}

	if callback != nil {
		callback(85, "设置文件权限...")
	}

	if err := m.setMySQLFilePermissions(datadir); err != nil {
		utils.Warnf("设置文件权限失败: %v", err)
	}

	if callback != nil {
		callback(90, "启动 MySQL 服务...")
	}

	if err := m.startMySQLService(ctx); err != nil {
		return nil, err
	}

	if callback != nil {
		callback(100, "物理还原完成")
	}

	utils.Warnf("注意：旧数据目录 %s 已保留，请在确认数据无误后手动清理", oldDir)

	result.Duration = time.Since(startTime)
	result.Success = true

	return result, nil
}

// execXtrabackupRestore 使用 Percona XtraBackup 执行物理还原
func (m *MySQLBackup) execXtrabackupRestore(ctx context.Context, xtrabackupPath, backupDir, datadir string, callback ProgressCallback) error {
	utils.Infof("使用 Percona XtraBackup 进行物理还原")

	args := []string{
		"--copy-back",
		"--src-dir=" + backupDir,
		"--datadir=" + datadir,
	}

	cmdStr := xtrabackupPath + " " + strings.Join(args, " ")
	utils.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, xtrabackupPath, args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stderrBytes, _ := io.ReadAll(stderr)
	stderrOutput := string(stderrBytes)

	if err := cmd.Wait(); err != nil {
		utils.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("xtrabackup --copy-back 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		utils.LogCommand(cmdStr, stderrOutput, false)
	}

	utils.Infof("XtraBackup 物理还原完成，数据目录: %s", datadir)
	return nil
}

// getMySQLServiceName 获取 MySQL 服务名
func (m *MySQLBackup) getMySQLServiceName() string {
	if svc, ok := m.config.Extra["SERVICE_NAME"]; ok && svc != "" {
		return svc
	}

	if runtime.GOOS == "windows" {
		return "MySQL"
	}

	candidates := []string{"mysqld", "mysql", "mariadb", "percona"}
	for _, candidate := range candidates {
		cmd := exec.Command("systemctl", "is-active", "--quiet", candidate)
		if err := cmd.Run(); err == nil {
			return candidate
		}
	}

	return "mysqld"
}

// stopMySQLService 停止 MySQL 服务
func (m *MySQLBackup) stopMySQLService(ctx context.Context) error {
	serviceName := m.getMySQLServiceName()
	utils.Infof("正在停止 MySQL 服务: %s", serviceName)

	if utils.IsWindows() {
		cmd := exec.CommandContext(ctx, "net", "stop", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}
	} else {
		cmd := exec.CommandContext(ctx, "systemctl", "stop", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}
		cmd = exec.CommandContext(ctx, "service", serviceName, "stop")
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("无法停止 MySQL 服务 %s", serviceName)
}

// startMySQLService 启动 MySQL 服务
func (m *MySQLBackup) startMySQLService(ctx context.Context) error {
	serviceName := m.getMySQLServiceName()
	utils.Infof("正在启动 MySQL 服务: %s", serviceName)

	if utils.IsWindows() {
		cmd := exec.CommandContext(ctx, "net", "start", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}
	} else {
		cmd := exec.CommandContext(ctx, "systemctl", "start", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}
		cmd = exec.CommandContext(ctx, "service", serviceName, "start")
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("无法启动 MySQL 服务 %s", serviceName)
}

// setMySQLFilePermissions 设置 MySQL 文件权限
func (m *MySQLBackup) setMySQLFilePermissions(datadir string) error {
	return filepath.Walk(datadir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, 0755)
	})
}
