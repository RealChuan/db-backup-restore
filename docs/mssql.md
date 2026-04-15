# SQL Server 数据库备份与还原命令手册

本文档基于 `backup` 包中的 `MSSQLBackup` 实现，汇总了备份、还原、备份管理及恢复操作的底层 **sqlcmd** 和 **T-SQL** 命令。这些命令可直接在数据库服务器上执行，也可通过 Go 程序调用。

---

## 1. 备份命令

### 1.1 全量备份

```sql
BACKUP DATABASE [YourDatabaseName] 
TO DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak' 
WITH COMPRESSION, STATS = 10;
```

### 1.2 带描述的备份

```sql
BACKUP DATABASE [YourDatabaseName] 
TO DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak' 
WITH COMPRESSION, DESCRIPTION = N'Full backup before migration', STATS = 10;
```

### 1.3 差异备份

```sql
BACKUP DATABASE [YourDatabaseName] 
TO DISK = N'C:\backup\YourDatabaseName_diff_20260415_150405.bak' 
WITH DIFFERENTIAL, COMPRESSION, STATS = 10;
```

### 1.4 事务日志备份

```sql
BACKUP LOG [YourDatabaseName] 
TO DISK = N'C:\backup\YourDatabaseName_log_20260415_150405.trn' 
WITH COMPRESSION, STATS = 10;
```

---

## 2. 还原命令

> **🔄 还原注意事项**
>
> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。
> 还原过程中数据库将不可用，建议在维护窗口内执行。

### 2.1 还原到最新完整备份（默认）

```sql
USE master;
RESTORE DATABASE [YourDatabaseName] 
FROM DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak' 
WITH REPLACE, STATS = 10;
```

### 2.2 按时间点还原（Point-in-Time）

> **⚠️ 重要注意事项 ⚠️**
>
> 只有按照时间点还原（Point-in-Time），才可以完全还原到指定数据库状态，确保所有数据表数据的一致性。

```sql
USE master;
RESTORE DATABASE [YourDatabaseName] 
FROM DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak' 
WITH REPLACE, STOPAT = '2026-04-15 15:30:00', STATS = 10;
```

### 2.3 还原到新数据库

```sql
USE master;
RESTORE DATABASE [YourDatabaseName_New] 
FROM DISK = N'C:\backup\YourDatabaseName_20260415_150405.bak' 
WITH REPLACE, 
MOVE 'YourDatabaseName_Data' TO 'C:\data\YourDatabaseName_New.mdf',
MOVE 'YourDatabaseName_Log' TO 'C:\data\YourDatabaseName_New.ldf',
STATS = 10;
```

### 2.4 差异备份还原

```sql
USE master;
RESTORE DATABASE [YourDatabaseName] 
FROM DISK = N'C:\backup\YourDatabaseName_full.bak' 
WITH NORECOVERY, STATS = 10;

RESTORE DATABASE [YourDatabaseName] 
FROM DISK = N'C:\backup\YourDatabaseName_diff.bak' 
WITH RECOVERY, STATS = 10;
```

### 2.5 事务日志还原

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

## 3. 备份管理命令

> **📊 备份管理指南**
>
> 定期管理备份文件可以确保备份的有效性和可用性，同时避免磁盘空间浪费。

### 3.1 列出所有备份

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

### 3.2 查看备份详细信息

```sql
SELECT * FROM msdb.dbo.backupset WHERE backup_set_id = 123;
```

### 3.3 删除指定备份记录

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

### 3.4 删除完成时间早于某时间点的所有备份记录

```sql
EXEC msdb.dbo.sp_delete_backuphistory @oldest_date = '2026-04-01 00:00:00';
```

### 3.5 验证备份有效性

```sql
RESTORE VERIFYONLY FROM DISK = N'C:\backup\YourDatabaseName.bak' WITH NOUNLOAD;
```

### 3.6 从备份文件中获取数据库信息

```sql
RESTORE FILELISTONLY FROM DISK = N'C:\backup\YourDatabaseName.bak';
```

