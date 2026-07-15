# 达梦数据库 (Dameng) 备份/还原

## 概述

本工具支持达梦数据库的逻辑备份（dexp/dimp）和物理备份，采用**联机备份 + 脱机还原**的设计：

- **逻辑备份**: 全库导出 (FULL) 和按模式导出 (SCHEMAS)，使用 `dexp`/`dimp`
- **物理备份（联机）**: 通过 `disql` 在数据库运行状态下执行 `BACKUP DATABASE` 语句，支持全量、差异增量、累积增量、归档日志备份（含压缩、加密、并行）
- **物理还原（脱机）**: 通过 `dmrman` 在数据库停止状态下执行 `RESTORE`/`RECOVER`，支持全量、增量、归档还原 + 时间点恢复 (PITR) / LSN 恢复

> **设计说明**：达梦物理备份分**联机**和**脱机**两种方式（参见[官方文档](https://eco.dameng.com/document/dm/zh-cn/pm/backup-restore-combat.html)）：
>
> - **联机备份**：数据库处于运行状态，通过 `disql` 执行 `BACKUP DATABASE` 语句。本工具采用此方式，无需停止数据库即可备份。
> - **脱机备份/还原**：数据库已停止，通过 `dmrman` 工具执行（需指定 `dm.ini` 路径）。本工具仅在**还原**时使用 `dmrman`（还原前会停止服务，还原后重启）。
>
> 早期版本曾使用 `dmrman` 执行备份，但 `dmrman` 是脱机工具，未配置 `dm.ini` 时会挂起，导致备份永不结束且内存持续上涨。已修复为联机备份。

## ⚠️ 归档模式 (ARCHIVELOG) 要求

**达梦数据库的联机物理备份必须处于 ARCHIVELOG 模式，与 Oracle 一致。** 如果数据库未开启归档模式，`disql` 联机备份将报错 `[-7015]: Cannot do online backup when archiving is not enabled` 并可能导致进程挂起。

### 检查归档模式

```sql
-- 通过 disql 查询，返回 Y 表示已开启，N 表示未开启
SELECT ARCH_MODE FROM V$DATABASE;
```

### 开启归档模式

本工具在执行物理备份时会**自动检查并启用归档模式**。归档目录从 `BaseBackupDir` 自动推导（路径为 `{BaseBackupDir}/dameng/archivelog`），无需用户配置。

如果需要手动开启，执行以下 SQL：

```sql
ALTER DATABASE MOUNT;
ALTER DATABASE NORMAL;
ALTER DATABASE ARCHIVELOG;
ALTER DATABASE ADD ARCHIVELOG 'DEST=/opt/dmdbms/data/DAMENG/arch, TYPE=LOCAL, FILE_SIZE=2048, SPACE_LIMIT=204800';
ALTER DATABASE OPEN;
```

参数说明：

| 参数          | 说明                                   |
| ------------- | -------------------------------------- |
| `DEST`        | 归档日志存储路径（需提前创建目录）     |
| `TYPE`        | 归档类型：`LOCAL`（本地归档）          |
| `FILE_SIZE`   | 单个归档文件大小，单位 MB（推荐 2048） |
| `SPACE_LIMIT` | 归档总空间上限，单位 MB（推荐 204800） |

> **注意**：开启归档模式需要将数据库置于 MOUNT 状态，会短暂中断服务。建议在业务低峰期操作。

## 配置参数

在 `config.json` 中配置达梦数据库连接和工具路径：

```json
{
  "type": "dameng",
  "host": "localhost",
  "port": 5236,
  "user": "SYSDBA",
  "password": "your_password",
  "database": "",
  "extra": {
    "DM_HOME": "/opt/dmdbms",
    "DM_INSTANCE": "DMSERVER",
    "DM_DATA_DIR": "/opt/dmdbms/data/DAMENG"
  }
}
```

### Extra 参数说明

| 参数                 | 必填 | 说明                                                                        |
| -------------------- | ---- | --------------------------------------------------------------------------- |
| `DM_HOME`            | 是   | 达梦安装目录，`dexp`/`dimp`/`dmrman`/`disql` 工具依赖此路径                 |
| `DM_INSTANCE`        | 否   | 达梦实例名，多实例场景需指定，默认使用 Database 字段值                      |
| `DM_DATA_DIR`        | 否   | 数据目录路径，物理还原时需要（如 `/opt/dmdbms/data/DAMENG`）                |
| `AUTO_GHOST_CLEANUP` | 否   | 是否在物理备份前自动清理归档目录中不属于当前实例的幽灵归档文件，默认 `true` |

> **归档目录说明**：归档日志目录从 `BaseBackupDir` 自动推导为 `{BaseBackupDir}/dameng/archivelog`，无需在 `extra` 中配置。该目录同时用于：
>
> - 启用归档模式时写入归档日志
> - 还原时通过 `WITH ARCHIVEDIR` 应用归档日志
>
> ### 幽灵归档清理
>
> 在多实例共享归档目录或实例重建等场景下，归档目录中可能残留不属于当前实例归档链的文件（DB_MAGIC/LSN 不匹配），称为**幽灵归档**。这些文件会导致达梦归档校验失败，影响备份和还原。
>
> 本工具在以下时机自动清理幽灵归档：
>
> | 时机       | 条件                                            | 说明                                                         |
> | ---------- | ----------------------------------------------- | ------------------------------------------------------------ |
> | 物理备份前 | `AUTO_GHOST_CLEANUP=true`（默认）且归档目录非空 | 需数据库在线，通过 `V$ARCHIVE_FILE` 查询合法归档路径         |
> | 物理还原前 | 归档目录非空（无条件执行）                      | 必须在停止服务前执行，因为查询 `V$ARCHIVE_FILE` 需数据库在线 |
>
> **清理流程**：
>
> 1. 通过 `disql` 查询 `V$ARCHIVE_FILE` 视图，获取当前实例归档链中所有合法归档文件路径
> 2. 遍历归档目录中的文件，与合法路径集合对比
> 3. 删除不在合法集合中的文件（即幽灵归档）
>
> **安全防护**：
>
> - 合法归档列表为空时**跳过清理**，防止误删所有归档文件（空列表可能是非归档模式或查询解析异常）
> - 路径对比前做 `filepath.Clean` 归一化，避免 Windows 上分隔符不匹配
> - 清理失败仅记录警告日志，**不阻塞**后续备份/还原流程

## 命令示例

### 逻辑备份

```bash
# 全库逻辑备份
db-backup-restore backup -c config.json -t dameng --backup-type logical -target-path /backup/dameng

# 指定模式逻辑备份（在 config.json 中设置 database 字段）
db-backup-restore backup -c config.json -t dameng --backup-type logical -target-path /backup/dameng
```

### 物理备份（联机）

```bash
# 全量物理备份（自动检查并启用归档模式）
db-backup-restore backup -c config.json -t dameng --backup-type physical -target-path /backup/dameng

# 差异增量物理备份（自上次备份以来的变化）
db-backup-restore backup -c config.json -t dameng --backup-type physical --backup-mode incremental -target-path /backup/dameng

# 累积增量物理备份（自上次全量备份以来的所有变化）
db-backup-restore backup -c config.json -t dameng --backup-type physical --backup-mode differential -target-path /backup/dameng

# 独立归档日志备份
db-backup-restore backup -c config.json -t dameng --backup-type physical --backup-mode archive -target-path /backup/dameng

# 按 LSN 范围归档日志备份
db-backup-restore backup -c config.json -t dameng --backup-type physical --backup-mode archive --archive-from-lsn 1000 --archive-until-lsn 5000 -target-path /backup/dameng

# 启用加密和压缩的归档日志备份
db-backup-restore backup -c config.json -t dameng --backup-type physical --backup-mode archive --encryption --encryption-key mypassword -target-path /backup/dameng
```

### 还原（脱机）

> **说明**：达梦还原统一使用默认的 `full` 模式（`--restore-mode full`，可省略），dmrman 的 `RECOVER WITH BACKUPDIR` 自动查找并应用增量备份集，无需区分全量/增量还原。

```bash
# 逻辑还原
db-backup-restore restore -c config.json -t dameng --backup-identifier /backup/dameng/dameng_full_20260703.dmp

# 物理还原（默认 full 模式，自动处理增量链）
db-backup-restore restore -c config.json -t dameng --backup-type physical --backup-identifier /backup/dameng/dm_full_20260703

# 物理还原（归档还原模式）
db-backup-restore restore -c config.json -t dameng --backup-type physical --restore-mode archive --backup-identifier /backup/dameng/dm_arch_20260703

# 时间点恢复 (PITR)
db-backup-restore restore -c config.json -t dameng --backup-type physical --backup-identifier /backup/dameng/dm_full_20260703 --recovery-point-in-time "2026-07-03T10:30:00"

# 按 LSN 恢复（配合归档还原模式）
db-backup-restore restore -c config.json -t dameng --backup-type physical --restore-mode archive --lsn 99999 --backup-identifier /backup/dameng/dm_arch_20260703

# 逻辑还原（模式映射，将源模式数据导入目标模式）
db-backup-restore restore -c config.json -t dameng --backup-identifier /backup/dameng/dameng_full_20260703.dmp --remap-schema SOURCE_SCHEMA:TARGET_SCHEMA
```

### 列出和删除备份

```bash
# 列出备份
db-backup-restore list -c config.json -t dameng

# 删除备份
db-backup-restore delete -c config.json -t dameng --delete-identifier /backup/dameng/dameng_full_20260703.dmp
```

### 归档模式管理

> **注意**：仅 Oracle 和达梦支持归档模式管理操作。

#### 启用归档模式

```bash
# 启用达梦归档模式（指定归档目录）
db-backup-restore enable-archive -c config.json -t dameng --archive-dest c:/work/database_backup/dameng/physical/archivelog

# 启用达梦归档模式（使用默认配置，从 BaseBackupDir 自动推导）
db-backup-restore enable-archive -c config.json -t dameng
```

#### 关闭归档模式

```bash
# 关闭达梦归档模式（将切换为 NOARCHIVELOG 模式）
db-backup-restore disable-archive -c config.json -t dameng
```

> **警告**：关闭归档模式后将无法执行联机物理备份，且会限制时间点恢复能力，不推荐在生产环境使用。

### 验证和检查备份

```bash
# 验证备份集有效性（通过 dmrman VALIDATE BACKUPSET）
db-backup-restore validate -c config.json -t dameng --validate-id /backup/dameng/dm_full_20260703

# 检查备份状态（通过 dmrman CHECK BACKUPSET）
db-backup-restore verify-status -c config.json -t dameng
```

### 模式映射（dimp REMAP_SCHEMA）

逻辑还原时，可通过 `--remap-schema` 参数将源模式的数据导入到目标模式，格式为 `source:target`：

```bash
# 将 SOURCE_SCHEMA 模式的数据导入到 TARGET_SCHEMA 模式
db-backup-restore restore -c config.json -t dameng --backup-identifier /backup/dameng/dameng_full_20260703.dmp --remap-schema SOURCE_SCHEMA:TARGET_SCHEMA
```

## 实时日志输出

物理备份/还原通常耗时较长，本工具采用**逐行流式输出**，将 `disql`/`dmrman` 的每一行输出实时打印到日志，便于排查长时间运行的任务：

- 备份过程的每一行 stdout 通过 `INFO` 级别日志实时输出（字段 `line`）
- 还原过程的每一行 stdout 通过 `INFO` 级别日志实时输出（字段 `line`）
- 所有 stderr 通过 `WARN` 级别日志实时输出
- 同时设置了 2 小时超时保护，防止备份进程挂起导致永不结束和内存持续上涨

> **设计说明**：早期版本使用 `io.ReadAll` 一次性读取全部输出，导致长时间运行的备份进程输出全部缓存在内存中，控制台无任何输出且内存持续上涨。已改为 `bufio.Scanner` 逐行流式读取，输出与内存问题均已修复。该机制对所有数据库（不仅限于达梦）生效。

## 代码结构

达梦备份相关代码分布在以下文件，按职责拆分：

| 文件                                 | 职责                                                                                                                                                                                                                                                                                       |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `internal/backup/dameng.go`          | 通用骨架：`DamengBackup` 结构体、`NewDamengBackup`、`Backup`/`Restore` 分发、`ListBackups`/`DeleteBackup`、`dexp`/`dimp`/`disql`/`dmrman` 命令执行器、服务启停                                                                                                                             |
| `internal/backup/dameng_physical.go` | **物理备份/还原专用**：联机备份脚本构建（`buildFullBackupScript` 等）、脱机还原脚本构建（`buildFullRestoreScript` 等）、归档模式检查与启用（`isArchiveLogMode`/`EnableArchiveLogMode`）、幽灵归档清理（`purgeStaleArchives`/`getValidArchivePaths`/`findStaleArchiveFiles`）、流式日志回调 |
| `internal/backup/dameng_logical.go`  | 逻辑备份/还原：`dexp`/`dimp` 命令构建与执行                                                                                                                                                                                                                                                |

> **归档模式代码位置**：`isArchiveLogMode` 和 `EnableArchiveLogMode` 仅在物理备份中使用，因此放在 `dameng_physical.go` 而非 `dameng.go`，与物理备份流程内聚。

## 原生工具参考

### dexp — 逻辑导出

| 参数             | 说明                                    |
| ---------------- | --------------------------------------- |
| `USERID`         | 连接串，格式: `user/password@host:port` |
| `FILE`           | 导出文件路径                            |
| `LOG`            | 日志文件路径                            |
| `FULL`           | 全库导出: `Y`/`N`                       |
| `SCHEMAS`        | 指定导出模式，逗号分隔                  |
| `COMPRESS`       | 压缩: `Y`/`N`                           |
| `COMPRESS_LEVEL` | 压缩级别: 1-9                           |
| `PARALLEL`       | 并行工作线程数                          |

### dimp — 逻辑导入

| 参数           | 说明                                    |
| -------------- | --------------------------------------- |
| `USERID`       | 连接串，格式: `user/password@host:port` |
| `FILE`         | 导入文件路径                            |
| `LOG`          | 日志文件路径                            |
| `FULL`         | 全库导入: `Y`/`N`                       |
| `SCHEMAS`      | 指定导入模式                            |
| `IGNORE`       | 忽略错误: `Y`/`N`                       |
| `REMAP_SCHEMA` | 模式映射，格式: `source:target`         |

### disql — 联机备份 SQL（本工具用于备份）

`disql` 是达梦的 SQL 客户端，本工具通过它联机执行 `BACKUP` 语句。连接方式：`disql user/password@host:port`。

#### 备份命令（通过 disql 联机执行）

> **语法要点**：disql 的 BACKUP 语句中，`TO` 后的备份名为标识符（不加引号），`BACKUPSET` 后的路径为字符串（单引号），加密密码用 `IDENTIFIED BY "password"`（双引号）。

| 命令                                                                                 | 说明                        |
| ------------------------------------------------------------------------------------ | --------------------------- |
| `BACKUP DATABASE FULL TO name BACKUPSET 'path'`                                      | 全量备份                    |
| `BACKUP DATABASE FULL TO name BACKUPSET 'path' COMPRESSED`                           | 全量备份（压缩）            |
| `BACKUP DATABASE FULL TO name BACKUPSET 'path' IDENTIFIED BY "password"`             | 全量备份（加密）            |
| `BACKUP DATABASE FULL TO name BACKUPSET 'path' PARALLEL n`                           | 全量备份（并行）            |
| `BACKUP DATABASE INCREMENT WITH BACKUPDIR 'dir' TO name BACKUPSET 'path'`            | 差异增量备份                |
| `BACKUP DATABASE INCREMENT CUMULATIVE WITH BACKUPDIR 'dir' TO name BACKUPSET 'path'` | 累积增量备份                |
| `BACKUP ARCHIVELOG ALL TO name BACKUPSET 'path'`                                     | 归档日志备份（全部）        |
| `BACKUP ARCHIVELOG FROM LSN n TO LSN n TO name BACKUPSET 'path'`                     | 归档日志备份（按 LSN 范围） |

#### 其他常用 SQL（通过 disql 执行）

| 用法                                                            | 说明                    |
| --------------------------------------------------------------- | ----------------------- |
| `SELECT ARCH_MODE FROM V$DATABASE;`                             | 查询归档模式（Y/N）     |
| `SELECT USERNAME FROM DBA_USERS WHERE ACCOUNT_STATUS = 'OPEN';` | 查询用户模式列表        |
| `ALTER DATABASE MOUNT;`                                         | 将数据库置于 MOUNT 状态 |
| `ALTER DATABASE ARCHIVELOG;`                                    | 开启归档模式            |
| `ALTER DATABASE ADD ARCHIVELOG 'DEST=..., TYPE=LOCAL, ...';`    | 添加归档配置            |
| `ALTER DATABASE OPEN;`                                          | 打开数据库              |

### dmrman — 脱机还原工具（本工具用于还原/验证）

`dmrman` 是达梦的脱机备份还原工具，需在数据库停止状态下运行。本工具仅在**还原**和**验证**时使用 `dmrman`（还原前会自动停止达梦服务，还原后重启）。

#### 还原命令（通过 dmrman 脱机执行）

> **语法要点**：dmrman 的还原/恢复语句中，路径参数均使用单引号。

| 命令                                                                              | 说明                       |
| --------------------------------------------------------------------------------- | -------------------------- |
| `RESTORE DATABASE 'dir' FROM BACKUPSET 'path'`                                    | 还原数据库                 |
| `RECOVER DATABASE 'dir' FROM BACKUPSET 'path'`                                    | 恢复数据库（应用归档日志） |
| `RECOVER DATABASE 'dir' WITH ARCHIVEDIR 'path'`                                   | 恢复数据库（指定归档目录） |
| `RECOVER DATABASE 'dir' FROM BACKUPSET 'path' UNTIL TIME 'YYYY-MM-DD HH24:MI:SS'` | PITR 恢复                  |
| `RECOVER DATABASE 'dir' WITH ARCHIVEDIR 'path' UNTIL LSN n`                       | 按 LSN 恢复                |
| `RECOVER DATABASE 'dir' UPDATE DB_MAGIC`                                          | 更新数据库魔数             |
| `RESTORE ARCHIVE LOG FROM BACKUPSET 'path' TO ARCHIVEDIR 'path'`                  | 还原归档日志到指定目录     |
| `VALIDATE BACKUPSET 'path'`                                                       | 验证备份集                 |
| `CHECK BACKUPSET`                                                                 | 检查备份状态               |

---

## 自动幽灵归档清理

> **适用场景**：多实例共享归档目录、实例重建后归档目录残留旧实例文件、归档目录被手动复制了外部文件等。

幽灵归档是指归档目录中存在但不在达梦 `V$ARCHIVE_FILE` 视图记录中的文件。这些文件的 DB_MAGIC 或 LSN 范围与当前实例不匹配，可能导致备份失败或还原异常。

本工具提供自动清理机制，通过 `AUTO_GHOST_CLEANUP` 配置项控制（默认启用）：

- **物理备份前**：当 `AUTO_GHOST_CLEANUP=true` 且归档目录已配置时，自动执行清理
- **物理还原前**：无论 `AUTO_GHOST_CLEANUP` 设置如何，只要归档目录已配置就执行清理（因为还原对归档一致性要求更高）

清理逻辑通过以下三个函数实现：

| 函数                    | 位置                                 | 说明                                                              |
| ----------------------- | ------------------------------------ | ----------------------------------------------------------------- |
| `purgeStaleArchives`    | `internal/backup/dameng_physical.go` | 清理入口：查询合法归档 → 找出幽灵文件 → 逐个删除                  |
| `getValidArchivePaths`  | `internal/backup/dameng_physical.go` | 通过 `disql` 查询 `V$ARCHIVE_FILE.ARCH_PATH` 获取合法归档路径集合 |
| `findStaleArchiveFiles` | `internal/backup/dameng_physical.go` | 遍历归档目录，返回不在合法集合中的文件列表                        |

### 配置示例

```json
{
  "type": "dameng",
  "extra": {
    "DM_HOME": "/opt/dmdbms",
    "DM_INSTANCE": "DMSERVER",
    "DM_DATA_DIR": "/opt/dmdbms/data/DAMENG",
    "AUTO_GHOST_CLEANUP": "true"
  }
}
```

> **注意**：如果归档目录由当前实例独占使用，通常无需关闭此功能。仅在归档目录中存在其他实例的合法归档且不应被删除时，才需设置 `AUTO_GHOST_CLEANUP` 为 `false`。
