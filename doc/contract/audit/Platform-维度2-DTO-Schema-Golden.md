# Platform 维度 2：DTO、Schema、Golden 契约核验

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**文档维护分支：** `feat/design_doc`  
**本轮范围：** Platform Diagnostics `Observation` DTO 单切片  
**不在本轮范围：** Fault HTTP Envelope（由真实 Transport/Server Owner 负责）、Request ID 元数据契约、HTTP Client Config/Response、其他模块领域 DTO；不修改业务代码、Schema、Golden 或测试。

## 1. 切片选择与权威边界

选择 `internal/platform/diagnostics.Observation`，原因：
- 它是 Platform 自身拥有、被 Server Diagnostics 报告直接嵌套的跨层结构。
- 已有版本化 JSON Schema 和两份 Golden，且 Go 侧存在 Validator 与 Golden 反序列化/再序列化测试，适合做字段级三方核验。
- 项目级公共契约编号为 `PLAT-CTR-004`，当前仍是 `pending_evidence`；本轮补充证据，不自行宣布 frozen，也不新增重复 CTR。

权威实现与契约资产：
- Go DTO：`internal/platform/diagnostics/observation.go`，`Observation`。
- 相关执行/包装结构：`internal/platform/diagnostics/aggregate.go` 的 `Result`、`Report`；`Result.Fault` 明确 `json:"-"`。
- Schema：`doc/platform/schema/PLAT-CTR-004-observation-1.0.0.schema.json`。
- Golden：`doc/platform/golden/PLAT-CTR-004-observation-healthy.golden.json`、`...-unavailable.golden.json`。
- Go 契约测试：`internal/platform/diagnostics/observation_contract_test.go`；聚合契约测试：`internal/platform/diagnostics/contract_test.go`。

## 2. Go DTO 字段核对

| 字段 | Go 类型/JSON Tag | Go Validator/运行时语义 |
|---|---|---|
| status | `Status` / `json:"status"` | 固定五态：healthy、degraded、unavailable、skipped、failed |
| reason_code | `string` / `omitempty` | 非 healthy 必填；稳定小写标识；最大 160 字节 |
| safe_message | `string` | 必填；TrimSpace 后非空；无控制字符；最大 512 字节 |
| checked_at | `time.Time` | 必填；非零；Runner 归一化为 UTC |
| duration | `time.Duration` | 非负；JSON 数值单位为纳秒 |
| evidence_ref | `string` / `omitempty` | 可选；无控制字符；最大 512 字节；不透明引用 |
| attributes | `map[string]string` / `omitempty` | 最多16项；key非空/最多64字节；value最多256字节；总计最多2048字节；禁止控制字符 |

Go 实现路径：
- `Observation.Validate()` 检查状态、必填语义、标识格式、文本长度、控制字符和 attributes 边界。
- `Runner.execute` 对 `checked_at` 统一 UTC；缺省时间由 Runner 补当前 UTC 时间；缺省/非正 duration 由 Runner 补实际耗时。
- `Result.Fault` 不参与 JSON 序列化，内部 Cause 不会通过 Observation 暴露。

## 3. Schema 与 Golden 对照

Schema 的字段集合、状态枚举、required 集合、条件 reason_code、duration 非负整数、attributes 项数和单项长度，与 Go DTO 的主要结构相符。两份 Golden 分别覆盖 healthy（无 reason_code）与 unavailable（有 reason_code/evidence_ref/attributes），并由 Go 测试反序列化、Validate、再序列化、再次 Validate。

发现以下需要处理的契约差异/补证：

| 编号 | 发现 | 证据与影响 | 关闭所需证据 |
|---|---|---|---|
| PLAT2-D2-001 | Go 使用字节长度限制，JSON Schema 的 maxLength/minLength 按 Unicode 字符数计算，非 ASCII 内容可能被 Schema 接受但被 Go 拒绝 | Go 对 reason_code/safe_message/evidence_ref/attributes key/value 使用 `len()` 字节计数；Schema 使用 `maxLength`。中文 safe_message 的 Golden 已存在，但未覆盖边界长度差异 | 明确公共契约以 UTF-8 字节还是 Unicode 字符计长；同步 Validator/Schema 描述或调整实现；增加中文多字节边界 Golden/Contract Test |
| PLAT2-D2-002 | Schema 未表达 Go 对 safe_message 与 attributes key 的 TrimSpace 非空约束 | Schema 的 `minLength: 1` / key 正则允许纯空格；Go `strings.TrimSpace(...)` 会拒绝 | 增加 Schema 可表达的非空白约束并补空白/制表符边界样例，或明确由 Go Validator 承担额外语义并在契约文档标注 |
| PLAT2-D2-003 | Go 对 attributes 设置 2048 字节总量上限，Schema 仅限制项数及单个 key/value 长度 | 大量合法长度属性可通过 Schema，但总量超过 2048 字节会被 Go 拒绝；标准 JSON Schema 不便表达 key/value 字节总和 | 明确 Schema 是结构约束、Go Validator 是总预算权威；补超总量 Golden/Contract Test 与文档说明，不为形式一致伪造 Schema 关键字 |
| PLAT2-D2-004 | 当前 Golden 测试只证明 Golden 与 Go Validator 双向可读，不证明 JSON Schema 与 Go 对同一组边界样例给出相同结论 | `TestPLATCTR004GoldenFixturesSatisfyGoValidator` 未调用 JSON Schema Validator；需确认 CI 中是否有独立 Schema 校验脚本及其边界覆盖 | 找到并核实固定基线 CI Schema 校验入口；补同一组有效/无效边界样例的 Schema 与 Go 双重校验，或登记明确的验证职责分工 |

