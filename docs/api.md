# 数据库备份与还原 API 参考

本项目通过 `DatabaseBackup` 接口提供统一的数据库备份与还原操作，支持 MySQL、PostgreSQL、Oracle、SQL Server 四种数据库。所有操作均通过 `backup.NewBackup(cfg)` 创建驱动实例后调用。

---

## 支持的数据库

| 数据库     | 逻辑备份     | 物理备份         | 目录库管理                      | 列出数据库 |
| ---------- | ------------ | ---------------- | ------------------------------- | ---------- |
| MySQL      | ✅ mysqldump | ✅ XtraBackup    | ❌                              | ✅         |
| PostgreSQL | ✅ pg_dump   | ✅ pg_basebackup | ❌（ValidateBackup 仅支持物理） | ✅         |
| Oracle     | ❌           | ✅ RMAN          | ✅                              | ❌         |
| SQL Server | ❌           | ✅ T-SQL         | ✅                              | ✅         |

> **目录库管理**包含：ValidateBackup、RegisterBackup、UnregisterBackup、VerifyBackupStatus、DeleteInvalidBackups 五个操作。Oracle 使用 RMAN 目录库，SQL Server 使用 msdb 系统表；MySQL/PostgreSQL 无内置目录库机制，这些操作返回 NotSupportedError。

---

## 操作说明

### Backup — 备份

执行数据库备份，支持逻辑备份和物理备份两种类型。

| 参数              | 说明                                                                    |
| ----------------- | ----------------------------------------------------------------------- |
| Type              | 备份类型：`logical`（逻辑）或 `physical`（物理）                        |
| Mode              | 备份模式：`full`（全量）、`incremental`（增量）、`differential`（差异） |
| EnableCompression | 是否启用压缩备份                                                        |
| CompressionLevel  | 压缩级别                                                                |
| Encryption        | 是否启用加密                                                            |
| EncryptionKey     | 加密密钥                                                                |
| TargetPath        | 备份目标路径                                                            |
| ArchiveLogDest    | 归档日志目标路径（Oracle）                                              |
| ParallelWorkers   | 并行工作线程数                                                          |
| Timeout           | 超时时间                                                                |

**注意事项**：

- MySQL/PostgreSQL 不支持增量/差异模式，指定时自动降级为全量备份
- 物理备份会备份整个数据库实例，不支持单库备份
- 压缩功能仅 Oracle/PostgreSQL 物理备份生效；MySQL 逻辑备份不支持压缩
- Oracle 备份前若数据库未开启归档模式，会自动启用归档模式

---

### Restore — 还原

执行数据库还原，根据备份标识符自动判断还原方式。

| 参数                | 说明                                           |
| ------------------- | ---------------------------------------------- |
| BackupIdentifier    | 备份标识符（文件路径或备份集 ID）              |
| TargetDatabaseName  | 目标数据库名（逻辑还原时可指定还原到新数据库） |
| Overwrite           | 是否覆盖现有数据库                             |
| RecoveryPointInTime | 时间点还原的目标时间                           |
| BackupID            | 备份集 ID（Oracle/MSSQL）                      |
| BackupType          | 备份类型：`logical` 或 `physical`              |
| Timeout             | 超时时间                                       |

**注意事项**：

- 物理还原需要管理员权限
- 物理还原会还原整个实例，指定目标数据库将被忽略
- PostgreSQL 逻辑还原到新数据库时，会自动创建目标数据库
- MySQL 逻辑还原不会自动创建目标数据库，需提前创建
- 时间点还原（Point-in-Time）仅在 Oracle 和 SQL Server 上支持
- 物理还原过程中，原数据目录会重命名为 `{datadir}_old_{timestamp}` 保留

---

### ListBackups — 列出备份

列出所有备份记录。

**实现模式**：

- MySQL/PostgreSQL：基于文件系统扫描备份目录（FileSystemBackupManager）
- Oracle：查询 RMAN 目录库 `LIST BACKUP SUMMARY`
- SQL Server：查询 msdb 系统表 `msdb.dbo.backupset`

---

### DeleteBackup — 删除备份

删除指定的备份记录及对应的物理备份文件。

**实现模式**：

- MySQL/PostgreSQL：删除文件系统中的备份文件
- Oracle：执行 `DELETE NOPROMPT BACKUPSET <id>`，支持按备份集 ID 或时间删除
- SQL Server：删除 msdb 备份记录 + 删除物理备份文件，支持按 backup_set_id 或时间删除

