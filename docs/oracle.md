# Oracle 数据库备份与还原命令手册

本文档基于 `backup` 包中的 `OracleBackup` 实现，汇总了备份、还原、归档模式切换、备份管理及恢复操作的底层 **SQLPlus** 和 **RMAN** 命令。这些命令可直接在数据库服务器上执行，也可通过 Go 程序调用。

---

## 1. 归档模式（ARCHIVELOG）管理

### 1.1 查看当前归档状态

```sql
-- SQL*Plus 中执行
ARCHIVE LOG LIST;

-- 或查询 V$DATABASE
SELECT LOG_MODE FROM V$DATABASE;
```

### 1.2 启用归档模式

**前提**：数据库需以 `SYSDBA` 身份登录，且处于 `MOUNT` 状态。

```sql
SHUTDOWN IMMEDIATE;
STARTUP MOUNT;
ALTER DATABASE ARCHIVELOG;
-- 设置归档日志存放路径（可选）
ALTER SYSTEM SET LOG_ARCHIVE_DEST_1='LOCATION=/u01/archivelog' SCOPE=BOTH;
ALTER DATABASE OPEN;
```

> **Windows 路径示例**：`LOCATION=D:\archivelog`

### 1.3 禁用归档模式（不推荐生产环境）

```sql
SHUTDOWN IMMEDIATE;
STARTUP MOUNT;
ALTER DATABASE NOARCHIVELOG;
ALTER DATABASE OPEN;
```

---

## 2. RMAN 备份命令

所有备份命令要求数据库处于 **ARCHIVELOG** 模式。

### 2.1 全量备份（BackupFull）

```rman
RUN {
  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '/backup/%U';
  BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '/backup/%U';
  BACKUP CURRENT CONTROLFILE FORMAT '/backup/cf_%U';
  BACKUP SPFILE FORMAT '/backup/spfile_%U';
  RELEASE CHANNEL ch1;
}
DELETE NOPROMPT OBSOLETE;
```

### 2.2 增量备份（BackupIncremental）

```rman
RUN {
  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '/backup/%U';
  BACKUP INCREMENTAL LEVEL 1 DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '/backup/%U';
  BACKUP CURRENT CONTROLFILE FORMAT '/backup/cf_%U';
  BACKUP SPFILE FORMAT '/backup/spfile_%U';
  RELEASE CHANNEL ch1;
}
DELETE NOPROMPT OBSOLETE;
```

### 2.3 差异备份（BackupDifferential）

在 Oracle 中，差异备份使用 `CUMULATIVE` 关键字：

```rman
RUN {
  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '/backup/%U';
  BACKUP INCREMENTAL LEVEL 1 CUMULATIVE DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '/backup/%U';
  BACKUP CURRENT CONTROLFILE FORMAT '/backup/cf_%U';
  BACKUP SPFILE FORMAT '/backup/spfile_%U';
  RELEASE CHANNEL ch1;
}
DELETE NOPROMPT OBSOLETE;
```

### 2.4 并行备份（Parallelism > 1）

```rman
RUN {
  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '/backup/%U';
  ALLOCATE CHANNEL ch2 DEVICE TYPE DISK FORMAT '/backup/%U';
  BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '/backup/%U';
  BACKUP CURRENT CONTROLFILE FORMAT '/backup/cf_%U';
  BACKUP SPFILE FORMAT '/backup/spfile_%U';
  RELEASE CHANNEL ch1;
  RELEASE CHANNEL ch2;
}
DELETE NOPROMPT OBSOLETE;
```

### 2.5 压缩备份

```rman
CONFIGURE COMPRESSION ALGORITHM 'MEDIUM';
BACKUP AS COMPRESSED BACKUPSET DATABASE PLUS ARCHIVELOG DELETE INPUT;
```

### 2.6 加密备份

```rman
CONFIGURE ENCRYPTION FOR DATABASE ON;
SET ENCRYPTION IDENTIFIED BY 'your_password' ONLY;
BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT;
```

---

## 3. RMAN 还原命令