注意：
- Schema 的 `additionalProperties:false` 比 Go 默认 `json.Unmarshal` 的未知字段行为更严格。当前未找到 Platform 对外接收 Observation JSON 的反序列化入口；本轮不将其直接判为缺陷，若未来出现跨进程输入边界，再明确严格解码策略。
- `checked_at` Schema 要求 UTC 的 `Z` 后缀；Runner 输出 UTC，Golden 也使用 Z。直接构造的 Observation 在 Runner 外部 marshal 时不自动归一化，因此该约束由 Runner 输出路径保证；需要避免将任意调用方直接 marshal 视为已验证输出。
- `duration` 是 Go `time.Duration`，JSON 数字单位为纳秒；Schema 描述已明确该单位。
- healthy 的 `reason_code` 可省略；非 healthy 条件必填与 Go Validator 一致。
- `evidence_ref` 和 `attributes` 可省略；Go omitempty 与 Schema 非 required 一致。

## 4. 真实消费者与传输边界

已确认的生产消费链：
1. News `internal/news/diagnostics/checks.go`、`business_quality.go` 构造 Platform Check/Observation；模块测试由 Platform Runner 执行。
2. Research `internal/research/diagnostics/checks.go` 与 `internal/research/application/diagnostics.go` 将领域快照检查映射为 Platform Observation。
3. Server `internal/server/diagnostics/service.go` 聚合 Platform Runner 和模块 Checks；`internal/server/diagnostics/aggregator.go` 将 Platform Report 包装为 Server Report。
4. Server `internal/server/health/handler.go` 的 `/diagnostics` 路由直接 JSON 输出 Report；`/readyz` 则只输出 CheckID 到状态的简化映射。

因此 Observation 不只是 Platform 包内部 DTO：它嵌套在 Server 的 `/diagnostics` HTTP 响应中。但当前固定基线只找到 Observation 自身的 JSON Schema/Golden，未发现完整 Server Diagnostics Report 的权威 Schema/HTTP Golden。该完整响应契约归 Server 维度2继续核验，本轮不越界补建 Server Schema。

固定基线 Server Bootstrap 尚未把 News/Research Checks 注入生产 Diagnostics Service（见 Platform 维度1审计记录）；以上消费者证据证明模块级适配和 Server 序列化路径存在，不证明这些领域 Checks 已在当前生产组合根运行。

## 5. 本切片结论

**状态：partial / PLAT-CTR-004 pending_evidence。**

已证明：
- Observation Go DTO、版本化 Schema、healthy/unavailable Golden 和 Go Validator 存在。
- 必填字段、状态枚举、条件 reason_code、UTC 时间输出、纳秒 duration 和主要字段边界有对应实现。
- News、Research 是模块级生产方；Server Diagnostics/Health Handler 是聚合及 HTTP 输出消费者。
- Fault 内部对象不会通过 Result JSON 直接暴露。

尚未证明：
- Go 字节长度与 Schema 字符长度差异已裁决。
- safe_message/attributes key 的空白语义在 Schema 中等价表达。
- attributes 总量约束的权威验证职责已写清并有边界测试。
- Schema 与 Go 对同一边界样例的双重校验已建立。
- Server 完整 Diagnostics Report 的 Schema/HTTP Golden 已存在（转入 Server 维度2，不在本切片重复处理）。

不新增 CTR；以上四项作为 PLAT-CTR-004 的证据缺口记录。不得仅凭当前 Go Golden 测试通过将 PLAT-CTR-004 标记 frozen。

## 6. 下一步

继续阶段B十模块六维核验，下一切片保持在 **Platform 维度2**：核对 Platform `Fault` 是否确实仅作为内部错误对象、各 HTTP/领域 Adapter 的映射边界是否有明确 DTO/测试证据。只核 Platform 自身生产方及直接消费者映射，不扩展到 Server 的完整 HTTP Error Envelope（留给 Server 维度2），不修改业务实现。完成后再按证据决定 Platform 维度2是否还有独立切片。

## 7. Fault 内部错误对象与映射边界（第二切片，2026-11-10）

### 7.1 内部对象契约

核验文件：
- `internal/platform/fault/fault.go`：`Class`、`Fault`、`Valid()`、`New()`、`Validate()`、`Error()`、`Unwrap()`。
- `internal/platform/fault/fault_test.go`、`class_test.go`、`unwrap_test.go`。
- `doc/platform/08-公共平台契约规范.md` 第4、24节。
- `doc/platform/17-PLAT-002公共契约差异与待决事项.md` 第3、8节及P002-D02。

