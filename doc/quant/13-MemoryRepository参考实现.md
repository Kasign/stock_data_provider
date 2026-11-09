# QNT-013 内存 Repository 参考实现

## 本次实现

新增 `result.MemoryRepository`，实现现有 `result.Repository` 写入端口：

- 使用互斥锁将 result_id 唯一性检查与写入作为单一原子操作。
- 保存前调用 `Record.ValidatedSnapshot`，隔离调用方后续修改。
- 相同 result_id 且 Manifest/payload 指纹相同，幂等成功。
- 相同 result_id 但内容指纹不同，返回 `ErrResultConflict`。
- 遵守 context 取消，不在取消后继续写入。
- 不提供读取方法，避免绕过尚未完成的读取授权复核。

## 使用边界

该实现只用于单元测试和本地组合测试。它不持久化、不支持跨进程幂等，也不替代 MariaDB/Platform Storage Adapter。生产环境仍需数据库唯一约束、事务/原子 upsert 和并发冲突测试。

## 验收覆盖

- 首次保存与相同内容重复保存。
- 同身份不同内容冲突。
- 保存后调用方修改 payload/快照引用不影响仓库记录。
- Context 取消不落库。
- 多 goroutine 并发写入同一记录只保留一份。
