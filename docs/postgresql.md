# PostgreSQL 数据库备份与还原

PostgreSQL 支持两种备份方式：

- **逻辑备份**：使用 `pg_dump` 工具，备份文件为 SQL 格式，支持单库和多库备份
- **物理备份**：使用 `pg_basebackup`，适用于大规模数据库的快速备份恢复

---

## 📚 相关文档

- [PostgreSQL 逻辑备份与还原](./postgresql_logical.md)
- [PostgreSQL 物理备份与还原](./postgresql_physical.md)

---

## 🔗 官方文档

- [PostgreSQL 备份与恢复指南](https://www.postgresql.org/docs/current/backup.html)
