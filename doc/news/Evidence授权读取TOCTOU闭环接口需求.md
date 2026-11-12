# Evidence 授权读取 TOCTOU 闭环接口需求

- 日期：2026-11-10
- 适用范围：News `GetEvidence` / `ListEvidence` 内容披露路径
- 当前状态：需求澄清完成；跨模块一致性机制尚未实现，NEWS-CTR-002 / GOV-CTR-006 相关读取竞态仍为 pending_evidence
- 代码基线：`b4767ab60dfcba187e4cb9a211ac910d8763f6a7`

## 1. 当前实现事实

当前 `GetEvidence.Execute` 的顺序为：

1. 调用 `EvidencePITRepository.GetEvidenceVersionAt`，读取 Observation 和完整 `NewsItemVersion`（包括标题、ContentLayers）。
2. 读取精确 caller/resource/purpose/content-level 对应的最新 Authorization Binding。
3. 校验 Binding 与 Observation/Version 绑定关系。
4. 调用 `EvidenceReadAuthorizer.Execute`，验证历史 fingerprint 并读取当前策略集合。
5. 授权通过后构造并返回 DTO。

这能确保未授权 DTO 不返回给跨模块调用方，但不能证明数据库内容读取与当前策略复核处于同一原子边界。授权检查前正文已经进入 News 进程内存；策略在检查与内容读取之间变化的竞态也没有被消除。ListEvidence 逐项复用 GetEvidence，因此继承相同限制。

**禁止将现有顺序描述为已消除 TOCTOU，禁止将“授权失败时返回空 DTO”当作一致性证明。** 当前生产披露必须保持关闭，直到下述边界有实现和并发测试证据。

## 2. 必须保证的语义

任何受限内容读取必须同时绑定：

- AuthorizationRef：`authorization_ref`、`decision_fingerprint`、`policy_version`。
- 主体与请求：`subject_ref/caller_ref`、`purpose_code`、`content_level`。
- 资源版本：`resource_ref`、`news_id`、`version_id`、`content_fingerprint`。
- 当前策略状态：完整 PolicySnapshot 引用及其实际生效的 Provider Output、Source、Endpoint 策略版本。
- 读取边界：一个明确、可审计的线性化点；不能证明检查和读取属于同一有效授权窗口时，必须丢弃内容并拒绝返回。

拒绝条件包括：当前策略 denied/unknown、策略服务不可用、Binding 或 fingerprint 不匹配、资源版本变化、授权过期、策略撤销已生效、许可内容级别不足，以及一致性机制无法确认。

## 3. 可接受的一致性实现模式

跨模块 Owner 必须选择并证明以下一种模式，不允许仅增加两次普通查询或“读前/读后各校验一次”作为修复。

### 模式 A：共享事务与策略版本锁

仅当授权策略与 News 内容读取确实共享同一事务资源时可用：

1. 开启同一数据库事务。
2. 锁定/读取当前完整策略快照及撤销状态，获得不可变版本引用。
3. 在该事务中校验 Binding、资源版本和请求 Scope。
4. 在同一事务内读取目标内容版本。
5. 提交前保持策略版本锁定；撤销操作必须与该锁冲突并按明确顺序线性化。

验收必须证明：读取先取得锁时，撤销不能先于该读取的线性化点生效；撤销先提交时，后续读取不能继续使用旧策略。若策略存储和 News DB 不共享事务，此模式不成立。

### 模式 B：Owner 管理的读取许可/租约

适用于策略 Owner 与 News 内容库不共享事务的情况。许可必须由策略 Owner 管理，至少绑定：

- caller、purpose、content level；
- 精确 resource/version/content fingerprint；
- AuthorizationRef fingerprint 和完整 PolicySnapshot 引用；
- permit 唯一引用、签发时间、到期时间及撤销/失效语义。

许可机制必须提供可证明的并发撤销语义：撤销与许可获取具有原子顺序；已生效撤销不能被之后启动的读取绕过。若允许在途许可完成，必须明确许可持有期、最长有效期和撤销生效点，并保证许可到期后不得继续返回内容。News 必须在许可有效窗口内完成精确版本读取；不能只在读取前获得一个可长期缓存的布尔值。

### 不可接受的伪修复

