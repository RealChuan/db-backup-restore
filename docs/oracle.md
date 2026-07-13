# Oracle 命令参考

> 详细的 API 说明和各数据库对比请参阅 [API 参考](./api.md)。

---

## 归档模式管理

归档模式是 Oracle 数据库进行热备份的前提条件。在归档模式下，数据库会将重做日志文件归档保存，从而支持时间点恢复。

### 查看当前归档状态

```sql
-- SQL*Plus 中执行
ARCHIVE LOG LIST;

-- 或查询 V$DATABASE
SELECT LOG_MODE FROM V$DATABASE;
```

**输出说明**：

- `ARCHIVELOG`：数据库处于归档模式
- `NOARCHIVELOG`：数据库处于非归档模式

### 启用归档模式

**前提条件**：数据库需以 `SYSDBA` 身份登录，且处于 `MOUNT` 状态。

```sql
SHUTDOWN IMMEDIATE;
STARTUP MOUNT;
ALTER DATABASE ARCHIVELOG;
-- 设置归档日志存放路径（可选）
ALTER SYSTEM SET LOG_ARCHIVE_DEST_1='LOCATION=/u01/archivelog' SCOPE=BOTH;
ALTER DATABASE OPEN;
```

**Windows 路径示例**：`LOCATION=D:\archivelog`

**CLI 命令**：

```bash
# 启用 Oracle 归档模式（使用默认归档目录）
db-backup-restore enable-archive -c config.json -t oracle

# 启用 Oracle 归档模式（指定归档目录）
db-backup-restore enable-archive -c config.json -t oracle --archive-dest /u01/archivelog
```

### 禁用归档模式

> **警告**：禁用归档模式会限制恢复能力，不推荐在生产环境使用。

```sql
SHUTDOWN IMMEDIATE;
STARTUP MOUNT;
ALTER DATABASE NOARCHIVELOG;
ALTER DATABASE OPEN;
```

**CLI 命令**：

```bash
# 关闭 Oracle 归档模式
db-backup-restore disable-archive -c config.json -t oracle
```

---

## 备份（Backup）

所有备份命令要求数据库处于 **ARCHIVELOG** 模式。

### 全量备份

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

### Level 0 增量基础备份

Level 0 增量备份是增量策略的基础。虽然 Level 0 备份的数据内容与全量备份相同，但 RMAN 会将其记录为增量备份，使其可以作为后续 Level 1 增量备份的父备份。

> **重要**：如果计划使用增量备份策略，必须先创建 Level 0 备份，而非全量备份。全量备份不被 RMAN 视为增量策略的一部分。

```rman
RUN {
  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '/backup/%U';
  BACKUP INCREMENTAL LEVEL 0 DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '/backup/%U';
  BACKUP CURRENT CONTROLFILE FORMAT '/backup/cf_%U';
  BACKUP SPFILE FORMAT '/backup/spfile_%U';
  RELEASE CHANNEL ch1;
}
DELETE NOPROMPT OBSOLETE;
```

### 差异增量备份（Incremental Level 1）

Oracle 的差异增量备份（Differential Incremental）备份自上次 Level 0 或 Level 1 备份以来所有变化的块：

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

### 累积增量备份（Incremental Level 1 Cumulative）

Oracle 的累积增量备份（Cumulative Incremental）备份自上次 Level 0 备份以来所有变化的块。相比差异增量，累积增量备份数据量更大，但还原时只需应用 Level 0 和最新的累积增量备份即可。

> **术语说明**：本工具的 `--backup-mode differential` 对应 Oracle 的累积增量备份（CUMULATIVE），而 `--backup-mode incremental` 对应 Oracle 的差异增量备份（默认 LEVEL 1）。

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

### 独立归档日志备份

在增量备份策略中，通常需要在全量/增量备份之间单独备份归档日志，以减少数据丢失风险：

```rman
RUN {
  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '/backup/arch_%U';
  BACKUP ARCHIVELOG ALL FORMAT '/backup/arch_%U';
  RELEASE CHANNEL ch1;
}
```

### 并行备份

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

### 压缩备份

