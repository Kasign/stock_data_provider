# Quant Repository Port 与不可变结果记录

## 本阶段交付

新增 `result.Record` 与 `result.Repository` 写入端口：

- Record 由已构造的 Manifest 和原始 payload 组成，不把未冻结的 QuantResult Envelope 假设写入存储模型。
- `Record.Validate` 调用 Manifest.VerifyPayload，确保内容寻址身份、Manifest 指纹和实际 payload 指纹一致。
- Repository 当前只定义 Save 写入端口，不暴露 Get/Query；在 QuantResultRef/Envelope 关系及读取时授权复核流程冻结前，不提供可绕过授权的结果读取接口。
- Save 语义固定：相同 result_id、相同内容为幂等成功；相同 result_id、不同内容返回 ErrResultConflict。
- Repository 实现必须通过数据库唯一约束/原子写入保证并发下的幂等与冲突判定，不能采用先查后写的非原子流程。

## 明确不在本阶段

- 不实现 MariaDB/SQL/Platform Storage Adapter，不新增根 Migration。
- 不定义表结构、索引、保留策略或事务跨 Outbox 的具体实现。
- 不定义 Envelope、payload discriminator 子 Schema 或 GetResult 返回结构。
- 不在 Repository 中执行授权签发或读取授权；读取端口待授权复核契约冻结后另行设计。

## 验收

Record 测试覆盖合法 Manifest/payload 绑定、payload 篡改和 Manifest 身份篡改。数据库并发幂等、冲突、事务和历史查询属于真实 Storage/L3 集成验收，不能由本阶段的纯单元测试替代。


## 保存前内存隔离

- 新增 `Record.ValidatedSnapshot`：先执行 `Record.Validate`，再深拷贝 Manifest 的快照引用切片和 payload 字节。
- Repository 实现若要保留 Record、交给异步任务或在返回后继续处理，必须先取得并使用该快照；不得仅复制 Record struct，因为其切片字段仍共享底层数组。
- 快照只隔离 Go 进程内的可变内存，不代表业务 payload Schema 已验证，也不提供跨进程/持久化层的不可变性；持久化仍须依靠内容寻址、事务和唯一约束。
- 单元测试覆盖源记录修改不影响快照、快照修改不影响源记录，以及无效记录不能生成已校验快照。