### 3.7 注册备份到目录库

```sql
EXEC msdb.dbo.sp_add_backup_filehistory 
    @backup_set_id = NULL,
    @file_name = N'C:\backup\YourDatabaseName.bak';
```

### 3.8 从目录库中移除备份记录

```sql
EXEC msdb.dbo.sp_delete_backuphistory @backup_set_id = 123;
```

### 3.9 检查备份状态并更新目录库

```sql
DECLARE @backupSetId INT;
DECLARE backup_cursor CURSOR FOR
SELECT backup_set_id FROM msdb.dbo.backupset;

OPEN backup_cursor;
FETCH NEXT FROM backup_cursor INTO @backupSetId;

WHILE @@FETCH_STATUS = 0
BEGIN
    BEGIN TRY
        RESTORE VERIFYONLY FROM DISK = (
            SELECT physical_device_name 
            FROM msdb.dbo.backupmediafamily 
            WHERE media_set_id = (SELECT media_set_id FROM msdb.dbo.backupset WHERE backup_set_id = @backupSetId)
        );
    END TRY
    BEGIN CATCH
        UPDATE msdb.dbo.backupset SET is_valid = 0 WHERE backup_set_id = @backupSetId;
    END CATCH
    FETCH NEXT FROM backup_cursor INTO @backupSetId;
END

CLOSE backup_cursor;
DEALLOCATE backup_cursor;
```

### 3.10 删除无效的备份记录

```sql
DELETE FROM msdb.dbo.backupset WHERE is_valid = 0;
```

---

## 4. 备份与还原的 Go 代码映射

> **🔗 代码映射指南**
>
> 本部分列出了 `MSSQLBackup` 实现中的 Go 方法与底层 sqlcmd/T-SQL 命令的对应关系。

| Go 方法 | 对应的底层命令 |
|---------|----------------|
| `Backup()` | `BACKUP DATABASE [...] TO DISK = N'<path>' WITH [...]` |
| `Restore()` | `RESTORE DATABASE [...] FROM DISK = N'<path>' WITH [...]` |
| `Restore(PointInTime)` | `RESTORE DATABASE [...] WITH STOPAT = '<time>', [...]` |
| `ListBackups()` | `SELECT * FROM msdb.dbo.backupset JOIN msdb.dbo.backupmediafamily ...` |
| `DeleteBackup(backupID)` | `DELETE FROM msdb.dbo.backupset WHERE backup_set_id = <id>;` |
| `DeleteBackup(time)` | `EXEC msdb.dbo.sp_delete_backuphistory @oldest_date = '<time>';` |
| `ValidateBackup(backupPath)` | `RESTORE VERIFYONLY FROM DISK = N'<path>';` |
| `GetBackupInfo(backupID)` | `SELECT * FROM msdb.dbo.backupset WHERE backup_set_id = <id>;` |
| `RegisterBackup(backupPath)` | `EXEC msdb.dbo.sp_add_backup_filehistory @file_name = N'<path>';` |
| `UnregisterBackup(backupID)` | `EXEC msdb.dbo.sp_delete_backuphistory @backup_set_id = <id>;` |
| `VerifyBackupStatus()` | 遍历所有备份执行 `RESTORE VERIFYONLY` |
| `DeleteInvalidBackups()` | `DELETE FROM msdb.dbo.backupset WHERE is_valid = 0;` |

---

## 5. 常用辅助查询

> **🔍 辅助查询指南**
>
> 本部分提供了一些常用的 SQL 查询语句，用于监控数据库状态和备份情况。

### 5.1 检查数据库恢复模式

```sql
SELECT name, recovery_model_desc 
FROM sys.databases 
WHERE name = 'YourDatabaseName';
```

### 5.2 查看数据库文件位置

```sql
SELECT name, physical_name 
FROM sys.master_files 
WHERE database_id = DB_ID('YourDatabaseName');
```

### 5.3 查看最近的备份历史

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

### 5.4 检查备份文件是否存在

```sql
EXEC xp_fileexist 'C:\backup\YourDatabaseName.bak';
```

