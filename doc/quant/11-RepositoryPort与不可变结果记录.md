# Quant Repository Port 与不可变结果记录

## 本阶段交付

新增 `result.Record` 与 `result.Repository` 写入端口：

- Record 由已构造的 Manifest 和原始 payload 组成，不把未冻结的 QuantResult Envelope 假设写入存储模型。
- `Record.Validate` 调用 Manifest.VerifyPayload，确保内容寻址身份、Manifest 指纹和实际 payload 指纹一致；Manifest 的快照引用在构造和复核时均拒绝非法 UTF-8，避免 JSON 序列化替换字节后改变引用内容。
- Repository 仍只定义 Save 写入端口，不暴露裸 Get/Query；结果读取统一由 `application.GetResult` 通过 `AuthorizedResultReader` Port 完成，避免消费者绕过授权。
- Save 语义固定：相同 result_id、相同内容为幂等成功；相同 result_id、不同内容返回 ErrResultConflict。
- Repository 实现必须通过数据库唯一约束/原子写入保证并发下的幂等与冲突判定，不能采用先查后写的非原子流程。

## 明确不在本阶段

- 不实现 MariaDB/SQL/Platform Storage Adapter，不新增根 Migration。
- 不定义表结构、索引、保留策略或事务跨 Outbox 的具体实现。
- 不由 Repository 定义 GetResult；完整 Envelope 由 Application 从已验证 Manifest 和已注册强类型 Payload 构造。
- 不在 Repository 中执行授权签发或读取授权；读取端口待授权复核契约冻结后另行设计。

## 验收

Record 测试覆盖合法 Manifest/payload 绑定、payload 篡改和 Manifest 身份篡改。数据库并发幂等、冲突、事务和历史查询属于真实 Storage/L3 集成验收，不能由本阶段的纯单元测试替代。


## 保存前内存隔离

- 新增 `Record.ValidatedSnapshot`：先执行 `Record.Validate`，再深拷贝 Manifest 的快照引用切片和 payload 字节。
- Repository 实现若要保留 Record、交给异步任务或在返回后继续处理，必须先取得并使用该快照；不得仅复制 Record struct，因为其切片字段仍共享底层数组。
- 快照只隔离 Go 进程内的可变内存，不代表业务 payload Schema 已验证，也不提供跨进程/持久化层的不可变性；持久化仍须依靠内容寻址、事务和唯一约束。
- 单元测试覆盖源记录修改不影响快照、快照修改不影响源记录，以及无效记录不能生成已校验快照。

## QuantResultRef 元数据规范化边界

- `BuildRef` 对 security_id（若提供）、period、algorithm_version、parameter_fingerprint、authorization_ref 拒绝空白值及首尾空白，避免同一引用身份以不同文本形式通过 Go 构造器。
- 对应 QNT-CTR-004 Schema 同步要求上述字符串至少包含一个非空白字符。
- 此校验只约束文本表示，不解释 security_id、fingerprint 或 authorization_ref 的业务格式，也不证明授权有效。


## 授权读取 Application（QNT-CTR-005 / GOV-CTR-006）

新增 `internal/quant/application.GetResult`，作为 Researchflow/Server 的模块内调用入口：

- `PrincipalResolver` 只从 Server 可信认证上下文解析 `subject_ref`；GetResult 不接受调用方自报主体。
- `AuthorizedResultReader.ReadAuthorized` 同时承担当前策略复核与不可变记录读取。实现必须把当前策略快照读取、主体/资源/用途/级别判断、记录读取放在可证明的线性化/一致性边界；无法证明策略版本一致或发现撤销时拒绝并且不返回记录。
- 读取请求固定 `purpose_code=quant_analysis`、`content_level=full`，资源为调用方提供的精确 `result_id`。
- Application 校验 AuthorizationRef 与 PolicySnapshot 的 `policy_version/snapshot_ref`、Quant Owner、allowed、主体/资源/用途/级别、有效期、策略版本 tuple 顺序与唯一性，以及 GOV-CTR-006 JCS + SHA-256 指纹。
- 随后校验 Record 的 Manifest 与 Payload 指纹，按已注册的 `(result_type, payload_schema_version)` 严格解码，再构造完整 Envelope。未知类型、未知版本、篡改记录、授权证明无效均 fail closed。
- Application 只返回完整 Envelope，不返回裸 Payload 或 Ref；Repository 继续保持 Save-only。

### 生产装配尚未完成

- 当前分支提供的是可编译、可测试的 Port 与 Application 用例；没有生产 AuthorizedResultReader、当前策略存储、审计持久化或 MariaDB/Platform Storage Adapter。
- Server 必须注入可信 PrincipalResolver、UTC Clock 和生产 Reader；生产 Reader 需定义授权检查/读取的原子一致性边界。不能用历史 allowed、`expires_at=null` 或缓存结果代替当前策略检查。
- 本地单测只验证应用层授权证明、内容指纹和 fail-closed 行为；不证明真实存储并发撤销、审计落库或 TOCTOU 已验收。


### 读取审计 Port

- `application.GetResult` 强制依赖 `AccessAuditor`，审计事件只包含主体、结果引用、用途、决策、原因码、授权引用和发生时间，不包含 Payload 或受限内容。
- 已识别主体的读取尝试在结果释放前写入审计：显式拒绝记为 denied；授权/策略服务不可用和结果完整性错误记为 unknown；验证完成并准备释放记为 allowed。
- 审计写入失败时返回 `ErrAuditUnavailable`，不向调用方释放 Envelope。无法解析可信主体、无效 result_id 或调用已取消时不伪造主体审计记录。
- 当前仅完成 Port 和内存测试；生产审计持久化、保留策略和审计存储故障注入属于 Server/Platform Storage 集成验收。

### 授权证明严格解码

- AuthorizationRef 与 PolicySnapshot 拒绝未知 JSON 字段。
- AuthorizationRef 必须显式包含 `expires_at`；显式 `null` 表示没有固定过期时间，但仍须执行当前策略复核；字段缺失不等于 `null`，必须拒绝。
- PolicySnapshot 必须包含非空策略版本列表，按 `(policy_type, policy_ref, policy_version)` 严格字典序排列且无重复。
- 应用层读取验证已通过项目冻结的 GOV-CTR-006 Quant Golden，验证 JCS + SHA-256 指纹结果一致。
