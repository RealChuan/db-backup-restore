# 数据库备份与还原 API 参考

本项目通过 `DatabaseBackup` 接口提供统一的数据库备份与还原操作，支持 MySQL、PostgreSQL、Oracle、SQL Server、达梦 (Dameng) 五种数据库。所有操作均通过 `backup.NewBackup(cfg)` 创建驱动实例后调用。

---

## 支持的数据库

| 数据库     | 逻辑备份     | 物理备份         | 目录库管理                                              | 归档模式管理 | 列出数据库 |
| ---------- | ------------ | ---------------- | ------------------------------------------------------- | ------------ | ---------- |
| MySQL      | ✅ mysqldump | ✅ XtraBackup    | ❌                                                      | ❌           | ✅         |
| PostgreSQL | ✅ pg_dump   | ✅ pg_basebackup | ❌（ValidateBackup 仅支持物理）                         | ❌           | ✅         |
| Oracle     | ❌           | ✅ RMAN          | ✅                                                      | ✅           | ❌         |
| SQL Server | ❌           | ✅ T-SQL         | ✅                                                      | ❌           | ✅         |
| 达梦       | ✅ dexp      | ✅ disql+dmrman  | ⚠️（ValidateBackup ✅; VerifyBackupStatus ✅; 其余 ❌） | ✅           | ✅ disql   |

> **目录库管理**包含：ValidateBackup、RegisterBackup、UnregisterBackup、VerifyBackupStatus、DeleteInvalidBackups 五个操作。Oracle 使用 RMAN 目录库，SQL Server 使用 msdb 系统表；达梦支持 ValidateBackup（`dmrman VALIDATE BACKUPSET`）和 VerifyBackupStatus（`dmrman CHECK BACKUPSET`）；MySQL/PostgreSQL 无内置目录库机制，这些操作返回 NotSupportedError。

> **归档模式管理**包含：EnableArchiveLog、DisableArchiveLog 两个操作。仅 Oracle 和达梦支持，MySQL/PostgreSQL/SQL Server 不支持。

---

## 操作说明

### Backup — 备份

执行数据库备份，支持逻辑备份和物理备份两种类型。

| 参数            | 说明                                                                                                                                        |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Type            | 备份类型：`logical`（逻辑）或 `physical`（物理）                                                                                            |
| Mode            | 备份模式：`full`（全量）、`incremental`（增量）、`differential`（差异）、`level0`（Level 0 增量基础，仅 Oracle）、`archive`（独立归档日志） |
| Encryption      | 是否启用加密（物理备份，Oracle/达梦支持）                                                                                                   |
| EncryptionKey   | 加密密钥（需配合 Encryption 使用）                                                                                                          |
| TargetPath      | 备份目标路径                                                                                                                                |
| ArchiveLogDest  | 归档日志目标路径（Oracle/达梦，从 BaseBackupDir 自动推导）                                                                                  |
| ArchiveFromLSN  | 归档备份起始 LSN（仅达梦，配合 `archive` 模式使用）                                                                                         |
| ArchiveUntilLSN | 归档备份结束 LSN（仅达梦，配合 `archive` 模式使用）                                                                                         |
| Timeout         | 超时时间                                                                                                                                    |

**注意事项**：

- MySQL/PostgreSQL 不支持增量/差异模式，指定时返回 NotSupportedError
- 达梦逻辑备份支持全库 (FULL) 和按模式 (SCHEMAS) 导出
- 达梦物理备份支持全量、增量、累积增量和归档日志备份
- 物理备份会备份整个数据库实例，不支持单库备份
- 压缩/并行/保留策略：通过 Extra 配置项控制（`ENABLE_COMPRESSION`、`COMPRESSION_LEVEL`、`PARALLEL_WORKERS`、`RETENTION_DAYS`），详见各数据库 Extra 参数表
- Oracle 备份前若数据库未开启归档模式，会自动启用归档模式
- 达梦归档备份支持按 LSN 范围备份（`ArchiveFromLSN`/`ArchiveUntilLSN`）

---

### Restore — 还原

执行数据库还原，根据备份标识符自动判断还原方式。