> **🔄 还原注意事项**
>
> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。
> 还原过程中数据库将不可用，建议在维护窗口内执行。

### 3.1 还原到最新完整备份（默认）

```rman
RUN {
  SHUTDOWN IMMEDIATE;
  STARTUP MOUNT;
  RESTORE DATABASE;
  RECOVER DATABASE;
  ALTER DATABASE OPEN;
}
```

### 3.2 按时间点还原（Point-in-Time）

> **⚠️ 重要注意事项 ⚠️**
>
> 只有按照时间点还原（Point-in-Time），才可以完全还原到指定数据库状态，确保所有数据表数据的一致性。
>
> 其他还原方式（如默认还原或按标签还原）会应用所有归档日志，可能导致部分数据表数据不一致。

```rman
RUN {
  SHUTDOWN IMMEDIATE;
  STARTUP MOUNT;
  SET UNTIL TIME "TO_DATE('2026-04-03 15:30:00', 'YYYY-MM-DD HH24:MI:SS')";
  RESTORE DATABASE;
  RECOVER DATABASE;
  ALTER DATABASE OPEN RESETLOGS;
}
```

### 3.3 按备份标签还原

```rman
RUN {
  SHUTDOWN IMMEDIATE;
  STARTUP MOUNT;
  RESTORE DATABASE FROM TAG='TAG20260408T100801';
  RECOVER DATABASE;
  ALTER DATABASE OPEN;
}
```

### 3.4 还原控制文件丢失的场景

```rman
STARTUP NOMOUNT;
RESTORE CONTROLFILE FROM AUTOBACKUP;
ALTER DATABASE MOUNT;
RESTORE DATABASE;
RECOVER DATABASE;
ALTER DATABASE OPEN RESETLOGS;
```

### 3.5 还原到异机（DUPLICATE）

```rman
DUPLICATE TARGET DATABASE TO newdb
  FROM ACTIVE DATABASE
  SPFILE
  PARAMETER_VALUE_CONVERT '/old_path/','/new_path/'
  SET DB_FILE_NAME_CONVERT '/old_data/','/new_data/';
```

---

## 4. 备份管理命令

> **📊 备份管理指南**
>
> 定期管理备份文件可以确保备份的有效性和可用性，同时避免磁盘空间浪费。

### 4.1 列出所有备份

```rman
LIST BACKUP SUMMARY;
```

输出示例：
```
BS Key  Type LV Size     Device Type Completion Time
------- ---- -- -------- ----------- -------------------
1       Full   120.00M   DISK        2026-04-03 15:34:28
2       Inc    45.00M    DISK        2026-04-04 02:00:12
```

### 4.2 查看备份详细信息

```rman
LIST BACKUP OF DATABASE;
LIST BACKUP OF ARCHIVELOG ALL;
LIST BACKUP OF CONTROLFILE;
```

### 4.3 删除指定备份集

```rman
DELETE NOPROMPT BACKUPSET 123;
```

### 4.4 删除完成时间早于某时间点的所有备份

```rman
DELETE NOPROMPT BACKUP COMPLETED BEFORE "TO_DATE('2026-04-01 00:00:00', 'YYYY-MM-DD HH24:MI:SS')";
```

### 4.5 删除过期备份（根据保留策略）

```rman
DELETE NOPROMPT OBSOLETE;
```

### 4.6 验证备份有效性

```rman
-- 验证整个数据库备份（带逻辑检查）
RESTORE DATABASE VALIDATE CHECK LOGICAL;

-- 验证指定备份集
VALIDATE BACKUPSET 123;
```

### 4.7 交叉核对备份（检查备份文件是否物理存在）

```rman
CROSSCHECK BACKUP;
```

### 4.8 备份目录库管理

#### 4.8.1 注册备份到目录库

```rman
CATALOG START WITH 'D:\backup\rman';
```

#### 4.8.2 从目录库中移除备份记录

```rman
CHANGE BACKUPSET 123 UNCATALOG;
```

#### 4.8.3 检查备份状态并更新目录库

```rman
CROSSCHECK BACKUP;
```

