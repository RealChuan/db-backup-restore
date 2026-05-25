# MySQL 数据库备份与还原

MySQL 支持两种备份方式：

- **逻辑备份**：使用 `mysqldump` 工具，适用于 InnoDB 引擎，备份文件为 SQL 格式
- **物理备份**：使用 `Percona XtraBackup`，适用于大规模数据库的快速备份恢复

---

## 结构体

### MySQLBackup

`MySQLBackup` 实现 `DatabaseBackup` 接口，针对 MySQL 数据库。

```go
type MySQLBackup struct {
    BaseBackup
    mysqlPath     string // mysql 命令路径
    mysqldumpPath string // mysqldump 命令路径
    fsManager     *FileSystemBackupManager
}
```

### 创建实例

```go
func NewMySQLBackup(config *DBConfig) (*MySQLBackup, error)
```

- `config.Type` 必须为 `"mysql"`
- 通过 `config.Extra["MYSQL_BIN_PATH"]` 可指定 MySQL 二进制文件目录（包含 mysql 和 mysqldump）

---

## 支持的操作

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| Backup | `(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error)` | 根据备份类型调用逻辑或物理备份实现 |
| Restore | `(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)` | 根据备份类型调用逻辑或物理还原实现 |
| ListBackups | `(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error)` | 列出所有备份（委托给 FileSystemBackupManager） |
| DeleteBackup | `(ctx context.Context, identifier string, opts ...BackupOptions) error` | 删除指定备份（委托给 FileSystemBackupManager） |
| GetBackupInfo | `(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error)` | 获取备份详细信息（委托给 FileSystemBackupManager） |
| DeleteAllBackups | `(ctx context.Context, opts ...BackupOptions) error` | 删除所有备份（委托给 FileSystemBackupManager） |

### 不支持的操作

以下操作通过 `BaseBackup` 提供默认实现，返回 `NotSupportedError`：

- `ValidateBackup` - 不支持完整验证备份文件完整性
- `RegisterBackup` / `UnregisterBackup` - 不支持注册/取消注册备份到目录库
- `VerifyBackupStatus` - 不支持检查备份状态
- `DeleteInvalidBackups` - 不支持删除无效备份记录

---

## 配置项

通过 `DBConfig.Extra` 传入的配置项：

| 配置键 | 说明 | 适用场景 |
| --- | --- | --- |
| `MYSQL_BIN_PATH` | MySQL 二进制文件目录（包含 mysql 和 mysqldump） | 逻辑备份/还原 |
| `XTRABACKUP_BIN_PATH` | XtraBackup 二进制文件目录 | 物理备份/还原 |
| `SERVICE_NAME` | MySQL 服务名（默认自动检测） | 物理还原 |
| `DATA_DIR` | MySQL 数据目录（物理还原时必须配置） | 物理还原 |

---

## 备份类型与模式

- **备份类型**：支持 `logical`（逻辑备份）和 `physical`（物理备份）
- **备份模式**：不支持 `incremental`（增量）和 `differential`（差异）模式，指定时将自动降级为全量备份

---

## 📚 相关文档

- [MySQL 逻辑备份与还原](./mysql_logical.md)
- [MySQL 物理备份与还原](./mysql_physical.md)

## 🔗 官方文档

- [MySQL 备份与恢复指南](https://dev.mysqlserver.cn/doc/refman/8.4/en/backup-and-recovery.html)
