# GOV-CTR-006：AuthorizationRef 机器契约与 Golden

- 决议日期：2026-11-06
- 适用范围：Data、News、Quant、Research 的授权快照签发与跨模块引用
- 当前状态：语义决议 accepted；Schema 定义通过 Draft 2020-12 结构校验，四个 Golden 的 JCS/SHA-256 复算一致；模块 CI Validator、Scope/资源映射和授权竞态 Contract Test 仍 pending_evidence
- 权威关系：公共字段与语义以项目级契约目录第 7.2 节为准；本文给出机器 Schema、Principal/Resource 映射、fingerprint 规范和 Golden 测试向量。

## 1. AuthorizationRef Schema

机器文件：`schema/GOV-CTR-006-authorization-ref-1.0.0.schema.json`。

字段固定为：
- `authorization_ref`：不透明、稳定、不可猜测的审计引用；不是 bearer token。
- `policy_owner`：data/news/quant/research。
- `policy_version`：AuthorizationPolicySnapshot.snapshot_ref，不是单个策略版本号。
- `decision`：allowed/denied/conditional/unknown。
- `purpose_code`：quant_analysis/research_analysis/risk_review/user_display。
- `content_level`：metadata/summary/snippet/full。
- `subject_ref`、`resource_ref`：可信主体与本次授权目标资源的不透明引用。
- `decided_at`、`expires_at`：RFC3339Nano UTC；expires_at 必须存在，可为 null。
- `conditions`：稳定 condition_code 数组；去重并按字典序排序。
- `decision_fingerprint`：小写 64 位 SHA-256 十六进制。

## 2. AuthorizationPolicySnapshot

机器文件：`schema/GOV-CTR-006-policy-snapshot-1.0.0.schema.json`。

`policy_version` 必须指向签发 Owner 保存的不可变 AuthorizationPolicySnapshot。结构为：
- `snapshot_ref`：与 AuthorizationRef.policy_version 完全相同。
- `policies[]`：至少一项，每项包含 `policy_type`、`policy_ref`、`policy_version`。

即使一次授权只评估一个策略，也必须形成一项快照；不得让 policy_version 在单策略和多策略场景下具有两种语义。多策略时必须完整记录 Provider Output Policy、Source Policy、Endpoint Policy 等所有实际生效版本。policies 按 (policy_type, policy_ref, policy_version) 字典序排列且不得重复。

## 3. Principal / Resource / Scope 映射

| policy_owner | subject_ref 来源 | resource_ref | purpose_code | content_level |
|---|---|---|---|---|
| data | Data API Key 认证后映射的可信 caller principal；禁止客户端自报 | Data 不可变查询 snapshot_ref | quant_analysis / research_analysis / risk_review | full |
| news | 可信 caller_ref（Research/Researchflow/授权用户主体） | 实际 News Evidence/版本资源引用；Provider/Source/Endpoint 策略版本进入组合快照 | research_analysis / risk_review / user_display | metadata / summary / snippet / full |
| quant | 可信调用主体；MVP 为 Researchflow service principal | Quant result_id | quant_analysis | full |
| research | 可信调用主体；MVP 为 Researchflow service principal | Research evidence_id | research_analysis / risk_review | 按实际返回内容选择 |

所有 subject_ref 必须来自认证/服务调用上下文，禁止接受请求体、Header 或 Query 中未经认证的主体声明。resource_ref 必须唯一指向本次授权实际覆盖的资源；多资源结果应逐资源签发，或引用一个不可变快照且该快照完整列出覆盖范围。Research 为派生 Evidence 签发自己的快照，但不能覆盖或替代 Data/News/Quant 来源授权链。

## 4. Decision 与条件语义

- allowed：签发时允许；conditions 必须为空数组。每次实际读取仍重新检查当前策略。
- denied：明确拒绝；不得访问受限内容。
- unknown：无法可靠判断；默认拒绝受限内容。
- conditional：不是授权许可；conditions 必须非空。签发 Owner 验证所有条件后，必须重新签发新的 allowed AuthorizationRef，不得把原 conditional 引用当作允许凭证。
- expires_at=null 只表示没有固定过期时间，不代表永久授权；当前策略撤销、Scope 变化或授权服务不可用仍必须拒绝。
- expires_at 非 null 时，now >= expires_at 即过期。读取时策略版本在检查和内容读取之间发生变化，必须重新授权；无法确认一致性时丢弃内容并拒绝返回。

## 5. Fingerprint 唯一算法

1. 从 AuthorizationRef 取除 `authorization_ref` 和 `decision_fingerprint` 之外的全部字段。
2. 解析 `policy_version` 指向的 AuthorizationPolicySnapshot，取完整 `policies` 数组作为 `evaluated_policy_versions`；数组按 (policy_type, policy_ref, policy_version) 字典序排序。
3. 将 `evaluated_policy_versions` 加入步骤 1 的对象，形成唯一 fingerprint input。
4. 对 fingerprint input 执行 RFC 8785 JCS，得到 UTF-8 字节。
5. 计算 SHA-256，使用小写 64 位十六进制写入 decision_fingerprint。

