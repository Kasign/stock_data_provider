# PLAT-CTR-004 Diagnostics Observation 与 Runner 契约

- 契约编号：PLAT-CTR-004
- 契约版本：1.0.0
- 状态：freeze-ready（待项目负责人确认冻结）
- 代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 生产方：Platform Diagnostics Runner；各领域提供 Check 与领域 Observation 内容
- 直接消费者：Root Server Diagnostics 聚合器、News/Research 等领域 Check Adapter
- 权威性：本文件定义公共 Observation DTO、进程内 Check/Runner/Report 语义及聚合规则；完整 Diagnostics HTTP 响应由 Server 契约定义。

## 1. 责任边界

- Platform 定义 Observation 公共结构、Check 执行接口、有限并发/超时和基础状态聚合。
- 领域模块拥有 Check 的业务规则、reason_code 词汇、safe_message、attributes 白名单、evidence_ref 的含义/授权及领域诊断历史。
- Platform 不定义全领域 reason_code 大枚举，不接管领域质量模型、领域历史或证据存储。
- Runner 首期只执行只读、有限成本、遵守 Context 取消约定的 Check；写入、通知和高成本全量探测由领域侧单独授权和编排。
- WebUI 只消费 Server 输出，不在前端重新计算健康度或领域质量。

## 2. Observation 1.0.0 DTO

权威 JSON Schema：
`doc/platform/schema/PLAT-CTR-004-observation-1.0.0.schema.json`

权威示例：
- `doc/platform/golden/PLAT-CTR-004-observation-healthy.golden.json`
- `doc/platform/golden/PLAT-CTR-004-observation-unavailable.golden.json`

字段定义：

| 字段 | JSON/Go 类型 | 必填与语义 |
|---|---|---|
| status | string / `Status` | 必填；仅允许 healthy、degraded、unavailable、skipped、failed |
| reason_code | string | healthy 可省略；其他四种状态必填。稳定、低敏感、机器可处理的领域标识，不是全局枚举 |
| safe_message | string | 必填；脱敏的人类可读摘要，不包含正文、snippet、URL Query、凭证、SQL、堆栈或原始上游响应 |
| checked_at | date-time / `time.Time` | 必填；UTC RFC3339Nano，必须以 `Z` 结尾。Runner 输出前统一转 UTC；缺失时由 Runner 填入当前 UTC 时间 |
| duration | integer / `time.Duration` | 必填；非负纳秒数，Go 表示范围为 int64；Check 未提供时由 Runner 填入实际执行耗时 |
| evidence_ref | string | 可选；不透明、受权限保护的领域证据引用，不内嵌证据，不承诺 Platform 可读取 |
| attributes | object，值为 string | 可选；只允许领域显式白名单中的低敏感字符串标量，不允许任意 JSON、正文、snippet、原始响应或堆栈 |

字段约束：

- `reason_code`：1～160 UTF-8 字节；匹配 `^[a-z0-9][a-z0-9._-]*$`。非 healthy 必填，healthy 可省略。
- `safe_message`：TrimSpace 后非空，最多 512 UTF-8 字节，不含 Unicode 控制字符。
- `checked_at`：只接受 Runner 产出的 UTC 时间表示；不得把任意调用方直接 Marshal 的非 UTC Observation 当作合规输出。
- `duration`：非负；JSON 数值单位固定为纳秒。Check 未提供时使用零值，Runner 以实际耗时填充；负值不是有效输入。
- `evidence_ref`：最多 512 UTF-8 字节，不含控制字符；语义、访问权限和保留期由领域 Owner 定义。
- `attributes`：最多 16 项；key TrimSpace 后非空且最多 64 UTF-8 字节；value 最多 256 UTF-8 字节；key/value 均不得含控制字符；所有 key 与 value 的 UTF-8 字节总和最多 2048。值类型固定为 string，不扩展为 number、boolean、array 或 object。

## 3. Schema 与 Go Validator 的职责

JSON Schema 是跨模块 JSON 结构与基本类型的机器约束；Go `Observation.Validate()` 是 Platform 运行时的完整约束入口。二者不是逐字节等价的双重 Validator：

- JSON Schema 的 `minLength/maxLength` 按 Unicode 字符计数；Go Validator 的长度上限按 UTF-8 字节计数。Schema 的字符长度约束是结构性上限，Go Validator 的字节上限是运行时权威。
- Schema 表达单字段长度、类型、枚举、控制字符基础约束和非 healthy 的 reason_code 条件；Go Validator 额外执行 TrimSpace 非空、Unicode 控制字符和 attributes 总字节预算。
- attributes 的 2048 字节合计上限由 Go Validator 执行；不使用非标准 JSON Schema 扩展伪造跨属性字节求和。
- `checked_at` 的 UTC 规范化由 Runner 保证；直接构造并序列化 Observation 不构成 Runner 的合规输出路径。
- Schema `additionalProperties:false` 是 Observation 1.0.0 的结构边界。兼容性及未知字段处理遵循 GOV-CTR-004。