---

### GetBackupInfo — 获取备份详情

获取指定备份的详细信息。

**实现模式**：

- MySQL/PostgreSQL：读取备份目录下的元数据文件
- Oracle：执行 `LIST BACKUPSET <id>`
- SQL Server：查询 `msdb.dbo.backupset`，backupID 为空时返回最近 10 条

---

### ValidateBackup — 验证备份

验证备份文件的有效性。

**支持情况**：

- ✅ Oracle：`VALIDATE BACKUPSET <id>` 或 `RESTORE DATABASE VALIDATE CHECK LOGICAL`
- ✅ SQL Server：`RESTORE VERIFYONLY FROM DISK = N'<path>'`
- ✅ PostgreSQL：仅物理备份，`pg_verifybackup <backup_path>`
- ❌ MySQL：不支持，返回 NotSupportedError
- ❌ PostgreSQL 逻辑备份：不支持，仅物理备份可验证

---

### RegisterBackup — 注册备份

将外部备份文件注册到目录库。

**支持情况**：

- ✅ Oracle：`CATALOG START WITH '<path>'`
- ✅ SQL Server：`EXEC msdb.dbo.sp_add_backup_filehistory`
- ❌ MySQL/PostgreSQL：不支持

---

### UnregisterBackup — 取消注册备份

从目录库中移除备份记录（不删除物理文件）。

**支持情况**：

- ✅ Oracle：`CHANGE BACKUPSET <id> UNCATALOG`
- ✅ SQL Server：`EXEC msdb.dbo.sp_delete_backuphistory`
- ❌ MySQL/PostgreSQL：不支持

---

### VerifyBackupStatus — 检查备份状态

检查目录库中所有备份记录的有效性，无效记录将被删除。

**支持情况**：

- ✅ Oracle：`CROSSCHECK BACKUP`，过期记录执行 `DELETE NOPROMPT EXPIRED BACKUP`
- ✅ SQL Server：逐条执行 `RESTORE VERIFYONLY`，验证失败则删除记录及物理文件
- ❌ MySQL/PostgreSQL：不支持

---

### DeleteInvalidBackups — 删除无效备份

删除目录库中对应的物理文件已不存在的备份记录。

**支持情况**：

- ✅ Oracle：`DELETE NOPROMPT EXPIRED BACKUP`
- ✅ SQL Server：查询 msdb 记录，检查文件是否存在，不存在则删除记录
- ❌ MySQL/PostgreSQL：不支持

---

### DeleteAllBackups — 删除所有备份

删除所有备份记录及对应的物理备份文件。

**实现模式**：

- MySQL/PostgreSQL：清空备份目录
- Oracle：`DELETE NOPROMPT BACKUP`
- SQL Server：删除 msdb 所有备份相关表 + 删除物理备份文件

---

### ListDatabases — 列出数据库

列出数据库实例中的所有用户数据库。

**支持情况**：

- ✅ MySQL：执行 `SHOW DATABASES`，排除 information_schema、mysql、performance_schema、sys
- ✅ PostgreSQL：执行 `SELECT datname FROM pg_database WHERE datistemplate = false`，排除 postgres
- ✅ SQL Server：执行 `SELECT name FROM sys.databases WHERE name NOT IN ('master','tempdb','model','msdb') AND state = 0`
- ❌ Oracle：不支持（Oracle 基于实例架构，一个实例对应一个数据库）

---

## 配置项

### 通用配置

| 字段       | 说明                                                 |
| ---------- | ---------------------------------------------------- |
| `type`     | 数据库类型：`mysql`、`postgresql`、`oracle`、`mssql` |
| `host`     | 主机地址                                             |
| `port`     | 端口号                                               |
| `user`     | 用户名                                               |
| `password` | 密码                                                 |
| `database` | 默认数据库                                           |
| `ssl_mode` | SSL 连接模式                                         |

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

### Oracle 专用

| Extra 键      | 说明            | 必填   |
| ------------- | --------------- | ------ |
| `ORACLE_HOME` | Oracle 安装目录 | **是** |
| `ORACLE_SID`  | Oracle 实例标识 | **是** |

### SQL Server 专用

| Extra 键    | 说明                                | 必填 |
| ----------- | ----------------------------------- | ---- |
| `INSTANCE`  | SQL Server 命名实例名称             | 否   |
| `AUTH_TYPE` | 认证方式：`sql`（默认）或 `windows` | 否   |