```rman
RUN {
  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '/backup/%U';
  CONFIGURE COMPRESSION ALGORITHM 'MEDIUM';
  BACKUP AS COMPRESSED BACKUPSET DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '/backup/%U';
  BACKUP CURRENT CONTROLFILE FORMAT '/backup/cf_%U';
  BACKUP SPFILE FORMAT '/backup/spfile_%U';
  RELEASE CHANNEL ch1;
}
DELETE NOPROMPT OBSOLETE;
```

### 加密备份

```rman
RUN {
  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '/backup/%U';
  CONFIGURE ENCRYPTION FOR DATABASE ON;
  SET ENCRYPTION IDENTIFIED BY 'your_password' ONLY;
  BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '/backup/%U';
  BACKUP CURRENT CONTROLFILE FORMAT '/backup/cf_%U';
  BACKUP SPFILE FORMAT '/backup/spfile_%U';
  RELEASE CHANNEL ch1;
}
DELETE NOPROMPT OBSOLETE;
```

---

## 还原（Restore）

> 还原操作会覆盖现有的数据库文件，请在执行还原前确保已备份重要数据。
> 还原过程中数据库将不可用，建议在维护窗口内执行。

### 还原到最新完整备份

```rman
RUN {
  SHUTDOWN IMMEDIATE;
  STARTUP MOUNT;
  RESTORE DATABASE;
  RECOVER DATABASE;
  ALTER DATABASE OPEN;
}
```

### 按时间点还原（Point-in-Time）

> **重要**：只有按指定时间点还原，才可以完全还原到指定数据库状态，确保所有数据表数据的一致性。
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

### 按备份标签还原

```rman
RUN {
  SHUTDOWN IMMEDIATE;
  STARTUP MOUNT;
  RESTORE DATABASE FROM TAG='TAG20260408T100801';
  RECOVER DATABASE;
  ALTER DATABASE OPEN;
}
```

### 还原控制文件丢失场景

```rman
STARTUP NOMOUNT;
RESTORE CONTROLFILE FROM AUTOBACKUP;
ALTER DATABASE MOUNT;
RESTORE DATABASE;
RECOVER DATABASE;
ALTER DATABASE OPEN RESETLOGS;
```

### 异机还原

```rman
DUPLICATE TARGET DATABASE TO newdb
  FROM ACTIVE DATABASE
  SPFILE
  PARAMETER_VALUE_CONVERT '/old_path/','/new_path/'
  SET DB_FILE_NAME_CONVERT '/old_data/','/new_data/';
```

### 增量还原（NOREDO 模式）

跳过归档日志应用，仅还原数据文件：

```bash
db-backup-restore restore -c config.json -t oracle --backup-type physical \
  --restore-mode incremental --backup-identifier /backup/oracle --no-redo
```

### 归档还原（按序列号范围）

```bash
# 还原归档日志序列号 100 到 200
db-backup-restore restore -c config.json -t oracle --backup-type physical \
  --restore-mode archive --archive-from-seq 100 --archive-until-seq 200
```

### 控制文件还原（控制文件丢失的灾难恢复）

```bash
# 从自动备份还原控制文件
db-backup-restore restore -c config.json -t oracle --backup-type physical --restore-mode controlfile

# 指定 TAG 还原控制文件
db-backup-restore restore -c config.json -t oracle --backup-type physical \
  --restore-mode controlfile --backup-identifier TAG20260703T120000
```

### 按 SCN 还原

```bash
# 按 SCN 还原
db-backup-restore restore -c config.json -t oracle --backup-type physical --scn 123456789

# 按 TAG + SCN 组合还原
db-backup-restore restore -c config.json -t oracle --backup-type physical \
  --backup-identifier TAG20260703T120000 --scn 123456789
```

---

### 自动幽灵对象清理

在备份或还原前自动执行 RMAN 幽灵对象清理，消除控制文件/恢复目录中引用了已不存在物理文件的"幽灵"记录，避免后续操作因找不到文件而报错。

#### 触发时机