| 参数                | 说明                                                                                     |
| ------------------- | ---------------------------------------------------------------------------------------- |
| BackupIdentifier    | 备份标识符（Oracle/达梦: TAG 或备份集路径; MySQL/PostgreSQL/MSSQL: 文件路径）            |
| TargetDatabaseName  | 目标数据库名（MySQL/PostgreSQL/MSSQL 逻辑还原时可指定还原到新数据库）                    |
| RemapSchema         | 模式映射，格式: `source:target`（仅达梦 dimp 支持，将源模式数据导入目标模式）            |
| Overwrite           | 是否覆盖现有数据库                                                                       |
| RecoveryPointInTime | 时间点还原的目标时间（Oracle/达梦支持，可与 BackupIdentifier 组合）                      |
| RecoverySCN         | 按 SCN 还原（仅 Oracle 支持，可与 BackupIdentifier 组合）                                |
| RecoveryLSN         | 按 LSN 还原（仅达梦支持，配合 `archive` 模式使用）                                       |
| RestoreMode         | 还原模式：`full`（全量，默认）、`archive`（归档）、`controlfile`（控制文件，仅 Oracle）  |
| BackupType          | 备份类型：`logical` 或 `physical`                                                        |
| NoRedo              | 还原时跳过归档日志应用，即 NOREDO（仅 Oracle 支持）                                      |
| ArchiveFromSeq      | 归档还原起始序列号（仅 Oracle，配合 `archive` 模式使用）                                 |
| ArchiveUntilSeq     | 归档还原结束序列号（仅 Oracle，配合 `archive` 模式使用）                                 |
| ArchiveLogDest      | 归档日志目录路径（Oracle/达梦: 从 BaseBackupDir 自动推导，用于 RECOVER WITH ARCHIVEDIR） |
| Timeout             | 超时时间                                                                                 |

**注意事项**：

- 物理还原需要管理员权限
- 物理还原会还原整个实例，指定目标数据库将被忽略
- PostgreSQL 逻辑还原到新数据库时，会自动创建目标数据库
- MySQL 逻辑还原不会自动创建目标数据库，需提前创建
- 时间点还原（Point-in-Time）在 Oracle 和达梦上支持
- Oracle 还支持按 SCN 还原和归档还原（按序列号范围）
- 达梦还支持按 LSN 还原（配合 `archive` 模式）
- Oracle 支持增量还原（NOREDO 模式）和控制文件还原（控制文件丢失的灾难恢复）
- 达梦支持增量还原（RECOVER WITH BACKUPDIR）和归档还原
- 物理还原过程中，原数据目录会重命名为 `{datadir}_old_{timestamp}` 保留

---

### ListBackups — 列出备份

列出所有备份记录。

**实现模式**：

- MySQL/PostgreSQL：基于文件系统扫描备份目录（FileSystemBackupManager）
- Oracle：查询 RMAN 目录库 `LIST BACKUP SUMMARY`
- SQL Server：查询 msdb 系统表 `msdb.dbo.backupset`
- 达梦：基于文件系统扫描备份目录（FileSystemBackupManager）

---

### DeleteBackup — 删除备份

删除指定的备份记录及对应的物理备份文件。

**实现模式**：

- MySQL/PostgreSQL：删除文件系统中的备份文件
- Oracle：执行 `DELETE NOPROMPT BACKUPSET <id>`，支持按备份集 ID 或时间删除
- SQL Server：删除 msdb 备份记录 + 删除物理备份文件，支持按 backup_set_id 或时间删除
- 达梦：删除文件系统中的备份文件

---

### GetBackupInfo — 获取备份详情

获取指定备份的详细信息。

**实现模式**：

- MySQL/PostgreSQL：读取备份目录下的元数据文件
- Oracle：执行 `LIST BACKUPSET <id>`
- SQL Server：查询 `msdb.dbo.backupset`，backupID 为空时返回最近 10 条
- 达梦：读取备份目录下的元数据文件

---

### ValidateBackup — 验证备份

验证备份文件的有效性。

**支持情况**：

- ✅ Oracle：`VALIDATE BACKUPSET <id>` 或 `RESTORE DATABASE VALIDATE CHECK LOGICAL`
- ✅ SQL Server：`RESTORE VERIFYONLY FROM DISK = N'<path>'`
- ✅ PostgreSQL：仅物理备份，`pg_verifybackup <backup_path>`
- ✅ 达梦：`dmrman VALIDATE BACKUPSET "<path>"`
- ❌ MySQL：不支持，返回 NotSupportedError
- ❌ PostgreSQL 逻辑备份：不支持，仅物理备份可验证

---

### RegisterBackup — 注册备份

将外部备份文件注册到目录库。

**支持情况**：

- ✅ Oracle：`CATALOG START WITH '<path>'`
- ✅ SQL Server：`EXEC msdb.dbo.sp_add_backup_filehistory`
- ❌ MySQL/PostgreSQL/达梦：不支持

