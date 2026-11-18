# GOV-CTR-002 公共时间与 PIT 规则

- 契约编号：GOV-CTR-002
- 规则版本：1.1.0
- 状态：`frozen`
- 适用范围：所有跨模块时间字段、历史事实查询、研究 Run、事件和结果 DTO
- 权威性：本文件是 GOV-CTR-002 公共时间与 PIT 语义的唯一权威来源。各领域 DTO/Schema/Registry 负责字段级适用映射，不得复制或重新解释本规则。

## 1. 时间表示

- 跨模块绝对时间统一使用 RFC3339Nano UTC，并以 `Z` 结尾。
- 必须保留来源可证明的时间精度；不得补造小数位，不得将未知时区假设为 UTC 或 `Asia/Shanghai`。
- 来源时间带有明确 UTC offset 时，可转换为 UTC 表示；来源时区/offset 不明或精度不可验证时，保留原始来源证据，规范化值置 null/unknown。
- 交易日/业务日期使用 `YYYY-MM-DD`，按对应市场的权威交易日历解释；A 股交易日使用 `Asia/Shanghai` 交易日历。
- 日期不是 UTC 午夜时间戳，不得与绝对时间隐式互转。

## 1.1 输入解析与规范化

- 本规则中的“跨模块绝对时间统一使用 RFC3339Nano UTC、以 `Z` 结尾”，约束的是跨模块公开 DTO/Event 等规范化后的时间表示，不要求外部来源或用户输入的原始时间文本必须原本以 `Z` 结尾。
- 在外部输入边界（包括用户明确提供的时间、外部 Provider 原始时间）可接受 RFC 3339 时间戳中的 `Z` 或明确的数字 UTC offset（例如 `+08:00`、`-05:00`）；必须存在明确时区信息。不得接受无时区时间后再猜测其时区。
- 输入解析后立即转换为 UTC Instant；在跨模块 DTO/Event 输出前，统一序列化为 RFC3339Nano UTC `Z`。原始来源文本及来源时区/offset 证据按领域需要保留，不得因规范化而丢失可追溯证据。
- 外部输入契约可以进一步收紧格式，但必须在对应输入契约中明确；不得把跨模块输出的 canonical `Z` 要求误读为所有入口原始文本的统一限制。跨模块 DTO 若直接作为输入/输出契约，仍遵守其已发布 Schema 的明确验证范围。
- Platform `time.Service` 负责当前时刻读取、UTC 规范化和 IANA 时区转换，不是 RFC3339 原始字符串解析器，也不负责推断来源时间。Provider Adapter/入口 Adapter 负责解析外部时间并规范化。
- 当前时刻来源按用例选择：普通进程内当前时间使用 Platform SystemClock；Run 创建、Deadline、Lease 等跨实例协调使用数据库权威 UTC 时钟。两者都通过 Platform 时间抽象接入；不得用当前时钟替代用户指定的历史 PIT 时点或 Provider 来源时间。

## 2. 公共时间字段语义

| 字段 | 语义 | 不得替代 |
|---|---|---|
| `occurred_at` | 现实事件实际发生时间 | 发布时间、抓取时间、系统观察时间 |
| `published_at` | 来源声明的发布时间 | 外部实际可获得时间、系统观察时间 |
| `available_at` | 有可验证证据证明内容对外可获得的时间 | published_at、fetched_at、system_observed_at |
| `system_observed_at` | 系统首次观察某个具体不可变来源版本的时间 | 主体聚合 first/last observed 时间、抓取开始时间 |
| `system_as_of` | 系统知识截止时间；限定某次计算/研究允许使用的信息边界 | 交易日、当前时间、发布时间、执行截止 |
| `valid_time_start/end` | 业务事实在现实世界中的有效区间 | 系统知识时间、发布时间 |
| `fetched_at` | Adapter/系统从来源抓取或读取的处理时间 | available_at、system_observed_at |
| `normalized_at` | 系统完成标准化处理的时间 | source time、system_as_of |
| `created_at/updated_at` | 记录创建/更新的审计时间 | 业务有效时间或知识截止 |
| `deadline_at` | 某次执行允许继续运行的截止时间 | system_as_of |
| `trade_date` | 按市场交易日历解释的日期 | UTC 时间戳 |