- 先读取完整正文，再调用当前策略 Resolver。
- 先调用 Resolver，随后无版本绑定地读取“当前最新正文”。
- Resolver 返回普通 bool/decision，但没有策略版本、资源版本和并发撤销语义。
- 读取前后各查一次策略，却没有共享事务、锁、租约或其他线性化机制。
- 将历史 allowed AuthorizationRef 或缓存的策略结果视为当前许可。
- 因为 DTO 最终未返回，就声称敏感正文没有被读取。

## 4. News 侧最小改造边界

一致性机制确定前，News 不自行实现策略锁、跨服务租约或公共 JCS：

- 保留 `EvidenceReadAuthorizer` 的 fail-closed 行为；注入缺失时不构造可披露 Facade。
- 将完整内容读取与只读元数据/版本定位区分开，避免授权前不必要地加载正文；但该拆分本身不构成 TOCTOU 修复。
- 最终内容读取必须接受 Owner 提供的、绑定精确资源版本的有效读取许可/事务上下文。
- `GetEvidence` 与 `ListEvidence` 必须复用同一受控读取路径；列表任一项许可失败时整页失败，不返回部分内容。
- 许可无效、版本不一致、撤销竞态或依赖不可用时，返回稳定错误且不返回内容 DTO。

## 5. 跨模块责任与待确认项

| 责任方 | 必须提供/确认 |
|---|---|
| Platform / GOV Owner | GOV-CTR-006 Runtime Verifier 的唯一实现；PolicySnapshot 完整性与版本引用验证；明确公共读取许可原语是否存在，若不存在则明确不由 Platform 代替领域 Owner 决策 |
| News Owner | 精确 Evidence 资源/版本绑定；授权前元数据定位与最终内容读取分离；接入受控读取边界；失败时不返回正文 |
| Server 组合根 | 注入真实策略 Owner、Verifier 和读取许可协作者；不以 mock/fallback 作为生产实现 |
| 策略 Owner（Source/Endpoint/Provider 策略权威方） | 提供当前策略读取、撤销线性化语义及模式 A 或 B 的可验证机制 |
| 集成验收 Owner | 组织并发撤销/读取 Contract Test，记录测试命令、结果和对应 SHA |

需要跨模块共同回答的决策：

1. 当前策略权威存储与 News 内容库是否共享同一个 MariaDB 事务资源？
2. 若不共享，策略 Owner 是否能提供模式 B 的许可/租约？撤销对已签发许可的生效语义是什么？
3. 读取的线性化点是什么，撤销操作的线性化点是什么？两者如何排序？
4. 在途读取是否允许在撤销后完成？如果允许，最大许可时长和可披露边界是什么？

这些问题没有答案前，不在 News 分支猜测实现接口或承诺实时撤销。

## 6. 必须补齐的竞态验收

| 用例 | 并发动作 | 必须断言 |
|---|---|---|
| RACE-01 | 读取取得许可前，策略撤销并提交 | 读取拒绝；不读取/不返回受限正文 |
| RACE-02 | 读取取得许可后，策略尝试撤销 | 按已冻结线性化规则串行化；不得出现规则之外的放行 |
| RACE-03 | 策略版本在授权与内容读取之间更新 | 旧版本不得用于新读取；重新授权或拒绝 |
| RACE-04 | Evidence Version 在元数据定位后发生变化 | 只能读取 Binding 明确绑定的不可变版本；不得回退/升级到当前版本 |
| RACE-05 | 许可到期与读取并发 | 到期边界按 `now >= expires_at` 拒绝；不返回部分正文 |
| RACE-06 | Verifier/策略 Owner/许可服务超时或返回不完整 | fail closed；错误可诊断；正文不返回 |
| RACE-07 | ListEvidence 多条记录中一条许可失效 | 整页失败，不返回已成功读取的部分正文 |

每个测试必须断言实际返回内容和读取副作用，不能只检查 decision 字段。MariaDB 事务模式必须使用真实 MariaDB 并发测试；租约模式必须包含策略 Owner 的并发/撤销测试。

## 7. 当前结论

- News 当前实现属于授权预检，不是原子授权读取。
- 本文只冻结 News 对跨模块机制的需求，不自行冻结模式 A/B；选择权归策略 Owner 与 Platform/Server 集成决策。
- 该项保持 P1 阻断：未完成一致性实现及 RACE-01～07 适用测试前，不得启用生产受限内容披露。


## 8. 一致性模式决策（2026-11-12）

### 8.1 选择模式 A：News MariaDB 共享事务与策略版本行锁

本模块选择模式 A，不采用跨服务许可租约。依据：