---

### UnregisterBackup — 取消注册备份

从目录库中移除备份记录（不删除物理文件）。

**支持情况**：

- ✅ Oracle：`CHANGE BACKUPSET <id> UNCATALOG`
- ✅ SQL Server：`EXEC msdb.dbo.sp_delete_backuphistory`
- ❌ MySQL/PostgreSQL/达梦：不支持

---

### VerifyBackupStatus — 检查备份状态

检查目录库中所有备份记录的有效性，无效记录将被删除。

**支持情况**：

- ✅ Oracle：`CROSSCHECK BACKUP`，过期记录执行 `DELETE NOPROMPT EXPIRED BACKUP`
- ✅ SQL Server：逐条执行 `RESTORE VERIFYONLY`，验证失败则删除记录及物理文件
- ✅ 达梦：`dmrman CHECK BACKUPSET`
- ❌ MySQL/PostgreSQL：不支持

---

### DeleteInvalidBackups — 删除无效备份

删除目录库中对应的物理文件已不存在的备份记录。

**支持情况**：

- ✅ Oracle：`DELETE NOPROMPT EXPIRED BACKUP`
- ✅ SQL Server：查询 msdb 记录，检查文件是否存在，不存在则删除记录
- ❌ MySQL/PostgreSQL/达梦：不支持

---

### DeleteAllBackups — 删除所有备份

删除所有备份记录及对应的物理备份文件。

**实现模式**：

- MySQL/PostgreSQL：清空备份目录
- Oracle：`DELETE NOPROMPT BACKUP`
- SQL Server：删除 msdb 所有备份相关表 + 删除物理备份文件
- 达梦：清空备份目录

---

### ListDatabases — 列出数据库

列出数据库实例中的所有用户数据库。

**支持情况**：

- ✅ MySQL：执行 `SHOW DATABASES`，排除 information_schema、mysql、performance_schema、sys
- ✅ PostgreSQL：执行 `SELECT datname FROM pg_database WHERE datistemplate = false`，排除 postgres
- ✅ SQL Server：执行 `SELECT name FROM sys.databases WHERE name NOT IN ('master','tempdb','model','msdb') AND state = 0`
- ✅ 达梦：使用 disql 执行 `SELECT USERNAME FROM DBA_USERS WHERE ACCOUNT_STATUS = 'OPEN'`
- ❌ Oracle：不支持（Oracle 基于实例架构，一个实例对应一个数据库）

---

### EnableArchiveLog — 启用归档模式

将数据库切换到 ARCHIVELOG 模式，这是执行联机物理备份的前提条件。

**支持情况**：

- ✅ Oracle：`SHUTDOWN IMMEDIATE; STARTUP MOUNT; ALTER DATABASE ARCHIVELOG; ALTER DATABASE OPEN;`
- ✅ 达梦：`ALTER DATABASE MOUNT; ALTER DATABASE NORMAL; ALTER DATABASE ARCHIVELOG; ALTER DATABASE ADD ARCHIVELOG ...; ALTER DATABASE OPEN;`
- ❌ MySQL/PostgreSQL/SQL Server：不支持

**参数**：

| 参数        | 说明                                             |
| ----------- | ------------------------------------------------ |
| ArchiveDest | 归档日志存储目录路径（为空则使用数据库默认配置） |

> **注意**：开启归档模式需要将数据库置于 MOUNT 状态，会短暂中断服务。建议在业务低峰期操作。

---

### DisableArchiveLog — 关闭归档模式

将数据库从 ARCHIVELOG 模式切换为 NOARCHIVELOG 模式。

**支持情况**：

- ✅ Oracle：`SHUTDOWN IMMEDIATE; STARTUP MOUNT; ALTER DATABASE NOARCHIVELOG; ALTER DATABASE OPEN;`
- ✅ 达梦：`ALTER DATABASE MOUNT; ALTER DATABASE NORMAL; ALTER DATABASE NOARCHIVELOG; ALTER DATABASE OPEN;`
- ❌ MySQL/PostgreSQL/SQL Server：不支持

> **警告**：关闭归档模式后将无法执行联机物理备份，且会限制时间点恢复能力，不推荐在生产环境使用。

---

## CLI 命令总览

### 全局参数