`provider_time` 属于来源特定时间，必须由生产方显式映射到准确的公共时间语义后才能跨模块暴露；不得将 Provider 字段原样命名为公共时间语义。

## 3. system_as_of 与 Run 传播

- `system_as_of` 是系统知识截止，不等于当前时间、交易日、发布时间、抓取时间或执行截止时间。
- Researchflow 在 CreateRun 时确定并持久化唯一 `system_as_of`；所有下游请求、查询、Stage、Attempt、Evidence/Context、结果和重试必须继承同一值。
- Stage/Attempt 重试不得刷新或放宽 `system_as_of`。下游返回的事实/结果不得突破所属 Run 的知识截止边界。
- 旧字段 `as_of` 不得仅凭名称自动映射为 `system_as_of`；Adapter 必须确认语义完全等价后显式映射，否则拒绝或保留原语义。
- `deadline_at` 只约束执行生命周期，与知识截止分离；不得用 deadline 替代 system_as_of。

## 4. available_at 与严格 PIT

- `available_at` 缺少可验证来源证据时必须为 null/unknown；不得从 `published_at`、`fetched_at`、`system_observed_at` 或 Provider 当前时间推导。
- 严格 PIT 的公开事实必须同时满足：
  1. `system_observed_at <= system_as_of`；
  2. `available_at` 非 null，且 `available_at <= system_as_of`。
- 无法证明任一边界时，不得作为该截止时间下已知且公开可得的事实参与严格 PIT；不得自动降级为宽松 PIT。
- 若某业务场景需要非严格 PIT，必须由对应版本化 Template/契约显式声明模式及风险，不得由 Adapter 或消费者自行放宽。
- `published_at` 只保留来源声明，不证明外部实际可获得；即使 `published_at <= system_as_of`，也不能代替 available_at。

## 5. 业务有效时间与系统知识时间

- `valid_time_start/end` 表达事实在业务世界中的有效区间，与 system_as_of/system_observed_at 是不同时间轴。
- valid_time 两端必须同时为 null，或同时提供且满足 start <= end；字段是否 nullable/required 由领域 Schema 明确。
- 历史查询必须分别按业务有效时间和系统知识时间筛选，不得把“事实当时有效”解释为“系统当时已知”。
- ResearchEvent 等领域可另有来源时间选择/规范化规则版本；该版本解释来源时间如何选取，不替代本规则的 UTC 表示、PIT 边界或 DTO schema_version。

## 6. 字段级责任边界

本规则不定义每个领域字段的 Provider 来源优先级、时间冲突处理、时区识别算法、交易日历版本选择或字段 required/nullable。各生产方必须在 DTO/Schema/Registry 中明确：
- 时间字段具体含义及其映射到本规则哪一种公共时间语义；
- 来源字段、可信时区/offset、精度和缺失证据；
- 是否允许 null、是否必需，以及 PIT 使用条件；
- 交易日字段使用的市场和日历版本；
- 对历史数据无法证明时间边界时的 fail/degrade/unknown 行为。

例如 News 的单次版本观察时间应映射为 `system_observed_at`；主体级 first/last observed 聚合时间不能替代具体版本观察时间。此类模块字段映射由 NEWS-CTR-001/003 等契约负责，不在公共规则重复定义。

## 7. Golden 最小覆盖要求

各时间字段生产方的 Golden/Contract Test 按适用情况覆盖：
- UTC `Z`、明确非零 offset 转 UTC、秒级和纳秒级精度保留；
- 未知时区、缺少来源证据、规范化值为 null；
- available_at 缺失、晚于 cutoff、等于 cutoff、早于 cutoff；
- system_observed_at 晚于 cutoff；
- valid_time_start/end 成对、反向区间和 null；
- system_as_of 在 Run 创建、Stage、Attempt、重试和结果间保持不变；
- trade_date 按对应市场交易日历解释，不与 UTC 午夜互换。

不适用的情况由字段 Schema 明确排除，不要求所有字段机械覆盖所有类别。