1. GOV-CTR-006 与 NEWS-CTR-002 将 Provider Output、Source、Endpoint 策略组合决策责任归 News；Research/Researchflow 只消费授权结果，不拥有这些来源策略。
2. News Evidence 内容、Observation、Version 已由 News Repository 写入 MariaDB；Server 组合根向 News Repository 注入共享 `*sql.DB`。策略权威数据应归入同一 News 数据库事务资源，不引入第二套策略服务。
3. 当前 Static Catalog 只是启动期输入快照，不能继续作为读取时权威状态。News 必须将不可变策略版本和当前版本指针持久化，并让策略发布/撤销与 Evidence 读取竞争同一数据库行锁。

该决策不代表模式 A 已实现或当前内容读取已安全开放。

### 8.2 目标数据结构

新增两类 News 自有表：

- `news_policy_versions`：不可变策略版本，复合主键为 `(policy_type, policy_ref, policy_version)`；保存规范化策略 JSON、payload hash、创建时间。允许的 policy_type 仅为 `source`、`endpoint`、`provider_output`。
- `news_current_policy_versions`：每个 `(policy_type, policy_ref)` 唯一一行，指向当前不可变版本；复合外键保证目标版本存在。策略撤销也必须发布新的 revoked 版本并切换指针，不允许原地修改历史版本。

策略发布事务必须先写入不可变版本，再更新当前指针；两个动作在同一事务提交。旧版本不得 UPDATE/DELETE。

### 8.3 读取事务与线性化点

最终正文读取必须在一个 MariaDB 事务中完成：

1. 根据 Binding 的完整 PolicySnapshot 构造所有适用策略 tuple，并按 `(policy_type, policy_ref)` 字典序排序。
2. 对每个 `news_current_policy_versions` 当前指针行执行 `SELECT ... FOR UPDATE`；任何行缺失、版本不匹配、payload hash 错误或策略集合不完整均拒绝。
3. 在该事务中读取对应不可变策略版本，校验状态、有效期、用途和内容级别；Source、Endpoint、Provider Output 的实际适用集合必须与 Binding 完全一致。
4. 在同一事务中按 `news_id + version_id` 读取正文，并复核版本 fingerprint、所属 Observation 和 `system_as_of`。
5. 成功提交事务后才允许将正文交给 Application 组装 DTO；事务失败或提交失败必须丢弃正文。

读取线性化点为持有完整策略行锁的读取事务提交。撤销/收窄事务更新同一当前指针行：读取先取得锁时，撤销只能在读取提交后生效；撤销先提交时，后续读取只能看到新版本。两者由 InnoDB 行锁排序，不依赖进程内锁或 advisory lock。

### 8.4 代码实施顺序

1. V9 Migration：不可变策略版本表、当前版本指针表及复合外键/索引。
2. Domain：按策略类型校验 payload 与 ref/version 绑定，定义完整策略组合的只读快照。
3. Storage：策略发布事务和读取事务的共享锁协议；所有锁按固定 tuple 顺序取得。
4. Application：将 GOV-CTR-006 Fingerprint Verifier 与授权前元数据定位接入；最终内容读取只能调用原子授权读取 Port，不再在普通 Resolver 预检通过后直接读取正文。
5. MariaDB 并发集成：撤销先提交、读取先锁定、策略版本替换、缺失策略行、payload 篡改、资源版本不匹配和提交失败。
6. RACE-01～07 对照 NEWS-CTR-002 逐项记录实际读取副作用和返回内容。

JCS 仍只调用项目公共 GOV-CTR-006 Runtime Verifier；本模式不复制 JCS 算法。公共 Verifier 未可用时，授权读取仍 fail closed。

### 8.5 实施边界

- 本决策只在 News 自有 MariaDB 与 News 内容库共享同一个 `*sql.DB` 时成立；Server 不得为 News 策略另建连接池。
- Static Catalog 只可作为启动导入/配置校验输入，不再作为生产读取时当前策略权威来源。
- 不新增 News HTTP 管理 API；策略发布/撤销先作为 News 内部 Application Port，由 Server 组合根或受控运维入口注入。
- 本决策不放宽任何来源授权，不自动启用当前 disabled/skipped 的来源，不授权真实网络采集。
- 在 V9、策略发布/撤销事务、原子正文读取、公共 Verifier 注入及 RACE 测试全部通过前，受限内容披露保持关闭。