| 参数            | 说明                                                                         |
| --------------- | ---------------------------------------------------------------------------- |
| `-c/--config`   | 配置文件路径（JSON 格式，必填）                                              |
| `-t/--db-type`  | 数据库类型：`mysql`、`postgresql`、`oracle`、`mssql`、`dameng`（默认 mysql） |
| `--backup-type` | 备份类型：`logical`（逻辑）或 `physical`（物理，默认 logical）               |
| `--output`      | 输出格式：`text`（人类可读）或 `json`（机器可解析，默认 text）               |
| `--notify`      | 操作失败时发送 webhook 通知的 URL（可选，所有子命令通用）                    |

### 子命令

| 命令              | 说明                                 |
| ----------------- | ------------------------------------ |
| `backup`          | 执行数据库备份                       |
| `restore`         | 执行数据库还原                       |
| `list`            | 列出所有备份                         |
| `list-databases`  | 列出所有用户数据库                   |
| `delete`          | 删除指定备份                         |
| `validate`        | 验证备份有效性                       |
| `info`            | 获取备份详细信息                     |
| `register`        | 注册备份到目录库（仅 Oracle/MSSQL）  |
| `unregister`      | 取消注册备份（仅 Oracle/MSSQL）      |
| `verify-status`   | 验证备份状态（仅 Oracle/MSSQL/达梦） |
| `delete-invalid`  | 删除无效备份（仅 Oracle/MSSQL）      |
| `delete-all`      | 删除所有备份                         |
| `enable-archive`  | 启用归档模式（仅 Oracle/达梦）       |
| `disable-archive` | 关闭归档模式（仅 Oracle/达梦）       |
| `list-drivers`    | 列出所有支持的数据库驱动             |
| `validate-config` | 验证配置文件的有效性                 |

### backup 子命令参数

| 参数                  | 说明                                                                              |
| --------------------- | --------------------------------------------------------------------------------- |
| `--backup-mode`       | 备份模式：`full`、`incremental`、`differential`、`level0`、`archive`（默认 full） |
| `--encryption`        | 是否启用加密（物理备份，Oracle/达梦支持）                                         |
| `--encryption-key`    | 加密密钥（需配合 `--encryption` 使用）                                            |
| `--archive-from-lsn`  | 归档备份起始 LSN（仅达梦，配合 `--backup-mode archive` 使用）                     |
| `--archive-until-lsn` | 归档备份结束 LSN（仅达梦，配合 `--backup-mode archive` 使用）                     |

### restore 子命令参数

| 参数                       | 说明                                                                          |
| -------------------------- | ----------------------------------------------------------------------------- |
| `--backup-identifier`      | 备份标识符（Oracle/达梦: TAG 或备份集路径; MySQL/PostgreSQL/MSSQL: 文件路径） |
| `--target-database`        | 还原的目标数据库名（MySQL/PostgreSQL/MSSQL 逻辑还原时指定）                   |
| `--remap-schema`           | 模式映射，格式: `source:target`（仅达梦 dimp 支持）                           |
| `--restore-mode`           | 还原模式：`full`、`archive`、`controlfile`（默认 full）                       |
| `--recovery-point-in-time` | 时间点还原，格式: `2006-01-02T15:04:05`（Oracle/达梦支持）                    |
| `--scn`                    | 按 SCN 还原（仅 Oracle 支持）                                                 |
| `--lsn`                    | 按 LSN 还原（仅达梦支持，配合 `--restore-mode archive` 使用）                 |
| `--no-redo`                | 还原时跳过归档日志应用，即 NOREDO（仅 Oracle 支持）                           |
| `--archive-from-seq`       | 归档还原起始序列号（仅 Oracle，配合 `--restore-mode archive` 使用）           |
| `--archive-until-seq`      | 归档还原结束序列号（仅 Oracle，配合 `--restore-mode archive` 使用）           |

### 管理命令参数

| 命令             | 参数                  | 说明                            |
| ---------------- | --------------------- | ------------------------------- |
| `delete`         | `--delete-identifier` | 删除备份的标识符（必填）        |
| `validate`       | `--validate-id`       | 验证备份的 ID（必填）           |
| `info`           | `--info-id`           | 获取备份信息的 ID（必填）       |
| `register`       | `--register-path`     | 注册备份的路径（必填）          |
| `unregister`     | `--unregister-id`     | 移除备份记录的 ID（必填）       |
| `enable-archive` | `--archive-dest`      | 归档日志目录路径（可选）        |
| `list`           | `--backup-type`       | 备份类型筛选: logical, physical |

---

## 配置项

### 通用配置

