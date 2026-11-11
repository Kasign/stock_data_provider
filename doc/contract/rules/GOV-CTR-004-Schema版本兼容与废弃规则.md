# GOV-CTR-004 Schema 版本、兼容、废弃与迁移规则

- 契约编号：GOV-CTR-004
- 规则版本：1.0.0
- 状态：freeze-ready（待项目负责人确认冻结）
- 适用范围：所有跨模块 DTO、Event Envelope/Payload、Result Manifest/Ref 和可独立演进的 Application Contract
- 权威性：本文件是 GOV-CTR-004 公共版本兼容语义的唯一权威来源。逐 DTO/Event 的版本、兼容矩阵和消费者支持范围由对应领域 CTR 登记。

## 1. Schema 版本身份

- 每个独立演进的 DTO、Event Envelope、Payload、Result Manifest、Ref 和 Application Contract 使用自己的 `schema_version`。
- Schema 版本与模块软件版本、算法版本、Template 版本、Event 业务版本相互独立。
- `schema_version` 只描述当前 DTO/Schema 自身的序列化结构和公开语义；嵌套且可独立演进的 Payload 必须使用自己的版本字段。
- 版本格式为 SemVer 字符串 `MAJOR.MINOR.PATCH`，不带 `v` 前缀。
- Quant Envelope、Manifest、Payload 版本身份按 DEC-PLAT-002-028 分离；Ref DTO 只描述 Ref 自身 Schema 版本。

## 2. 版本变化规则

| 版本级别 | 允许变化 | 禁止变化 |
|---|---|---|
| PATCH | 递增 PATCH 版本后修正文档、描述、注释、非规范性示例 | 不得改变 Schema 验证结果、字段含义、默认值或运行行为；不得原地修改已发布版本 |
| MINOR | 新增 optional 字段、新增独立能力 | 不得要求旧消费者理解新必需字段/枚举；不得改变既有字段解释 |
| MAJOR | 破坏性结构或语义变化 | 不得覆盖旧版本或伪装为兼容升级 |

以下变化属于 MAJOR：
- required/optional/nullable、JSON type、unit、precision、range、default 改变；
- 删除字段，改变既有字段含义或枚举含义；
- 改变数组顺序语义、状态迁移、授权边界、PIT 语义或错误语义；
- 收紧/放宽既有字段有效值集合，导致消费者接受行为发生变化。

新增 optional 字段只有在所有直接消费者可安全忽略，且不改变既有行为时，才可作为 MINOR。无法证明时按 MAJOR 处理。

## 3. 未知字段与枚举

- 每个 Schema 必须明确未知字段策略（忽略或拒绝）；安全敏感字段不得静默忽略。
- Enum 默认是 closed enum。未知枚举值不得映射为默认值、success 或其他已知值；消费者必须拒绝该 DTO/Event，或按该契约明确的 unsupported/degraded 安全路径处理。
- 只有 Schema 明确声明为 open enum 且定义未知值处理方式时，才允许兼容新增枚举值。
- 未知必需枚举或未知安全敏感字段必须 fail-safe；不得通过空值、默认值或旧语义继续处理。

## 4. 消费者版本支持

- 消费者只能解析其明确支持的 `schema_version`，或消费契约明确声明兼容的版本范围。
- 未知 MAJOR、不兼容版本、未知必需枚举或不允许的未知字段必须拒绝；不得回退到“最新可用”、近似版本或旧 Schema 猜测。
- 生产方和每个直接消费者必须维护支持版本/兼容范围；兼容性由明确矩阵和针对实际消费方向的 Golden/Contract Test 证明。
- JSON Schema 结构校验不等于消费者兼容性证明。

## 5. Schema 与 Golden 不可变

- 已发布的 `(schema_id, schema_version)` 定义不可原地改变。
- Event Registry 使用 `(event_type, schema_version)` 唯一定位 Schema；相同版本的规范 Schema、描述元数据与 Golden 必须保持不可变；即使只修正文档/示例，也必须发布新的 PATCH 版本。
- 语义/结构变化必须发布新版本并保留旧版本 Schema、Golden 和变更记录。
- 不得通过覆盖历史 Golden、静默重算历史数据或用当前 Schema 重新解释旧结果来消除兼容问题。
- 历史 Run/Event/Delivery/Result 必须保留创建时的 schema_version、algorithm_version、template_version 和输入快照引用；确需迁移时生成显式、可审计的迁移产物。

## 6. 废弃与移除

废弃字段、接口或 Schema 版本必须记录：
- `deprecated_since`；
- replacement（字段/接口/Schema 版本）；
- 迁移说明和直接消费者清单；
- `removal_after`（明确版本或可验证移除条件）。

不设全项目统一的“两次发布周期”最低等待期。兼容窗口按实际部署状态、消费者迁移成本和风险声明：
- 仍有活跃消费者时，删除前必须证明所有直接消费者已迁移并通过兼容 Contract Test；
- 初版能力尚无部署消费者、无外部兼容承诺时，可通过明确项目级决议直接退役旧定义/接口，不强制建设无意义兼容层；
- 直接退役仍须保留退役记录、替代关系和影响范围。DEC-PLAT-002-018 的旧 Analysis API 退役属于此类。

## 7. 字段级责任边界

本规则不维护逐 DTO/Event 的版本表、兼容矩阵、消费者支持范围或迁移状态。每项契约由生产方/消费者明确：
- schema_id、schema_version 和独立嵌套 Payload 版本；
- unknown field/enum 策略；
- MINOR 新增字段是否可安全忽略；
- 每个直接消费者支持的版本/范围；
- 废弃、替代、removal_after 和兼容窗口；
- Golden/Contract Test 的兼容方向。

这些证据由 DATA、NEWS、QNT、RSC、FLOW、LLM、NTF、PLAT、SRV 等对应 CTR 验收，不反向阻断 GOV-CTR-004 公共规则本身冻结。
