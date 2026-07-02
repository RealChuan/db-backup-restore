# SQL Server 命令参考

> 详细的 API 说明和各数据库对比请参阅 [API 参考](./api.md)。

---

## 备份（Backup）

### 全量备份

```sql
BACKUP DATABASE [YourDatabaseName] TO DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak' WITH STATS = 10;
```

> **说明**：启用压缩时，在 `WITH` 子句中追加 `COMPRESSION`。

### 差异备份

```sql
BACKUP DATABASE [YourDatabaseName] TO DISK = N'C:\backup\YourDatabaseName_diff_20260415_150405.bak' WITH DIFFERENTIAL, COMPRESSION, STATS = 10;
```

### 事务日志备份

```sql
BACKUP LOG [YourDatabaseName] TO DISK = N'C:\backup\YourDatabaseName_log_20260415_150405.trn' WITH COMPRESSION, STATS = 10;
```

---

## 还原（Restore）

> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。
> 还原过程中数据库将不可用，建议在维护窗口内执行。

### 还原到最新完整备份

```sql
USE master;
RESTORE DATABASE [YourDatabaseName]
FROM DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak'
WITH STATS = 10;
```

> **说明**：覆盖模式下，在 `WITH` 子句中追加 `REPLACE`。

### 按时间点还原（Point-in-Time）

> 只有按时间点还原，才可以完全还原到指定数据库状态，确保所有数据表数据的一致性。

```sql
USE master;
RESTORE DATABASE [YourDatabaseName]
FROM DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak'
WITH STOPAT = '2026-04-15 15:30:00', STATS = 10;
```

> **说明**：覆盖模式下，在 `WITH` 子句中追加 `REPLACE`。

### 还原到新数据库

```sql
USE master;
RESTORE DATABASE [YourDatabaseName_New]
FROM DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak'
WITH REPLACE,
MOVE 'YourDatabaseName_Data' TO 'C:\data\YourDatabaseName_New.mdf',
MOVE 'YourDatabaseName_Log' TO 'C:\data\YourDatabaseName_New.ldf',
STATS = 10;
```

### 差异备份还原

```sql
USE master;
RESTORE DATABASE [YourDatabaseName]
FROM DISK = N'C:\backup\YourDatabaseName_full.bak'
WITH NORECOVERY, STATS = 10;

RESTORE DATABASE [YourDatabaseName]
FROM DISK = N'C:\backup\YourDatabaseName_diff.bak'
WITH RECOVERY, STATS = 10;
```

### 事务日志还原

```sql
USE master;
RESTORE DATABASE [YourDatabaseName]
FROM DISK = N'C:\backup\YourDatabaseName_full.bak'
WITH NORECOVERY, STATS = 10;

RESTORE LOG [YourDatabaseName]
FROM DISK = N'C:\backup\YourDatabaseName_log1.trn'
WITH NORECOVERY, STATS = 10;

RESTORE LOG [YourDatabaseName]
FROM DISK = N'C:\backup\YourDatabaseName_log2.trn'
WITH RECOVERY, STATS = 10;
```

---

## 列出数据库（ListDatabases）

```sql
SELECT name
FROM sys.databases
WHERE name NOT IN ('master', 'tempdb', 'model', 'msdb')
  AND state = 0
ORDER BY name;
```

---

## 备份管理

### 列出备份（ListBackups）

```sql
SELECT
    bs.backup_set_id AS BackupID,
    CASE bs.type
        WHEN 'D' THEN 'FULL'
        WHEN 'I' THEN 'INCREMENTAL'
        WHEN 'L' THEN 'LOG'
        ELSE 'UNKNOWN'
    END AS BackupType,
    bs.backup_start_date AS StartTime,
    bs.backup_finish_date AS CompletionTime,
    bs.backup_size AS Size,
    bs.name AS Tag,
    bmf.physical_device_name AS BackupPath,
    'DISK' AS DeviceType,
    'AVAILABLE' AS Status
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
ORDER BY bs.backup_finish_date DESC;
```

### 获取备份详情（GetBackupInfo）

按备份 ID 查询：

```sql
SELECT * FROM msdb.dbo.backupset WHERE backup_set_id = 123;
```

> 不指定 ID 时，返回最近 10 条备份记录。

### 删除备份（DeleteBackup）