| 操作    | 触发条件                                             |
| ------- | ---------------------------------------------------- |
| Backup  | `AUTO_GHOST_CLEANUP` 配置为 `true`（默认）时自动触发 |
| Restore | **无条件触发**，不受 `AUTO_GHOST_CLEANUP` 配置控制   |

#### RMAN 命令序列

清理脚本依次执行以下 5 条 RMAN 命令：

```rman
CROSSCHECK BACKUP;
CROSSCHECK ARCHIVELOG ALL;
DELETE NOPROMPT EXPIRED BACKUP;
DELETE NOPROMPT EXPIRED ARCHIVELOG ALL;
DELETE NOPROMPT OBSOLETE;
```

| 步骤 | 命令                                     | 作用                                   |
| ---- | ---------------------------------------- | -------------------------------------- |
| 1    | `CROSSCHECK BACKUP`                      | 标记物理文件已缺失的备份集为 EXPIRED   |
| 2    | `CROSSCHECK ARCHIVELOG ALL`              | 标记物理文件已缺失的归档日志为 EXPIRED |
| 3    | `DELETE NOPROMPT EXPIRED BACKUP`         | 删除 EXPIRED 状态的备份记录            |
| 4    | `DELETE NOPROMPT EXPIRED ARCHIVELOG ALL` | 删除 EXPIRED 状态的归档日志记录        |
| 5    | `DELETE NOPROMPT OBSOLETE`               | 删除按保留策略已过期的备份             |

#### 配置控制

通过 Oracle Extra 参数 `AUTO_GHOST_CLEANUP` 控制备份前是否执行清理：

- `"true"`（默认）：备份前自动执行清理
- `"false"`：备份前跳过清理

> **注意**：还原前始终执行清理，不受此参数影响。

#### 失败处理

清理失败仅记录警告日志，**不阻塞后续备份/还原流程**。

---

## 备份管理

定期管理备份文件可以确保备份的有效性和可用性，同时避免磁盘空间浪费。

### 列出备份（ListBackups）

```rman
LIST BACKUP SUMMARY;
```

**输出示例**：

```
BS Key  Type LV Size     Device Type Completion Time
------- ---- -- -------- ----------- -------------------
1       Full   120.00M   DISK        2026-04-03 15:34:28
2       Inc    45.00M    DISK        2026-04-04 02:00:12
```

### 获取备份详情（GetBackupInfo）

```rman
LIST BACKUP OF DATABASE;
LIST BACKUP OF ARCHIVELOG ALL;
LIST BACKUP OF CONTROLFILE;
```

### 删除备份（DeleteBackup）

按备份集 Key 删除：

```rman
DELETE NOPROMPT BACKUPSET 123;
```

删除早于指定时间的备份：

```rman
DELETE NOPROMPT BACKUP COMPLETED BEFORE "TO_DATE('2026-04-01 00:00:00', 'YYYY-MM-DD HH24:MI:SS')";
```

### 删除过期备份

```rman
DELETE NOPROMPT OBSOLETE;
```

### 验证备份（ValidateBackup）

```rman
-- 验证整个数据库备份（带逻辑检查）
RESTORE DATABASE VALIDATE CHECK LOGICAL;

-- 验证指定备份集
VALIDATE BACKUPSET 123;
```

### 交叉核对备份

检查备份文件是否物理存在：

```rman
CROSSCHECK BACKUP;
```

### 注册备份（RegisterBackup）

```rman
CATALOG START WITH 'D:\backup\rman';
```

### 取消注册备份（UnregisterBackup）

```rman
CHANGE BACKUPSET 123 UNCATALOG;
```

### 检查备份状态（VerifyBackupStatus）

```rman
CROSSCHECK BACKUP;
```

### 删除无效备份（DeleteInvalidBackups）

```rman
DELETE NOPROMPT EXPIRED BACKUP;
```

### 删除所有备份（DeleteAllBackups）

```rman
DELETE NOPROMPT BACKUP;
```

---

## 辅助查询

### 检查数据库是否处于归档模式

```sql
SELECT LOG_MODE FROM V$DATABASE;
```

### 查看归档日志位置

```sql
SHOW PARAMETER LOG_ARCHIVE_DEST;
```