不得仅对 AuthorizationRef 的 policy_version 字符串计算指纹；不得漏掉组合快照中的任何实际生效策略版本。不得将 Secret、原始凭证或受限内容正文放入 fingerprint input。该指纹用于审计一致性，不是签名或访问令牌。

## 6. Golden

四个签发方测试向量：
- `golden/GOV-CTR-006-data-allowed.golden.json`
- `golden/GOV-CTR-006-news-conditional.golden.json`
- `golden/GOV-CTR-006-quant-allowed.golden.json`
- `golden/GOV-CTR-006-research-allowed.golden.json`

每个 Golden 同时包含 AuthorizationRef、策略组合快照、fingerprint_input、JCS canonical_json 和预期 SHA-256。测试必须复算摘要并与 expected_sha256 完全相等；不得只比较文档中的预期字符串。

## 7.1 Contract Test 执行矩阵

测试按生产方分别运行；每个 Owner 对自己的签发逻辑负责，消费者不得复制 Owner 的策略判断。测试必须同时验证返回内容/副作用，不允许只断言 decision 字段。

| Test ID | Owner / 层 | 场景与输入 | 必须断言 | 失败结果 |
|---|---|---|---|---|
| GOV006-SCH-01 | 公共 Schema | 四份 Owner Golden + 两份 Schema | AuthorizationRef 与 PolicySnapshot 均通过 Draft 2020-12；启用 format assertion 校验 RFC3339 date-time | CI 失败 |
| GOV006-SCH-02 | 公共 Schema | 缺少必填字段、未知字段、非法 enum、空 ref、非法 fingerprint、allowed 携带 conditions、conditional 不带 conditions | 每个负例均被拒绝；不得静默丢弃未知字段 | CI 失败 |
| GOV006-SCH-03 | 语义 Validator | policy_version 与 snapshot_ref 不一致；policies 为空；重复 tuple；顺序非字典序 | 一致性、非空、tuple 唯一和稳定排序全部由跨对象 Validator 检查 | CI 失败 |
| GOV006-FP-01 | 公共 Fingerprint | 四个 Golden | 从 AuthorizationRef + PolicySnapshot 重新构造 fingerprint_input；JCS 后 SHA-256 必须等于 expected_sha256 和 decision_fingerprint | CI 失败 |
| GOV006-FP-02 | 公共 Fingerprint | 修改 policies 中任意一项 policy_type/ref/version，或漏掉一项 | 摘要必须变化；旧 fingerprint 不再匹配 | CI 失败 |
| GOV006-FP-03 | 公共 Fingerprint | 仅修改 authorization_ref 或 decision_fingerprint | fingerprint_input 不变；避免把引用自身或摘要自身递归纳入摘要 | CI 失败 |
| GOV006-DATA-01 | Data Owner | 有效 API Key、可信 principal、指定 snapshot_ref、purpose=research_analysis | subject_ref 由认证上下文派生；resource_ref 对应不可变 snapshot；记录全部 Data 策略版本 | 不允许从请求体覆盖主体或资源 |
| GOV006-DATA-02 | Data Owner | 无效/缺失/撤销 API Key，或 snapshot 不存在/不属于当前 principal | 不签发 allowed；不得返回受限数据 | denied/unknown + 无数据正文 |
| GOV006-NEWS-01 | News Owner | Provider Output + Source + Endpoint 三类策略均参与 | PolicySnapshot 完整包含三类实际生效版本；resource_ref 指向精确 Evidence 版本 | 缺一策略即测试失败 |
| GOV006-NEWS-02 | News Owner | source 条款要求 attribution，条件尚未满足 | 返回 conditional 和稳定 condition_code；不得把正文交给调用方 | 仅返回允许的 metadata，不返回受限正文 |
| GOV006-NEWS-03 | News Owner | attribution 条件满足后重新请求 | 签发新的 allowed AuthorizationRef；新引用有新 fingerprint；不得原地修改旧引用 | 旧 conditional 仍不可用于读取 |
| GOV006-QUANT-01 | Quant Owner | Researchflow service principal 请求 quant result | subject_ref 由可信服务身份派生；resource_ref 精确对应 result_id；purpose 固定 quant_analysis | 客户端伪造主体或资源时拒绝 |
| GOV006-QUANT-02 | Quant Owner | result_id 不存在、越权或策略拒绝 | 不签发 allowed，不返回结果内容 | denied/unknown + 无结果正文 |
| GOV006-RES-01 | Research Owner | 读取 Research Evidence 并形成派生 Evidence | Research 签发自己的 AuthorizationRef；保留每条上游 Data/News/Quant 来源授权链 | 不得以 Research allowed 覆盖上游 denied/expired/revoked |
| GOV006-RES-02 | Research Owner | 派生 Evidence 混合多个来源，其中一个来源当前撤销 | 重新计算可披露内容范围；不可安全剥离时拒绝整个派生内容 | 不返回受影响正文 |
| GOV006-READ-01 | 所有消费者 | 历史 allowed，但当前 Owner 策略已撤销或 Scope 收窄 | 每次读取均由 Owner 按当前策略复核；历史快照只用于审计 | 当前 denied/unknown 时拒绝 |
| GOV006-READ-02 | 所有消费者 | expires_at=null | 不因 null 自动通过；仍执行当前策略与 Scope 复核 | 按当前决策放行或拒绝 |
| GOV006-READ-03 | 所有消费者 | now == expires_at | 视为过期；边界使用大于等于 | 拒绝 |
| GOV006-READ-04 | 所有消费者 | 授权服务超时、不可用、响应不完整 | 受限资源默认拒绝；不回退使用缓存的历史 allowed | unknown/错误 + 无正文 |
| GOV006-RACE-01 | Owner + Resource Store | 授权检查后、正文读取前策略版本被撤销或替换 | 读取时验证快照/策略版本仍有效；无法证明一致性则丢弃正文并拒绝 | 不得发生 TOCTOU 放行 |
| GOV006-RACE-02 | Owner + Resource Store | 并发撤销与读取 | 明确线性化点；撤销生效后启动的读取不得使用旧授权；在途读取按 Owner 定义的原子边界处理 | 不得返回撤销后新读取的正文 |