### 5.5 获取数据库列表（排除系统数据库）

```sql
SELECT name 
FROM sys.databases 
WHERE name NOT IN ('tempdb') 
  AND state = 0 
ORDER BY name;
```

---

## 6. 典型故障处理命令

> **🛠️ 故障处理指南**
>
> 本部分提供了一些常见故障的处理方法，帮助您快速解决备份和恢复过程中遇到的问题。

### 6.1 还原时提示数据库正在使用

```sql
USE master;
ALTER DATABASE [YourDatabaseName] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
RESTORE DATABASE [YourDatabaseName] 
FROM DISK = N'C:\backup\YourDatabaseName.bak' 
WITH REPLACE, STATS = 10;
ALTER DATABASE [YourDatabaseName] SET MULTI_USER;
```

### 6.2 备份文件损坏或无效

```sql
RESTORE VERIFYONLY FROM DISK = N'C:\backup\YourDatabaseName.bak';
```

### 6.3 事务日志已满

```sql
BACKUP LOG [YourDatabaseName] TO DISK = N'C:\backup\YourDatabaseName_log.trn';
```

### 6.4 数据库处于可疑状态

```sql
USE master;
ALTER DATABASE [YourDatabaseName] SET EMERGENCY;
DBCC CHECKDB([YourDatabaseName], REPAIR_ALLOW_DATA_LOSS);
ALTER DATABASE [YourDatabaseName] SET ONLINE;
```

---

## 7. 异机恢复

> **📋 异机恢复指南**
>
> 异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 7.1 异机恢复的前提条件

- **平台兼容性**：源机器和目标机器都是 Windows 平台
- **SQL Server 版本**：目标数据库的 SQL Server 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保所有备份文件都被完整复制
- **目录结构**：目标数据库的目录结构应与备份时的结构一致，或在恢复时进行调整
- **SQL Server 环境**：目标机器上已正确安装 SQL Server 数据库软件

### 7.2 异机恢复的步骤

1. **准备环境**：在目标机器上安装 SQL Server 数据库软件
2. **复制备份文件**：将源机器上的备份文件复制到目标机器的相应目录
3. **还原数据库**：使用 RESTORE DATABASE 命令还原数据库，必要时使用 MOVE 选项调整文件路径

### 7.3 异机恢复的命令示例

```sql
USE master;
RESTORE DATABASE [YourDatabaseName] 
FROM DISK = N'D:\backup\YourDatabaseName.bak' 
WITH REPLACE,
MOVE 'YourDatabaseName_Data' TO 'D:\data\YourDatabaseName.mdf',
MOVE 'YourDatabaseName_Log' TO 'D:\data\YourDatabaseName.ldf',
STATS = 10;
```

### 7.4 异机恢复的注意事项

- **目录路径调整**：如果目标机器的目录结构与备份时不同，需要使用 `MOVE` 选项调整文件路径
- **SQL Server 环境**：确保目标机器上的 SQL Server 环境已正确安装
- **备份文件验证**：在恢复前使用 `RESTORE VERIFYONLY` 命令验证备份文件的可用性
- **权限**：确保 SQL Server 服务账户对备份文件和目标目录有足够的权限

---

## 8. 安全建议

> **🔒 安全最佳实践**
>
> 安全的备份策略是保障数据安全的重要组成部分，以下是一些安全最佳实践：

- **定期清理**：定期清理过期备份，避免磁盘空间耗尽。
- **异地存储**：对重要备份文件进行异地存储或云备份，确保灾难发生时数据安全。
- **加密保护**：使用加密备份保护敏感数据，防止未授权访问。
- **演练测试**：每季度至少演练一次完整还原流程，确保备份可用。
- **完整性校验**：对备份文件进行校验，确保其完整性和可用性。
- **权限管理**：严格控制备份文件的访问权限，仅授权人员可以访问。
- **备份策略**：根据业务需求制定合理的备份策略，包括备份频率和保留期限。
- **监控告警**：建立备份监控机制，及时发现和处理备份失败的情况。