按备份 ID 删除（同时删除相关表记录和物理备份文件）：

```sql
DECLARE @bsid INT = 123;
DELETE rfg FROM msdb.dbo.restorefilegroup rfg
JOIN msdb.dbo.restorehistory rh ON rfg.restore_history_id = rh.restore_history_id
WHERE rh.backup_set_id = @bsid;
DELETE rf FROM msdb.dbo.restorefile rf
JOIN msdb.dbo.restorehistory rh ON rf.restore_history_id = rh.restore_history_id
WHERE rh.backup_set_id = @bsid;
DELETE FROM msdb.dbo.restorehistory WHERE backup_set_id = @bsid;
DELETE FROM msdb.dbo.backupfilegroup WHERE backup_set_id = @bsid;
DELETE FROM msdb.dbo.backupfile WHERE backup_set_id = @bsid;
DELETE FROM msdb.dbo.backupset WHERE backup_set_id = @bsid;
```

### 删除早于指定时间的备份

```sql
EXEC msdb.dbo.sp_delete_backuphistory @oldest_date = '2026-04-01 00:00:00';
```

> 执行后同时删除对应的物理备份文件。

### 验证备份（ValidateBackup）

```sql
RESTORE VERIFYONLY FROM DISK = N'C:\backup\YourDatabaseName.bak' WITH NOUNLOAD;
```

> 传入备份 ID 时，先查询备份路径再验证；传入文件路径时直接验证。

### 从备份文件获取数据库信息

```sql
RESTORE FILELISTONLY FROM DISK = N'C:\backup\YourDatabaseName.bak';
```

### 注册备份（RegisterBackup）

```sql
EXEC msdb.dbo.sp_add_backup_filehistory
    @backup_set_id = NULL,
    @file_name = N'C:\backup\YourDatabaseName.bak';
```

### 取消注册备份（UnregisterBackup）

```sql
EXEC msdb.dbo.sp_delete_backuphistory @backup_set_id = 123;
```

### 检查备份状态（VerifyBackupStatus）

三步流程：查询备份记录 → 逐个验证 → 清理无效记录。

**步骤 1：查询备份记录**

```sql
SELECT
    bs.backup_set_id,
    bmf.physical_device_name
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
WHERE bmf.device_type = 2;
```

**步骤 2：对每条记录验证备份有效性**

```sql
RESTORE VERIFYONLY FROM DISK = N'<backupPath>' WITH NOUNLOAD;
```

**步骤 3：验证失败时删除备份记录和物理文件**

```sql
SET NOCOUNT ON;
DELETE rfg FROM msdb.dbo.restorefilegroup rfg JOIN msdb.dbo.restorehistory rh ON rfg.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = <id>;
DELETE rf FROM msdb.dbo.restorefile rf JOIN msdb.dbo.restorehistory rh ON rf.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = <id>;
DELETE FROM msdb.dbo.restorehistory WHERE backup_set_id = <id>;
DELETE FROM msdb.dbo.backupfilegroup WHERE backup_set_id = <id>;
DELETE FROM msdb.dbo.backupfile WHERE backup_set_id = <id>;
DELETE FROM msdb.dbo.backupset WHERE backup_set_id = <id>;
```

验证失败后，同时删除对应的物理备份文件。

### 删除无效备份（DeleteInvalidBackups）

流程：查询备份记录 → 检查文件是否存在 → 删除不存在的记录。

**步骤 1：查询备份记录**

```sql
SELECT
    bs.backup_set_id,
    bmf.physical_device_name
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
WHERE bmf.device_type = 2;
```

**步骤 2：对每条记录检查文件是否存在**

> 检查 `physical_device_name` 对应的文件是否存在于文件系统。

**步骤 3：文件不存在时删除备份记录**

```sql
SET NOCOUNT ON;
DELETE rfg FROM msdb.dbo.restorefilegroup rfg JOIN msdb.dbo.restorehistory rh ON rfg.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = <id>;
DELETE rf FROM msdb.dbo.restorefile rf JOIN msdb.dbo.restorehistory rh ON rf.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = <id>;
DELETE FROM msdb.dbo.restorehistory WHERE backup_set_id = <id>;
DELETE FROM msdb.dbo.backupfilegroup WHERE backup_set_id = <id>;
DELETE FROM msdb.dbo.backupfile WHERE backup_set_id = <id>;
DELETE FROM msdb.dbo.backupset WHERE backup_set_id = <id>;
```