`Fault` 字段全部标记 `json:"-"`，直接 `json.Marshal(Fault)` 的测试结果为 `{}`。这不是缺少对外 Schema，而是契约明确要求 Fault 只作为进程内错误对象；HTTP Transport 必须映射到独立公开 DTO。Fault 本身不应创建对外 JSON Schema/Golden，否则会错误地把内部结构固化为传输协议。

| 字段/行为 | 实现与测试证据 | 核验结论 |
|---|---|---|
| Class | 固定8类；`Class.Valid()` 与测试枚举一致 | 通过 |
| Code | 小写稳定标识，最多160字节；空值/非法标识拒绝 | 通过 |
| SafeMessage | TrimSpace后非空，最多512字节，不含控制字符 | 通过；语义仍要求生产方脱敏 |
| Dependency | 可选稳定逻辑标识，最多128字节；拒绝URL/IP/非法标识 | 通过 |
| Retryable | 仅 unavailable/timeout 允许为true | 通过；只是候选提示，不是重试命令 |
| Cause | error链由Unwrap保留；JSON隐藏 | 通过 |
| New | 仅构造对象，不自动调用Validate | 需依赖各边界/消费者校验；不可把New等同有效对象 |

### 7.2 生产方与消费者

- Platform HTTP Client `client.go` / `stream.go` 构造 Fault，显式提供稳定 Code、SafeMessage、Dependency、Retryable 和内部 Cause。缓冲/流式故障分类由 Platform 自身测试覆盖。
- Platform Diagnostics Runner 的 `normalizeFault` 对传入 Fault 调用 `Validate()`；无效 Class/Code/SafeMessage/Dependency/Retryable 会归一化为固定 `platform.diagnostics.invalid_fault`，不将不可信元数据写入 Observation。对应 Runner 测试覆盖未知 Class、控制字符、URL/IP Dependency、Retryable validation。
- LLM `ModelErrorFromPlatformFault` 是本基线中已确认的直接领域消费者。它将 Class/Code 映射为 LLM ModelErrorKind，保留 Retryable 和 SafeMessage，明确不复制 Dependency/Cause。LLM 测试覆盖8类映射、缺省Code/Message、response_too_large、取消/deadline，以及 Dependency/Cause 不泄露。
- Market Data HTTP 的 `writeErr` / `writeDomainErr` 走自己的公开 `errorBody` 映射，不直接序列化 Platform Fault；其 `platform_contract_test.go` 覆盖错误类别、默认不重试、Request ID、领域错误Cause脱敏和 OpenAPI Error Schema。该测试不能当成 Platform Fault 已被该HTTP Adapter直接消费的证据。
- Platform HTTP API 的任务错误目前由独立 `writeError` 输出其HTTP DTO；未发现该层直接接收/映射 `*fault.Fault` 的证据。

### 7.3 保留的映射边界证据缺口

| 编号 | 发现 | 当前判断 | 关闭/补证条件 |
|---|---|---|---|
| PLAT2-D2-005 | LLM `ModelErrorFromPlatformFault` 使用 `errors.As` 取出 Fault 后，直接投影 Class/Code/SafeMessage/Retryable，没有调用 `Fault.Validate()`；现有测试覆盖合法映射和Dependency/Cause排除，但未覆盖非法Class/Code、控制字符SafeMessage、非法Retryable组合 | **证据缺口，尚不能据此认定生产数据可被外部攻击者控制。** Platform HTTP Client构造的Fault来自受控内部路径；但该函数是跨模块投影边界，需明确它只接受可信且已校验的Platform Fault，或在投影前校验并安全降级 | LLM Owner补充信任边界说明；增加无效元数据映射测试。若选择在Mapper校验，非法Fault必须映射为固定安全的internal/upstream错误，不能回传原始SafeMessage/Code。由LLM模块验收，Platform提供Validate语义支持 |

此项并入既有项目差异 `P002-D02 / PLAT-CTR-001` 的消费者映射补证，不新增项目CTR。Platform Runner已执行Validate不代表所有其他消费者都自动经过Runner；LLM直接接收Fault的路径是独立边界。

### 7.4 本切片结论

**Fault内部对象契约本身：代码与单测证据基本齐备；跨模块投影完整性：partial。**

- Fault 不应有公开 JSON Schema/Golden；`{}` 序列化测试与契约文档一致。
- Fault 字段校验、内部Cause链、Retryable限制和Dependency逻辑标识边界有实现及测试证据。
- Diagnostics Runner 对Fault做校验/归一化；LLM Mapper 是独立直接消费者，不能把Runner的校验覆盖推定到LLM路径。
- P002-D02仍开放，直至Transport/Server与领域消费者的公开DTO映射证据齐备；本轮只核Platform Fault内部结构和已确认直接消费者，不核Server完整HTTP Error Envelope。
- 不新增CTR，不修改Fault实现、LLM实现、Schema、Golden或测试，不执行CI。

### 7.5 下一切片建议

Platform维度2下一切片核对 Request ID：区分内部Request Context/ID生成与各Transport公开响应头/DTO；只核Platform公共原语及直接消费者证据。Server完整HTTP错误Envelope与路由响应契约仍留在Server维度2。