### 强制切换日志并归档

```sql
ALTER SYSTEM SWITCH LOGFILE;
ALTER SYSTEM ARCHIVE LOG CURRENT;
```

### 查看备份集信息（SQL）

```sql
SELECT BS_KEY, BACKUP_TYPE, START_TIME, COMPLETION_TIME, STATUS
FROM V$BACKUP_SET
ORDER BY COMPLETION_TIME DESC;
```

---

## 故障处理

### 恢复时提示归档日志缺失

```rman
RUN {
  SET UNTIL TIME "TO_DATE('2026-04-03 15:00:00', 'YYYY-MM-DD HH24:MI:SS')";
  RESTORE DATABASE;
  RECOVER DATABASE;
  ALTER DATABASE OPEN RESETLOGS;
}
```

### 数据库无法打开，提示需要介质恢复

```rman
STARTUP MOUNT;
RECOVER DATABASE;
ALTER DATABASE OPEN;
```

### 数据文件损坏，单独恢复

```rman
SQL "ALTER DATABASE DATAFILE 4 OFFLINE";
RESTORE DATAFILE 4;
RECOVER DATAFILE 4;
SQL "ALTER DATABASE DATAFILE 4 ONLINE";
```

---

## 异机恢复指南

异机恢复是将数据库从一台机器恢复到另一台机器的过程，适用于灾难恢复、系统迁移等场景。

### 前提条件

- **平台兼容性**：源机器和目标机器都是 Windows 平台
- **Oracle 版本**：目标数据库的 Oracle 版本应与备份时的版本相同或更高
- **备份文件完整性**：确保所有备份文件（数据文件、控制文件、归档日志）都被完整复制
- **目录结构**：目标数据库的目录结构应与备份时的结构一致，或在恢复时进行调整
- **Oracle 环境**：目标机器上已正确安装 Oracle 数据库软件

### 恢复步骤

1. **准备环境**：在目标机器上安装 Oracle 数据库软件，配置环境变量
2. **复制备份文件**：将源机器上的备份文件复制到目标机器的相应目录
3. **启动实例**：在目标机器上启动 Oracle 实例到 NOMOUNT 状态
4. **注册备份**：使用 RMAN 的 `CATALOG START WITH '备份路径';` 命令将备份文件注册到目标数据库的控制文件
5. **还原控制文件**：如果目标数据库是全新的，需要先还原控制文件
6. **挂载数据库**：将数据库挂载到 MOUNT 状态
7. **还原数据文件**：使用 RMAN 的 `RESTORE DATABASE;` 命令还原数据文件
8. **应用归档日志**：使用 RMAN 的 `RECOVER DATABASE;` 命令应用归档日志
9. **打开数据库**：使用 `ALTER DATABASE OPEN RESETLOGS;` 命令打开数据库

### 命令示例

#### 注册备份

```rman
CATALOG START WITH 'D:\backup\rman';
```

#### 验证备份状态

```rman
CROSSCHECK BACKUP;
```

#### 还原控制文件

```rman
STARTUP NOMOUNT;
RESTORE CONTROLFILE FROM AUTOBACKUP;
ALTER DATABASE MOUNT;
```

#### 还原和恢复数据库

```rman
RESTORE DATABASE;
RECOVER DATABASE;
ALTER DATABASE OPEN RESETLOGS;
```

### 注意事项

- **目录路径调整**：如果目标机器的目录结构与备份时不同，需要在恢复前使用 `SET NEWNAME` 命令调整文件路径
- **Oracle 环境**：确保目标机器上的 Oracle 环境已正确安装，并且 ORACLE_HOME 和 ORACLE_SID 已正确设置
- **备份文件验证**：在恢复前使用 `CROSSCHECK BACKUP;` 命令验证备份文件的可用性
- **权限**：确保 Oracle 用户对备份文件和目标目录有足够的权限
- **网络连接**：如果使用网络复制备份文件，确保网络连接稳定，避免文件损坏

---

## 官方文档

- [Oracle RMAN 备份与恢复官方文档](https://docs.oracle.com/en/database/oracle/oracle-database/19/bradv/index.html)