#### 4.8.4 删除无效的备份记录

```rman
DELETE NOPROMPT EXPIRED BACKUP;
```

---

## 5. 备份与还原的 Go 代码映射

> **🔗 代码映射指南**
>
> 本部分列出了 `OracleBackup` 实现中的 Go 方法与底层 RMAN/SQL 命令的对应关系。

| Go 方法 | 对应的底层命令 |
|---------|----------------|
| `EnableArchiveLogMode(ctx, dest)` | `SHUTDOWN IMMEDIATE; STARTUP MOUNT; ALTER DATABASE ARCHIVELOG; ... ALTER DATABASE OPEN;` |
| `Backup(BackupFull)` | `BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT;` |
| `Backup(BackupIncremental)` | `BACKUP INCREMENTAL LEVEL 1 DATABASE PLUS ARCHIVELOG DELETE INPUT;` |
| `Backup(BackupDifferential)` | `BACKUP INCREMENTAL LEVEL 1 CUMULATIVE DATABASE PLUS ARCHIVELOG DELETE INPUT;` |
| `Restore(PointInTime)` | `SET UNTIL TIME ...; RESTORE DATABASE; RECOVER DATABASE; ALTER DATABASE OPEN RESETLOGS;` |
| `Restore(BackupTag)` | `RESTORE DATABASE FROM TAG='<tag>'; RECOVER DATABASE; ALTER DATABASE OPEN;` |
| `ListBackups()` | `LIST BACKUP SUMMARY;`（解析输出） |
| `DeleteBackup(backupID)` | `DELETE NOPROMPT BACKUPSET <id>;` |
| `DeleteBackup(timeRFC3339)` | `DELETE NOPROMPT BACKUP COMPLETED BEFORE TO_DATE(...);` |
| `ValidateBackup(backupID)` | `VALIDATE BACKUPSET <id>;` 或 `RESTORE DATABASE VALIDATE CHECK LOGICAL;` |
| `GetBackupInfo(backupID)` | `LIST BACKUPSET <id>;` 或 `LIST BACKUP OF DATABASE SUMMARY;` |
| `RegisterBackup(backupPath)` | `CATALOG START WITH '<backupPath>';` |
| `UnregisterBackup(backupID)` | `CHANGE BACKUPSET <id> UNCATALOG;` |
| `VerifyBackupStatus()` | `CROSSCHECK BACKUP;` |
| `DeleteInvalidBackups()` | `DELETE NOPROMPT EXPIRED BACKUP;` |

---

## 6. 常用辅助查询

> **🔍 辅助查询指南**
>
> 本部分提供了一些常用的 SQL 查询语句，用于监控数据库状态和备份情况。

### 6.1 检查数据库是否处于归档模式

```sql
SELECT LOG_MODE FROM V$DATABASE;
```

### 6.2 查看归档日志位置

```sql
SHOW PARAMETER LOG_ARCHIVE_DEST;
```

### 6.3 强制切换日志并归档

```sql
ALTER SYSTEM SWITCH LOGFILE;
ALTER SYSTEM ARCHIVE LOG CURRENT;
```

### 6.4 查看备份集信息（SQL）

```sql
SELECT BS_KEY, BACKUP_TYPE, START_TIME, COMPLETION_TIME, STATUS 
FROM V$BACKUP_SET 
ORDER BY COMPLETION_TIME DESC;
```

---

## 7. 典型故障处理命令

> **🛠️ 故障处理指南**
>
> 本部分提供了一些常见故障的处理方法，帮助您快速解决备份和恢复过程中遇到的问题。

### 7.1 恢复时提示归档日志缺失

```rman
-- 取消恢复，改为不完全恢复到缺失点之前
RUN {
  SET UNTIL TIME "TO_DATE('2026-04-03 15:00:00', 'YYYY-MM-DD HH24:MI:SS')";
  RESTORE DATABASE;
  RECOVER DATABASE;
  ALTER DATABASE OPEN RESETLOGS;
}
```

### 7.2 数据库无法打开，提示需要介质恢复