### 7.2 Validator 的职责边界

JSON Schema 负责单对象结构约束；跨对象和运行时语义不得假装由 Schema 完成：

1. **Schema Validator**：Draft 2020-12；必须启用 format assertion，而不是只把 date-time 当注释。
2. **Snapshot Validator**：校验 authorization_ref.policy_version == policy_snapshot.snapshot_ref；policies 非空；按 (policy_type, policy_ref, policy_version) tuple 唯一且严格字典序排序。
3. **Fingerprint Verifier**：根据 PolicySnapshot 重建 evaluated_policy_versions，再执行 RFC 8785 JCS + SHA-256；不信任调用方传入的 fingerprint_input/canonical_json。
4. **Owner Authorization Test**：每个 Owner 自己验证 subject_ref、resource_ref、purpose_code、content_level 与可信认证上下文和真实资源的绑定关系。
5. **Read-time Gate**：消费者必须调用资源 Owner 的当前授权检查；不得仅在任务创建时检查一次后长期缓存 allowed。
6. **Race Test**：授权检查与正文读取必须处于可证明一致的策略版本边界；无法证明时 fail closed。

### 7.3 测试分层与完成定义

- **L1 Schema/Golden**：GOV006-SCH-*、GOV006-FP-* 全通过。
- **L2 Owner Contract**：DATA/NEWS/QUANT/RESEARCH 各自的签发、Scope、资源映射、当前授权复核全通过。
- **L2 Consumer Contract**：Researchflow/Report/Notification 等直接消费者不得把 AuthorizationRef 当 bearer token；必须保留上游来源链并调用 Owner 当前复核。
- **L2 Concurrency/Security**：READ/RACE 场景全通过。
- 只有 L1 + 所有适用 L2 都有 CI 运行记录、测试命令、测试结果和提交 SHA，GOV-CTR-006 才能从 pending_evidence 升级为 frozen。文档存在、Schema 可解析或 Golden 静态校验通过都不等价于 Contract Test 通过。

## 7. 尚待证据

以下测试通过前，GOV-CTR-006 保持 pending_evidence：
- 将仓库中的两份 Schema 文件接入模块 CI，由实际 Validator 执行正向/反向校验，并校验 format/date-time；本轮已对 Schema 定义执行 Draft 2020-12 结构校验。
- 将四个 Golden 接入运行时测试，复算 JCS canonicalization 与 SHA-256；本轮四个 Golden 预期摘要独立复算一致，尚未形成模块 CI 证据。
- policy_version 与 snapshot_ref 一致性、policies 非空/唯一/排序、遗漏任一策略版本导致 fingerprint 不匹配。
- 四个 policy_owner 的 Principal/Scope/Resource 映射 Contract Test。
- allowed → revoked、expires_at 边界、conditional 未满足/全部满足、unknown、授权服务不可用的拒绝测试。
- 策略在授权检查与内容读取之间变化时，必须重新授权或丢弃内容的竞态测试。
- 历史 allowed 不得覆盖当前 denied/expired/unknown；来源授权链不得被 Research 派生授权覆盖。

以上证据只阻断依赖授权引用或受限资源访问的调用点；不阻断无授权依赖的 Domain 开发。
