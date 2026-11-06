# GOV-CTR-006：AuthorizationRef 机器契约与 Golden

- 决议日期：2026-11-06
- 适用范围：Data、News、Quant、Research 的授权快照签发与跨模块引用
- 当前状态：语义决议 accepted；Schema/Golden 已提交，运行时 Validator 与 Contract Test 仍 pending_evidence
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

## 7. 尚待证据

以下测试通过前，GOV-CTR-006 保持 pending_evidence：
- Draft 2020-12 Validator 对 AuthorizationRef 与 PolicySnapshot Schema 的正向/反向校验。
- 四个 Golden 的 JCS canonicalization 与 SHA-256 运行时复算。
- policy_version 与 snapshot_ref 一致性、policies 非空/唯一/排序、遗漏任一策略版本导致 fingerprint 不匹配。
- 四个 policy_owner 的 Principal/Scope/Resource 映射 Contract Test。
- allowed → revoked、expires_at 边界、conditional 未满足/全部满足、unknown、授权服务不可用的拒绝测试。
- 策略在授权检查与内容读取之间变化时，必须重新授权或丢弃内容的竞态测试。
- 历史 allowed 不得覆盖当前 denied/expired/unknown；来源授权链不得被 Research 派生授权覆盖。

以上证据只阻断依赖授权引用或受限资源访问的调用点；不阻断无授权依赖的 Domain 开发。