因此 Schema 验证通过不替代 Go Validator；Go Runner 对外输出必须满足两者的交集。Schema 不应拒绝任何由合规 Runner 生成的 Observation。

## 4. Check 与 Runner 进程内 API

等价权威 Go API：

```go
type CheckFunc func(context.Context) (Observation, error)

type Check struct {
    ID       string
    Name     string
    Required bool
    Timeout  time.Duration
    Run      CheckFunc
}

type Runner struct {
    MaxConcurrent int
    TotalTimeout  time.Duration
}

func (r Runner) Run(ctx context.Context, checks []Check) (Report, error)
```

规则：

- Check.ID 必填，TrimSpace 后非空，最多 128 UTF-8 字节，不含控制字符；同一次 Run 内必须唯一。
- Check.Name 可选，最多 128 UTF-8 字节，不含控制字符。
- Check.Run 必填；Check.Timeout 为零时使用 5 秒默认值，负值拒绝。
- 单次 Run 最多 128 个 Check。
- Runner.MaxConcurrent 小于等于零时使用默认 4，最大 32；超过上限拒绝。
- Runner.TotalTimeout 小于等于零时使用默认 30 秒；外部父 Context 更早取消/截止时，以父 Context 为准。
- 每个 Check 独立执行；单项超时/失败不阻断其他 Check。Runner 通过 Context 传递取消；Check 实现必须响应取消，Go 不保证能强制终止忽略 Context 的函数。
- 重复 ID、无效 Check 或 nil Context 在执行前整体拒绝；空 Check 列表返回空 Results 和 skipped Summary。
- Results 顺序必须与输入 Checks 顺序一致，不因并发执行顺序改变。
- Check panic 的 payload 不得进入 Fault、Observation 或日志；Runner 使用固定安全错误摘要。
- Check 返回的 Fault 仅作为进程内 `Result.Fault` 保留，不进入 JSON；Fault 的公共字段及错误映射遵循 PLAT-CTR-001。

## 5. Result、Report 与聚合

进程内返回结构的 JSON 字段形状：

- `Report`：`summary`、`results`。
- `Summary`：`status`、`counts`。
- `Counts`：`healthy`、`degraded`、`unavailable`、`skipped`、`failed` 五项计数。
- `Result`：`check_id`、可选 `name`、`required`、`observation`；内部 Fault 不序列化。

Summary 聚合优先级固定：

1. 任一 Required Check 为 failed，总体为 failed。
2. 否则，任一 Required Check 为 unavailable，总体为 unavailable。
3. 否则，任一 Check 为 degraded，或任一 Optional Check 为 failed/unavailable，总体为 degraded。
4. 若所有 Check 均为 skipped，或 Check 列表为空，总体为 skipped。
5. 其余情况为 healthy。

补充规则：

- Optional Check 的 skipped 不降低总体状态。
- Required Check 返回 skipped 属于契约错误；Runner 将该结果改为 failed，并提供固定 reason_code/safe_message。
- Check 返回 Fault 时，timeout/unavailable 类 Fault 映射为 unavailable Observation；其他 Fault/普通错误映射为 failed。
- 总体 Summary 只表示 Check 执行和基础能力摘要，不覆盖领域的 ProviderOutcome、Quality、Freshness、Coverage、Ingestion Run、Analysis Run 或 Delivery Run 状态。

`Report` 是 Platform Runner 的进程内结果结构，不构成完整的公开 Diagnostics HTTP DTO。Server 必须由自己的版本化 HTTP 契约定义完整响应、认证和分页；不得仅因 Platform 结构带 JSON Tag 就把它直接视为 Server 公共 Envelope。

## 6. Golden、Schema 与实现验收边界

- Observation 1.0.0 Schema 和 healthy/unavailable 两份 Golden 是本契约的机器定义与基础示例。
- Go Contract Test 验证 Observation.Validate、Runner 聚合、结果顺序、超时/取消和 required skipped 规则。
- Schema 与 Go Validator 的职责分工以第 3 节为准；Schema 与 Go 的每个边界条件不要求重复实现相同的字节求和算法。
- Server 完整 Diagnostics Report Schema/HTTP Golden、领域 Adapter 的 reason_code/attributes 白名单、真实消费者接线、CI 与部署验收分别归 Server/领域实现符合性工作，不阻断本轮 Platform 定义冻结。

## 7. 版本与兼容

本契约首版为 1.0.0。Observation 字段类型/必填性、状态枚举、reason_code 规则、时间单位/时区、duration 单位、attributes 类型/边界、Runner 聚合优先级或资源限制发生不兼容变化时，按 GOV-CTR-004 升级版本。新增领域 reason_code 不改变 Platform Schema 版本；由对应领域 Owner 维护词汇和兼容性。
