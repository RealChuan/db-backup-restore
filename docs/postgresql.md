# PostgreSQL 数据库备份与还原

PostgreSQL 支持两种备份方式：

- **逻辑备份**：使用 `pg_dump` 工具，备份文件为 SQL 格式，支持单库、多库和全部数据库备份
- **物理备份**：使用 `pg_basebackup`，适用于大规模数据库的快速备份恢复

---

## 核心结构体

### `PostgreSQLBackup`

```go
type PostgreSQLBackup struct {
    BaseBackup
    psqlPath           string   // psql 命令路径
    pgDumpPath         string   // pg_dump 命令路径
    pgDumpallPath      string   // pg_dumpall 命令路径
    pgBasebackupPath   string   // pg_basebackup 命令路径
    pgVerifyBackupPath string   // pg_verifybackup 命令路径
    pgCtlPath          string   // pg_ctl 命令路径
    env                []string // 环境变量（PGHOST、PGPORT、PGUSER、PGPASSWORD、PGDATABASE、PGSSLMODE）
    fsManager          *FileSystemBackupManager // 文件系统备份管理器
}
```

### 构造函数

```go
func NewPostgreSQLBackup(config *DBConfig) (*PostgreSQLBackup, error)
```

- 要求 `config.Type` 必须为 `"postgresql"`
- 通过 `config.Extra["PG_BIN_PATH"]` 自定义 PostgreSQL 工具路径（默认从 PATH 查找）
- 自动设置环境变量：`PGHOST`、`PGPORT`、`PGUSER`、`PGPASSWORD`、`PGDATABASE`
- 若 `config.SSLMode` 非空，额外设置 `PGSSLMODE`

---

## 配置项

| 配置项 | 来源 | 说明 |
| --- | --- | --- |
| `PG_BIN_PATH` | `config.Extra` | PostgreSQL 工具目录路径，设置后将从此目录查找 psql、pg_dump、pg_dumpall、pg_basebackup、pg_verifybackup、pg_ctl |
| `SSLMode` | `config.SSLMode` | SSL 连接模式，非空时设置 `PGSSLMODE` 环境变量 |
| `DATA_DIR` | `config.Extra` | PostgreSQL 数据目录路径，物理还原时必需 |
| `SERVICE_NAME` | `config.Extra` | Windows 下 PostgreSQL 服务名称，默认为 `postgresql-x64-18` |

---

## 支持的操作

### 备份与还原

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `Backup` | `(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error)` | 根据备份类型执行逻辑或物理备份；不支持增量/差异模式，将自动降级为全量备份 |
| `Restore` | `(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)` | 根据备份标识符自动判断逻辑/物理还原（文件→逻辑，目录→物理） |

### 备份管理（委托给 FileSystemBackupManager）

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `ListBackups` | `(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error)` | 列出所有备份 |
| `DeleteBackup` | `(ctx context.Context, identifier string, opts ...BackupOptions) error` | 删除指定备份 |
| `GetBackupInfo` | `(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error)` | 获取指定备份的详细信息 |
| `DeleteAllBackups` | `(ctx context.Context, opts ...BackupOptions) error` | 删除所有备份 |

### 备份验证

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `ValidateBackup` | `(ctx context.Context, backupID string, opts ...BackupOptions) error` | 验证物理备份完整性（使用 `pg_verifybackup`），不支持逻辑备份 |

---

## 驱动注册

通过 `registerPostgreSQLDriver()` 注册驱动，元数据如下：

- **名称**：`postgresql`
- **支持的操作**：backup、restore、list、delete、info、deleteAll
- **支持的备份类型**：`logical`、`physical`

---

## 📚 相关文档

- [PostgreSQL 逻辑备份与还原](./postgresql_logical.md)
- [PostgreSQL 物理备份与还原](./postgresql_physical.md)

---

## 🔗 官方文档

- [PostgreSQL 备份与恢复指南](https://www.postgresql.org/docs/current/backup.html)