### 删除所有备份（DeleteAllBackups）

```sql
SET NOCOUNT ON;
DELETE FROM msdb.dbo.restorefilegroup;
DELETE FROM msdb.dbo.restorefile;
DELETE FROM msdb.dbo.restorehistory;
DELETE FROM msdb.dbo.backupfilegroup;
DELETE FROM msdb.dbo.backupfile;
DELETE FROM msdb.dbo.backupset;
DELETE FROM msdb.dbo.backupmediafamily;
DELETE FROM msdb.dbo.backupmediaset;
```

> 执行后同时删除所有对应的物理备份文件。

---

## 辅助查询

### 检查数据库恢复模式

```sql
SELECT name, recovery_model_desc
FROM sys.databases
WHERE name = 'YourDatabaseName';
```

### 查看数据库文件位置

```sql
SELECT name, physical_name
FROM sys.master_files
WHERE database_id = DB_ID('YourDatabaseName');
```

### 查看最近的备份历史

```sql
SELECT TOP 10
    backup_set_id,
    database_name,
    type,
    backup_finish_date,
    backup_size
FROM msdb.dbo.backupset
ORDER BY backup_finish_date DESC;
```

### 检查备份文件是否存在

```sql
EXEC xp_fileexist 'C:\backup\YourDatabaseName.bak';
```

### 获取数据库列表（排除系统数据库）

```sql
SELECT name
FROM sys.databases
WHERE name NOT IN ('master', 'tempdb', 'model', 'msdb')
  AND state = 0
ORDER BY name;
```

---

## 故障处理

### 还原时提示数据库正在使用

```sql
USE master;
ALTER DATABASE [YourDatabaseName] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
RESTORE DATABASE [YourDatabaseName]
FROM DISK = N'C:\backup\YourDatabaseName.bak'
WITH REPLACE, STATS = 10;
ALTER DATABASE [YourDatabaseName] SET MULTI_USER;
```

### 备份文件损坏或无效

```sql
RESTORE VERIFYONLY FROM DISK = N'C:\backup\YourDatabaseName.bak';
```

### 事务日志已满

```sql
BACKUP LOG [YourDatabaseName] TO DISK = N'C:\backup\YourDatabaseName_log.trn';
```

### 数据库处于可疑状态

```sql
USE master;
ALTER DATABASE [YourDatabaseName] SET EMERGENCY;
DBCC CHECKDB([YourDatabaseName], REPAIR_ALLOW_DATA_LOSS);
ALTER DATABASE [YourDatabaseName] SET ONLINE;
```

---

## 异机恢复指南

### 前提条件

- **平台兼容性**：源机器和目标机器都是 Windows 平台
- **SQL Server 版本**：目标数据库的 SQL Server 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保所有备份文件都被完整复制
- **目录结构**：目标数据库的目录结构应与备份时的结构一致，或在恢复时进行调整
- **SQL Server 环境**：目标机器上已正确安装 SQL Server 数据库软件

### 恢复步骤

1. **准备环境**：在目标机器上安装 SQL Server 数据库软件
2. **复制备份文件**：将源机器上的备份文件复制到目标机器的相应目录
3. **还原数据库**：使用 RESTORE DATABASE 命令还原数据库，必要时使用 MOVE 选项调整文件路径

### 命令示例

```sql
USE master;
RESTORE DATABASE [YourDatabaseName]
FROM DISK = N'D:\backup\YourDatabaseName.bak'
WITH REPLACE,
MOVE 'YourDatabaseName_Data' TO 'D:\data\YourDatabaseName.mdf',
MOVE 'YourDatabaseName_Log' TO 'D:\data\YourDatabaseName.ldf',
STATS = 10;
```

### 注意事项

- **目录路径调整**：如果目标机器的目录结构与备份时不同，需要使用 `MOVE` 选项调整文件路径
- **SQL Server 环境**：确保目标机器上的 SQL Server 环境已正确安装
- **备份文件验证**：在恢复前使用 `RESTORE VERIFYONLY` 命令验证备份文件的可用性
- **权限**：确保 SQL Server 服务账户对备份文件和目标目录有足够的权限