```rman
STARTUP MOUNT;
RECOVER DATABASE;
ALTER DATABASE OPEN;
```

### 7.3 数据文件损坏，单独恢复

```rman
SQL "ALTER DATABASE DATAFILE 4 OFFLINE";
RESTORE DATAFILE 4;
RECOVER DATAFILE 4;
SQL "ALTER DATABASE DATAFILE 4 ONLINE";
```

---

## 8. 异机恢复

> **📋 异机恢复指南**
>
> 异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 8.1 异机恢复的前提条件

- **平台兼容性**：源机器和目标机器都是 Windows 平台
- **Oracle 版本**：目标数据库的 Oracle 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保所有备份文件（数据文件、控制文件、归档日志）都被完整复制
- **目录结构**：目标数据库的目录结构应与备份时的结构一致，或在恢复时进行调整
- **Oracle 环境**：目标机器上已正确安装 Oracle 数据库软件

### 8.2 异机恢复的步骤

1. **准备环境**：在目标机器上安装 Oracle 数据库软件，配置环境变量
2. **复制备份文件**：将源机器上的备份文件复制到目标机器的相应目录
3. **启动实例**：在目标机器上启动 Oracle 实例到 NOMOUNT 状态
4. **注册备份**：使用 RMAN 的 `CATALOG START WITH '备份路径';` 命令将备份文件注册到目标数据库的控制文件
5. **还原控制文件**：如果目标数据库是全新的，需要先还原控制文件
6. **挂载数据库**：将数据库挂载到 MOUNT 状态
7. **还原数据文件**：使用 RMAN 的 `RESTORE DATABASE;` 命令还原数据文件
8. **应用归档日志**：使用 RMAN 的 `RECOVER DATABASE;` 命令应用归档日志
9. **打开数据库**：使用 `ALTER DATABASE OPEN RESETLOGS;` 命令打开数据库

### 8.3 异机恢复的命令示例

#### 8.3.1 注册备份

```rman
CATALOG START WITH 'D:\backup\rman';
```

#### 8.3.2 验证备份状态

```rman
CROSSCHECK BACKUP;
```

#### 8.3.3 还原控制文件

```rman
STARTUP NOMOUNT;
RESTORE CONTROLFILE FROM AUTOBACKUP;
ALTER DATABASE MOUNT;
```

#### 8.3.4 还原和恢复数据库

```rman
RESTORE DATABASE;
RECOVER DATABASE;
ALTER DATABASE OPEN RESETLOGS;
```

### 8.4 异机恢复的注意事项

- **目录路径调整**：如果目标机器的目录结构与备份时不同，需要在恢复前使用 `SET NEWNAME` 命令调整文件路径
- **Oracle 环境**：确保目标机器上的 Oracle 环境已正确安装，并且 ORACLE_HOME 和 ORACLE_SID 已正确设置
- **备份文件验证**：在恢复前使用 `CROSSCHECK BACKUP;` 命令验证备份文件的可用性
- **权限**：确保 Oracle 用户对备份文件和目标目录有足够的权限
- **网络连接**：如果使用网络复制备份文件，确保网络连接稳定，避免文件损坏

---

## 9. 安全建议

> **🔒 安全最佳实践**
>
> 安全的备份策略是保障数据安全的重要组成部分，以下是一些安全最佳实践：

- **定期清理**：定期执行 `DELETE OBSOLETE` 清理过期备份，避免磁盘空间耗尽。
- **异地存储**：对重要备份文件进行异地存储或云备份，确保灾难发生时数据安全。
- **加密保护**：使用加密备份保护敏感数据，防止未授权访问。
- **演练测试**：每季度至少演练一次完整还原流程，确保备份可用。
- **完整性校验**：对备份文件进行校验，确保其完整性和可用性。
- **权限管理**：严格控制备份文件的访问权限，仅授权人员可以访问。
- **备份策略**：根据业务需求制定合理的备份策略，包括备份频率和保留期限。
- **监控告警**：建立备份监控机制，及时发现和处理备份失败的情况。