| 字段       | 说明                                                           |
| ---------- | -------------------------------------------------------------- |
| `type`     | 数据库类型：`mysql`、`postgresql`、`oracle`、`mssql`、`dameng` |
| `host`     | 主机地址                                                       |
| `port`     | 端口号                                                         |
| `user`     | 用户名                                                         |
| `password` | 密码                                                           |
| `database` | 默认数据库                                                     |
| `ssl_mode` | SSL 连接模式                                                   |

### MySQL 专用

| Extra 键              | 说明                                        | 必填                 |
| --------------------- | ------------------------------------------- | -------------------- |
| `MYSQL_BIN_PATH`      | MySQL 客户端工具目录（含 mysql、mysqldump） | 否，默认从 PATH 查找 |
| `XTRABACKUP_BIN_PATH` | XtraBackup 工具目录                         | 否，默认从 PATH 查找 |
| `DATA_DIR`            | MySQL 数据目录路径                          | 物理还原时必填       |
| `SERVICE_NAME`        | MySQL 系统服务名称                          | 否，默认自动检测     |

### PostgreSQL 专用

| Extra 键       | 说明                                                            | 必填                         |
| -------------- | --------------------------------------------------------------- | ---------------------------- |
| `PG_BIN_PATH`  | PostgreSQL 客户端工具目录（含 psql、pg_dump、pg_basebackup 等） | 否，默认从 PATH 查找         |
| `DATA_DIR`     | PostgreSQL 数据目录路径                                         | 物理还原时必填               |
| `SERVICE_NAME` | PostgreSQL 系统服务名称（Windows）                              | 否，默认 `postgresql-x64-18` |

> **SSLMode 说明**：`ssl_mode` 是顶层配置字段（非 Extra 参数），非空时自动设置 `PGSSLMODE` 环境变量。

### Oracle 专用

| Extra 键             | 说明                                                                                                      | 必填   |
| -------------------- | --------------------------------------------------------------------------------------------------------- | ------ |
| `ORACLE_HOME`        | Oracle 安装目录                                                                                           | **是** |
| `ORACLE_SID`         | Oracle 实例标识                                                                                           | **是** |
| `AUTO_GHOST_CLEANUP` | 是否在备份/还原前自动执行 RMAN 幽灵对象清理（CROSSCHECK + DELETE EXPIRED + DELETE OBSOLETE），默认 `true` | 否     |
| `RETENTION_DAYS`     | 增量备份保留窗口天数（默认 7，仅增量模式生效，控制 RMAN RECOVERY WINDOW）                                 | 否     |
| `PARALLEL_WORKERS`   | 并行工作线程数（默认 2，物理备份生效）                                                                    | 否     |
| `ENABLE_COMPRESSION` | 是否启用压缩备份（默认 `true`）                                                                           | 否     |
| `COMPRESSION_LEVEL`  | 压缩级别（0=默认; 1-3=LOW, 4-6=MEDIUM, 7-9=HIGH）                                                         | 否     |

### SQL Server 专用

| Extra 键    | 说明                                | 必填 |
| ----------- | ----------------------------------- | ---- |
| `INSTANCE`  | SQL Server 命名实例名称             | 否   |
| `AUTH_TYPE` | 认证方式：`sql`（默认）或 `windows` | 否   |

### 达梦 (Dameng) 专用

| Extra 键             | 说明                                             | 必填            |
| -------------------- | ------------------------------------------------ | --------------- |
| `DM_HOME`            | 达梦安装目录，dexp/dimp/dmrman 工具依赖此路径    | **是**          |
| `DM_INSTANCE`        | 达梦实例名，多实例场景需指定                     | 否              |
| `DM_DATA_DIR`        | 数据目录路径，物理备份还原时需要                 | 物理备份时必填  |
| `AUTO_GHOST_CLEANUP` | 是否在物理备份前自动清理归档目录中的幽灵归档文件 | 否，默认 `true` |
| `RETENTION_DAYS`     | 备份保留天数（仅增量模式生效）                   | 否              |
| `PARALLEL_WORKERS`   | 并行工作线程数（默认 2，物理备份生效）           | 否              |
| `ENABLE_COMPRESSION` | 是否启用压缩备份（默认 `true`）                  | 否              |
| `COMPRESSION_LEVEL`  | 压缩级别（0=默认; 1-9）                          | 否              |

> **归档目录**：无需配置，从 `BaseBackupDir` 自动推导为 `{BaseBackupDir}/dameng/archivelog`，用于归档模式启用和 PITR 还原。
