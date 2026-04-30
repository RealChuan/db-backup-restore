# MySQL 数据库备份与还原

MySQL 支持两种备份方式：

- **逻辑备份**：使用 `mysqldump` 工具，适用于 InnoDB 引擎，备份文件为 SQL 格式
- **物理备份**：使用 `Percona XtraBackup`，适用于大规模数据库的快速备份恢复

---

## 📚 相关文档

- [MySQL 逻辑备份与还原](./mysql_logical.md)
- [MySQL 物理备份与还原](./mysql_physical.md)

## 🔗 官方文档

- [MySQL 备份与恢复指南](https://dev.mysqlserver.cn/doc/refman/8.4/en/backup-and-recovery.html)
