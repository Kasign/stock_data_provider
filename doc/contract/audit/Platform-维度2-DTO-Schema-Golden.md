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
## 8. Request ID 公共原语与直接 Transport 消费者（第三切片，2026-11-10）

### 8.1 原语与Golden

核验文件：
- `internal/platform/observability/request_id.go`
- `internal/platform/observability/request_id_test.go`、`request_id_contract_test.go`
- `doc/platform/golden/PLAT-CTR-002-request-id.golden.json`
- `doc/platform/08-公共平台契约规范.md` 第5节

已确认契约：
- Header固定为 `X-Request-ID`；有效值为1～128字节ASCII token，仅允许字母、数字、`-`、`_`、`.`、`:`。
- `MiddlewareWithOptions` 零值策略不信任任何客户端输入，缺失、非法、多值均生成新ID；只有显式启用 `TrustIncomingRequestID` 才接受单值合法ID。
- 生成器优先使用16字节密码学随机数并编码为32位小写hex；随机源失败时回退到UTC纳秒时间与进程内原子计数器。
- ID进入私有Context Key；`WithRequestID` 只接收有效值；`PropagateRequestID` 只从Context取值覆盖出站Header，Context无ID时删除调用方预设Header。
- `Middleware` 保留旧的信任单值Header行为并标记Deprecated，契约限定只可用于可信Gateway覆盖且网络不可绕过的部署。

Golden覆盖Header名、最大长度、生成随机字节数、默认不信任、合法/非法值、重复Header、出站覆盖与无Context删除。Contract Test读取Golden并验证向量；单元测试另覆盖128字节边界、非法字符、重复值、显式可信选项和Context传播。

### 8.2 Server入口与日志调用链

- `internal/server/bootstrap/bootstrap.go` 在创建真实HTTP Server前，将完整路由Handler包裹在 `newHTTPMiddleware` 外层。
- `internal/server/bootstrap/http_middleware.go` 最外层使用 `MiddlewareWithOptions(..., MiddlewareOptions{})`，因此Root Server直接可达时会重建客户端自报ID；随后才进入并发限制、Body限制、Router和旧Analysis API Handler。
- 旧 `internal/platform/httpapi.Server.Handler()` 内部仍使用Deprecated `Middleware`，但当前Root Server组合根在其外层先生成可信Context ID；内层只会读到外层生成的单值ID。其独立Handler若被其他入口直接暴露，则仍继承旧信任行为，必须满足契约的可信Gateway前提。
- `requestLog`、任务创建/失败日志和panic日志从Platform Context读取并记录 `request_id`。外层Request ID Middleware先执行，因此并发拒绝、Body限制提前返回等响应也会带最终ID响应Header。
- Server中间件测试覆盖客户端值重建；但没有发现一个集成测试将真实Root Server入口、内部旧Handler和出站HTTP调用串联起来验证同一个ID端到端贯通。

### 8.3 出站传播与异步ID隔离

- Platform HTTP Client缓冲请求在每次尝试克隆Request后调用 `PropagateRequestID`；流式请求在发起网络调用前执行相同传播。
- 缓冲路径 `client_test.go` 覆盖Context ID出站传播；流式路径 `stream_test.go` 覆盖请求ID进入StreamStats；Request ID Contract Test覆盖有Context覆盖原Header、无Context删除原Header。
- Research `application/correlation.go` 使用独立私有Context Key承载Correlation ID；从Request ID Context不会自动继承Correlation ID，缺少可信Correlation ID时使用Research ID Generator生成。对应测试明确验证两种ID不混用。
- GOV-CTR-003要求的 `originating_request_id`、Worker/重试 `correlation_id`、Event ID及跨模块Golden/消费者完整链路不属于本Platform切片；登记簿仍将GOV-CTR-003列为pending_evidence，本轮不升级其状态。

### 8.4 证据缺口与结论

| 编号 | 发现 | 当前判断 | 关闭/补证条件 |
|---|---|---|---|
| PLAT2-D2-006 | Middleware、Server入口、缓冲/流式出站传播分别有测试，但缺少真实Root Server入口到出站Platform HTTP Client的端到端测试，未在一条链路中证明入口重建的ID被同一请求Context和出站Header持续使用 | 集成证据缺口，不是已证实的实现错误；静态组合根接线和各组件测试均存在 | Server/Integration Owner补一条组合测试：客户端伪造ID被重建，响应Header/请求日志使用重建ID，出站缓冲及流式调用只传播该ID；无Context时不透传调用方Header |
| P002-D03（既有） | Deprecated `Middleware` 仍保留信任单值Header的兼容行为；Root Server外层已使用零值安全Middleware，但独立暴露旧 `api.Handler()` 的入口仍必须受可信Gateway拓扑约束 | Root Server当前接线符合安全默认；兼容入口的部署约束和迁移/移除仍需Server/Integration证据 | 证明旧Handler没有绕过Root Server的生产暴露路径，或迁移所有入口后移除兼容Middleware；更新部署拓扑验收证据 |

本切片不新增CTR。PLAT-CTR-002原语及Golden/本地测试证据基本齐备；P002-D03仍需部署入口证据，PLAT-CTR-002不能仅凭包级测试标记frozen。GOV-CTR-003继续独立开放。

**切片结论：partial（原语与局部消费者通过，组合链路端到端证据待补）。** 不核Server完整HTTP Error Envelope，不重审Fault/Observation，不修改实现或测试，不执行CI。

### 8.5 下一切片建议

Platform维度2剩余最直接的公共结构是HTTP Client配置与结果/流式结果的契约边界。下一轮只核Platform HTTP Client对外配置、DestinationPolicy、缓冲/流式结果元数据与现有PLAT-CTR-005 Golden/Contract Test；不扩展到SSRF算法复审（归其他维度/专项安全审查），不审Server LLM Transport业务映射。
## 9. HTTP Client 配置、DestinationPolicy 与缓冲/流式结果（第四切片，2026-11-10）

### 9.1 公共API边界与机器契约适用性

核验文件：
- `internal/platform/httpclient/client.go`、`bound_doer.go`、`stream.go`、`address.go`
- `internal/platform/httpclient/contract_test.go`、`client_test.go`、`bound_doer_test.go`、`stream_test.go`
- `doc/platform/08-公共平台契约规范.md` 第8、23、25节
- `doc/platform/15-受控HTTP流式传输设计与验收方案.md` 第3、6～9节及21项A1验收矩阵
- 项目契约 `doc/整体任务规划/02-契约治理/跨模块契约目录与冻结状态.md` 中PLAT-CTR-005条目

PLAT-CTR-005在本基线中定义的是进程内Go调用契约，不是跨网络JSON DTO：`Config`、`DestinationPolicy`、`BoundDoer`、`StreamResponse`、`StreamStats`均不通过JSON序列化作为平台传输协议。故不应为了形式完整而给这些Go结构创建JSON Schema或JSON Golden。可用版本化Go API定义、契约文档、可执行Go Contract Test及消费者适配测试作为机器/行为证据；但项目冻结登记簿应明确记录“JSON Schema/Golden不适用”的理由及替代验收证据，不能把仓库中不存在的PLAT-CTR-005 JSON Golden误报为测试失败。

### 9.2 Config默认值、上限与资源隔离

代码已实现并校验：
- 缓冲请求：总Timeout默认15s、Dial 5s、TLS握手5s、响应头8s、MaxResponseBytes默认2MiB/硬上限16MiB、MaxRetries默认0/范围0～2、fallback MaxConcurrent默认4/硬上限32。
- 响应头默认上限1MiB/硬上限4MiB；代码显式设置到共享Transport。
- 流式：MaxStreamBytes默认16MiB/硬上限64MiB；StreamTimeout默认10min/上限30min；StreamIdleTimeout默认30s/上限5min；MaxConcurrentStreams默认2/上限8。
- DependencyLimits与StreamDependencyLimits均最多64项；依赖标识校验；缓冲并发单项1～32，流并发单项1～MaxConcurrentStreams。两套流/缓冲并发池相互隔离；未知Dependency使用有界fallback池，避免按请求动态增长Map。
- New复制两类并发配置Map；Bind复制AllowedHosts/AllowedPorts切片；Do/OpenStream入口再次复制策略切片，防止调用方后续修改改变本次策略。
- Transport禁用环境代理、最低TLS 1.2，响应头上限显式生效。

现有测试覆盖缓冲默认值中的主要项、MaxResponseBytes/Retry/缓冲并发边界、DependencyLimits Map复制；流式专项测试覆盖流并发/字节/超时上限、并发隔离、取消释放和完整生命周期。以下契约测试覆盖尚不完整：

| 编号 | 证据缺口 | 说明/补证条件 |
|---|---|---|
| PLAT2-D2-007 | `TestNewAppliesPLATCTR005Defaults`未断言MaxResponseHeaderBytes及四项流式默认值；`TestNewEnforcesPLATCTR005UpperBounds`未覆盖MaxResponseHeaderBytes 4MiB硬上限；没有找到StreamDependencyLimits非法值/条目数/Map复制的专门配置测试 | 代码已有相应默认和校验逻辑，现阶段是契约回归覆盖缺口，不据此认定实现错误。补齐Config零值默认断言、MaxResponseHeaderBytes边界、StreamDependencyLimits空白/非法/越界/最多64项及调用方Map变更隔离测试。将测试断言与第15节配置语义逐字段对应 |

### 9.3 DestinationPolicy 与BoundDoer

- 每次Do/OpenStream都必须提供DestinationPolicy；Dependency为稳定逻辑标识；AllowedHosts精确匹配且拒绝wildcard；AllowedPorts拒绝0；Host/Port集合有界；HTTP需显式AllowHTTP；重定向需显式AllowRedirects且最多3跳。
- BoundDoer在Bind时校验并复制策略切片；每个固定Provider endpoint单独绑定，不允许请求替换目的地址策略。
- Redirect默认拒绝；显式启用仍限制同源、禁止HTTPS降级、逐跳复核目标、限制跳数和循环。此处只核对策略输入/消费边界，不重复审查SSRF/DNS/IP算法。
- News Provider和Source Adapter通过BoundDoer绑定各自endpoint；Provider策略默认不允许HTTP重定向、不声明幂等。LLM Server Transport校验Provider BaseURL与单一精确Host/Port策略匹配，强制HTTPS、禁重定向、禁幂等重试；同一BoundDoer供JSON和SSE使用。
- News Provider Fault Mapper调用Fault.Validate后只传Code/Retryable，不透传Cause；与前述LLM Fault Mapper的校验差异已由PLAT2-D2-005登记，不在本切片重复登记。

### 9.4 缓冲与流式结果、资源所有权

**缓冲Do：** 仅在响应体完整读取且未超过MaxResponseBytes后返回；底层Body已关闭，返回Body是内存缓冲副本；调用方仍需关闭返回Body。请求并发额度在完整读取后释放。重试仅在策略显式幂等、Body可重放且总Context仍有效时进行；429不重试。响应体读取失败/超限时不返回半成品响应。

**流式OpenStream：** 响应头到达后立即返回StatusCode、独立Header快照、受控Body；不预读、不整体缓冲。Body所有权转移给调用方，EOF/读取错误/取消/deadline/主动Close均通过幂等终止状态清理底层Body、Context和并发额度。流式额度覆盖从排队到终止的整个生命周期，并与缓冲池隔离。单次阻塞Read空闲超时不包含两次Read之间的调用方处理时间；累计字节按解压后实际交付量计数。

`StreamStats`只含RequestID、Dependency、StatusCode、StartedAt、Duration、BytesRead、TerminationReason；不含URL、Query、凭据、Header、Body和原始错误。测试覆盖EOF、caller_close、cancel、deadline、idle_timeout、body_limit、read_error/upstream truncation、字节计数、响应头等待计时、HTTP/1.1/HTTP2、背压、并发额度释放及安全统计。

LLM消费者边界：`server/llmtransport.Transport.OpenStream`将Platform StreamResponse映射到LLM `HTTPStreamResponse`，复制Header并转移Body所有权；LLM Adapter在成功、HTTP状态错误、解析失败或取消路径均defer/显式Close Body。LLM窄接口没有暴露Platform `StreamStats`，因此LLM领域层不能直接将Platform传输终态/字节数/耗时写入自己的诊断。当前没有证据证明这违反已冻结LLM契约，但需由LLM/Server Owner确认A2诊断是否要求这些数据；若要求，应由Server Transport适配成LLM自有安全统计DTO，不让LLM直接依赖Platform内部统计类型。

| 编号 | 发现 | 当前判断 | 补证/关闭条件 |
|---|---|---|---|
| PLAT2-D2-008 | LLM Transport映射时不暴露Platform StreamStats | 条件性消费者可观测性缺口，不判定为Platform API缺陷；LLM已有自己的ModelResponse Duration，但不等价于Platform流传输Duration/BytesRead/TerminationReason | LLM/Server Owner确认A2诊断需求；若不需要，记录不适用理由；若需要，补安全字段映射和消费者测试 |

### 9.5 本切片结论

**Platform HTTP Client A1：实现和局部测试证据基本齐备；配置边界契约测试仍有缺口；跨模块/生产验收未完成。**

- 未发现本切片范围内已证实的HTTP Client配置或流式生命周期实现错误。
- PLAT-CTR-005是内部Go API契约，JSON Schema/Golden不适用的判断有技术依据，但项目登记簿尚需明确记录替代机器证据及不适用理由。
- PLAT2-D2-007并入既有PLAT-CTR-005配置/Contract Test补证，不新增CTR；PLAT2-D2-008并入LLM/Server A2消费者诊断确认，不新增CTR。
- Platform A1测试矩阵在专项文档中列出21项；本切片未重新运行CI，不能将固定基线的历史CI结果冒充本轮执行结果。LLM A2真实Gateway联调、Server资源装配与生产A3网络/Secret/Runtime验收仍是独立门禁。
- 本轮不修改业务代码、Schema、Golden、测试或CI。

### 9.6 下一切片建议

本轮HTTP Client切片完成后，不立即转Server维度2。回看项目Platform契约目录，PLAT-CTR-003（身份/授权）、PLAT-CTR-006（Storage/Migration生命周期）、PLAT-CTR-007（Typed Config/Secret）仍属于Platform公共契约，尚未在本审计文件中完成DTO/Schema/Golden适用性核验。下一切片先核PLAT-CTR-003：仅盘点Platform身份/授权相关公开结构、Header/Context边界、序列化适用性和直接消费者；不做Security算法/权限策略重审（维度4），不修改实现。确认该CTR在维度2是否存在适用机器契约后，再依次评估CTR-006/007；完成Platform维度2范围清单后才转Server维度2。

## 10. PLAT-CTR-003 身份、授权与 Secret 边界（第五切片，2026-11-10）

### 10.1 本轮范围与契约边界

固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。本轮对照项目契约目录 PLAT-CTR-003 原始定义，核对 Platform Bearer 身份原语、Principal/RoutePolicy、Secret 凭证边界和 Server 直接消费。PLAT-CTR-003 当前明确只包括 anonymous、owner Bearer、service Bearer；Owner Web Session 是后续独立扩展，不得静默并入该编号或据此宣布原契约已冻结。

### 10.2 Platform Bearer 原语

核验文件：
- `internal/platform/security/security.go`：`IdentityKind`、`Principal`、`ServiceCredential`、`Authenticator`、`RoutePolicy`、`Guard`、`PrincipalFromContext`。
- `internal/platform/security/security_test.go`、`rotation_test.go`：认证、策略、Scope、快照不可变性和轮换测试。
- `internal/platform/config/env.go`（Secret API）及 `internal/platform/config/secret_contract_test.go`：Secret显式访问和脱敏。

已证实：
- 身份枚举为 anonymous/owner/service；Principal只包含Kind、Subject、Scopes，不含密码、Token或其他凭证明文。
- Owner凭证只配置为 `PLATFORM_ADMIN_TOKEN`，映射Subject为owner，Scope仅为 `platform:admin`。
- ServiceCredential由稳定Name、config.Secret和显式Scope组成；Name规范化后限制为1～64字节的小写ASCII标识，Authenticator内唯一。
- Token长度限制32～256字节，只允许URL-safe字母数字、连字符和下划线；代码注释明确语法校验不证明熵，熵要求仍由部署生成流程保证。
- Authenticator只保存SHA-256摘要；认证时扫描全部已配置摘要并使用ConstantTimeCompare；拒绝重复凭证、无效/重复/未知Scope及Service获得platform:admin。
- 凭证和Scope在Authenticator构造时复制为快照；Principal写入Context及读取时复制Scope切片。
- RoutePolicy字段私有，只通过AnonymousAccess/OwnerAccess/ServiceAccess/OwnerOrServiceAccess构造；Router拒绝无效策略、空Handler、根Catch-all及重复/非法路由；未注册路由保持404。
- Guard只从Authorization Bearer认证结果构造Principal并写入Context，不读取客户端自报身份Header。匿名请求仅能访问显式Anonymous策略；Owner不会自动继承Service Scope，Service必须满足路由声明的全部Scope。
- `config.Secret` 的String/GoString/格式化及JSON/Text输出脱敏已有契约测试；原始值只能显式调用Value获取。

### 10.3 真实消费者与接入状态

- Server `internal/server/config/security.go` 从配置加载Owner Secret。
- Server `internal/server/bootstrap/bootstrap.go` 调用 `NewAuthenticator(processConfig.Security.OwnerToken, nil)`，即当前生产组合根只装配Owner Bearer，未装配ServiceCredential列表。
- Server `internal/server/bootstrap/router.go` 将当前Health/Readiness/Diagnostics和旧Analysis路由显式注册为Anonymous或Owner策略；没有Service路由。
- `router_test.go` 覆盖匿名白名单、Owner访问、Service不能访问Owner路由、未注册路由404；Platform安全测试覆盖Service Scope与身份隔离。
- 固定基线Bootstrap未接入Cookie Session认证，也未把Session Principal注入Bearer Router；这符合文档要求的独立认证链原则，但表示Web Session A2尚未形成生产接入证据。
- 本轮未取得全仓所有模块对Principal/Scope的完整import与调用点扫描结果，不能据局部Bootstrap证据声称所有领域消费者均已完成授权Context接入。领域资源授权和AuthorizationRef签发/读取复核属于维度4，不在本轮判定。

### 10.4 序列化、Schema/Golden适用性

- `Principal`、`ServiceCredential`、`RoutePolicy`、`Authenticator`均为进程内Go结构；没有JSON Tag/公开JSON序列化入口，RoutePolicy/Authenticator内部字段不可导出。PLAT-CTR-003本身不需要为这些结构创建JSON Schema或HTTP Golden；应以Go Contract Test、真实Router/Handler测试和消费者Context测试作为机器证据。
- Owner/Service Bearer Token不应进入DTO、响应或Golden；Secret脱敏测试是适用证据。
- Session扩展需单独看待：`SessionRecord`具有JSON字段并被RedisSessionStore序列化为内部Redis存储记录，字段包括subject、created_at、last_seen_at、idle_expires_at、absolute_expires_at、credential_version及可选csrf_token_digest；原始Session Token不在记录中，Redis Key使用Token摘要。该记录不是PLAT-CTR-003公开HTTP DTO，但属于持久化序列化格式；当前未发现独立schema_version或Golden兼容夹具，相关证据应归入Web Session扩展的既有P002-D12～D14/Server A2准入工作，不将其伪装为PLAT-CTR-003原契约缺陷。
- OwnerCredential字段全部JSON隐藏，String/LogValue只暴露Subject和CredentialVersion；PasswordHash有独立序列化/日志脱敏测试。
- SecurityAuditEvent有受限JSON字段和Validate/LogValue，但Writer由Server注入；本轮未发现它作为外部HTTP DTO的证据，不要求对外Schema/Golden。

### 10.5 本切片结论与遗留证据

**结论：PLAT-CTR-003 Platform Bearer A1原语基本符合既有契约；Server当前只接入Owner Bearer，Service身份的生产配置/消费者链路和完整跨模块Context消费尚未证明，整体保持partial / pending_evidence。**

不新增CTR。既有缺口与关闭条件：
- PLAT-CTR-003原登记的凭证轮换、脱敏和RoutePolicy本地测试已有实现/测试证据；轮换是通过部署Secret替换并重建Authenticator完成，旧Authenticator快照不会自行热更新。真实部署Secret分发/重启/旧值失效证据仍属L3。
- Server当前Authenticator服务凭证参数为空，不能把Platform支持Service Scope等同于项目已部署服务间认证。需要Server A2按实际服务调用矩阵装配最小ServiceCredential，并由消费者测试证明Scope注入/拒绝路径；若当前没有服务间HTTP路由，需记录不适用范围，而不是虚构Service消费者。
- Web Session不属于当前PLAT-CTR-003的已批准范围。Platform已存在Password/CredentialStore/SessionStore/Cookie/CSRF/RateLimiter/Audit原语，但Server Bootstrap未装配；须按既有P002-D12～D14和独立Web Session决议完成契约批准、Session Middleware/路由矩阵、CredentialVersion实时复核、审计Writer与真实Redis/MariaDB A2证据。不得将Bearer Router改造成伪Session Bearer。
- Principal/Scope不是领域资源授权结论；Data/News/Research/Quant等资源Owner仍需在维度4核查AuthorizationRef签发和读取时复核。

本轮没有修改业务实现、Schema、Golden或测试，没有执行CI。

## 11. PLAT-CTR-006 Storage、Repository、Migration 机器契约适用性（第六切片，2026-11-10）

### 11.1 本轮范围

固定代码基线：`b6dfc85ba214fe16268c8cba780641a8bfe15086`。本轮只核对 Platform Storage API、Migration定义、Server Registry/Bootstrap与模块Migration生产方的字段/机器证据及消费者完整性。不审查连接池、事务、锁竞争、失败恢复和资源关闭算法正确性（维度5）。

项目权威登记簿 PLAT-CTR-006 明确：Platform拥有SQL/Redis连接生命周期、Ping和Migration Runner；各领域模块拥有自身Repository、表结构、SQL和Migration内容；Root Server显式注册/执行，打开连接不自动执行Migration；Migration以namespace+单调version识别，已发布版本不可修改，历史必须为完整有序前缀；MariaDB DDL逐条执行，不假设事务回滚。

### 11.2 Platform API与Migration字段

核验文件：
- `internal/platform/storage/storage.go`
- `internal/platform/storage/storage_test.go`
- `internal/platform/storage/integration_test.go`
- `internal/server/migration/registry.go`、`registry_test.go`

Platform结构：
- `SQLConfig`：MaxOpenConns、MaxIdleConns、ConnMaxLifetime、ConnMaxIdleTime、PingTimeout；为进程内配置结构，无JSON传输定义。
- `Migration`：Version uint64、Description string、Statements []string；为进程内模块到Server/Platform的Go API，不是跨网络DTO。
- Server `NamespaceResult`：namespace、status、duration、reason_code、recovery_required；`RunReport`：started_at、finished_at、duration、status、namespaces，具有JSON Tag，但本基线只在Bootstrap日志中消费，未发现HTTP对外响应入口。

已证实的机器约束：
- Platform Runner要求namespace为1～64字节小写ASCII字母/数字/点/下划线/连字符，不能有首尾空白；migration数量1～1000；Version非零且唯一；Description非空、TrimSpace后有效且最多255个Unicode字符；Statements至少一条，每条非空且最多1MiB。
- Platform按Version升序排序，复制Statement切片；checksum按有序Statements内容计算，逐条SQL之间加入NUL分隔；已应用Version必须对应已注册定义且历史是有序前缀，checksum不一致拒绝继续。
- Server Registry在注册时复制Description/Statements、按Version排序、拒绝重复namespace/Version，并在首次执行快照后冻结注册；namespace正则与Platform的字符集/长度规则一致。
- Server Registry当前只检查Description非空和Statements非空，没有提前执行Platform的255字符/1MiB限制；实际执行时Platform会再次拒绝。这是校验时机差异，不是绕过Runner的持久化风险，但Registry API会暂时接受稍后无法执行的定义，建议统一注册阶段验证或将其明确为Platform Runner最终权威校验。
- Platform checksum仅覆盖Statements，不覆盖Description和显式Version（Version作为独立历史主键）。因此Description变更不会触发checksum mismatch。项目契约只明确已发布版本不可修改，但尚未明确Description是否属于不可变定义。需在契约中裁决：要么Description为可修订展示元数据，要么checksum纳入Description并规划兼容迁移；本轮不擅自判定SQL checksum错误。

### 11.3 真实生产方与Server消费者

固定基线发现模块Migration生产文件共四份：
- News：`internal/news/adapters/storage/migrations.go`，返回Platform `[]Migration`，模块拥有DDL。
- Research：`internal/research/storage/migrations.go`，返回Platform `[]Migration`，声明namespace `research`。
- Researchflow：`internal/researchflow/storage/migrations.go`，返回Platform `[]Migration`，声明namespace `researchflow`。
- Notification：`internal/notification/storage/migrations.go`，返回Platform `[]Migration`，模块拥有DDL。

Server Bootstrap `internal/server/bootstrap/bootstrap.go:newMigrationRegistry` 当前只注册News、Research、Notification三个namespace；未注册Researchflow的Migration。固定基线中Researchflow已有Migration定义及独立测试文件，但组合根未将其加入Registry。这是本轮发现的直接生产方—消费者接线缺口：不能将Researchflow Migration定义存在等同于Server会执行。该问题涉及组合根注册完整性，需在维度5生命周期/启动装配核验及责任模块修复中处理；本轮仅记录证据，不修改实现。

当前未发现Quant Migration生产文件，不能据此推断Quant必须有数据库Schema；其是否拥有持久化表需以Quant模块Repository/Storage事实另行核验，不在本切片扩张。

### 11.4 Schema/Golden适用性

- Platform `SQLConfig`、`Migration` 和Server Registry注册输入均为进程内Go API，不存在对外JSON DTO；不适用HTTP JSON Schema/Golden。适用证据是Go Contract Test、Registry注册测试、模块Migration定义测试和Root Server组合根注册清单测试。
- Server `RunReport` / `NamespaceResult`虽带JSON Tag，但当前仅用于Bootstrap结构化日志，不是公开HTTP API；不能仅凭JSON Tag要求新增OpenAPI Schema。若未来被诊断API/运维接口公开，应在该边界新增独立版本化DTO、Schema和Golden，不直接暴露内部报告类型。
- Migration SQL是不可变定义输入和执行对象，不应将完整DDL作为JSON DTO或Golden；可用稳定测试验证版本、语句顺序、checksum、Namespace清单和注册快照。
- `platform_schema_migrations` 是Platform Runner内部持久化账本，不是模块Repository。其namespace/version/checksum/status/failure_statement/applied_at/started_at/finished_at/failure_code为数据库机器状态；当前没有独立schema版本Golden，是否需要升级兼容夹具应在维度5历史表升级与恢复测试核验，不在本轮重做。

### 11.5 本切片结论与登记

**结论：PLAT-CTR-006 的Platform/Server Go API边界基本明确，Migration结构不适用公开JSON Schema/Golden；已有Platform与Registry测试覆盖字段基本约束、排序、复制和历史状态。当前仍为partial / pending_evidence。**

登记缺口（并入PLAT-CTR-006，不新增CTR）：
- PLAT2-D2-009：Server Registry注册校验与Platform Runner最终校验约束不完全同阶段；需明确Description字符数和Statement 1MiB边界由谁在注册阶段验证，并补边界Contract Test。
- PLAT2-D2-010：Migration checksum不包含Description，契约尚未裁决Description是否不可变定义的一部分；需要明确语义并补测试/兼容策略。
- PLAT2-D2-011：Researchflow已导出Migration定义，但Root Server `newMigrationRegistry` 未注册Researchflow；直接消费者链路不完整。修复应由Server组合根责任模块完成，维度5检查启动生命周期，不由Platform承担领域DDL所有权。
- A2真实MariaDB/Redis集成测试以 `PLATFORM_STORAGE_INTEGRATION=1` 为门控；本轮未运行外部服务集成测试，L3资源池/迁移锁/关闭顺序证据仍待环境验收。

本轮只更新审计文档，未修改业务代码、Schema、Golden、测试，未执行CI。


## 20. PLAT-CTR-007 Typed Config 与 Secret 字段、消费者及机器证据适用性（2026-11-10）

### 20.1 本轮范围与契约边界

固定代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`。本轮只核对 Platform Config 基础 API、Server 启动配置快照、已发现的模块配置消费者、Secret 脱敏边界和机器测试适用性；不设计远程配置中心/热更新，不审查资源授权，不修改业务代码。

项目权威登记簿 `PLAT-CTR-007` 定义：Platform 提供环境变量读取、基础类型解析和 Secret 脱敏；每个服务拥有自己的 Config struct/配置 Schema；配置在启动边界装载并作为不可变快照使用；Secret 首期由部署环境变量注入；配置错误不得回显原始值；Handler、Domain、Use Case 不直接读取进程环境变量；不建设全局配置中心或热更新。

### 20.2 Platform Config 基础 API

核验文件：
- `internal/platform/config/env.go`
- `internal/platform/config/dotenv.go`
- `internal/platform/config/env_test.go`
- `internal/platform/config/dotenv_test.go`
- `internal/platform/config/secret_contract_test.go`

已证实：
- `Env` 仅持有私有 Lookup 函数；`NewEnv` 读取进程环境，`NewEnvWithLookup` 支持测试注入；nil Lookup 安全退化为空源。
- Key 仅允许 ASCII 字母/数字及 `_`、`.`、`-`；非法 Key 不会读取。
- `String/RequiredString/Bool/Int/Duration/Secret/RequiredSecret` 覆盖首批类型；解析失败只返回 Key 和期望类型，不包含原始值。
- `ValidateRequired/ValidatePositiveInt/ValidatePositiveDuration/JoinValidationErrors` 提供基础校验；范围、跨字段和领域约束由模块 Config loader 承担。
- Dotenv 支持注释、export、单/双引号和常见转义；进程已有环境变量优先；重复 Key 首项优先；先解析完整文件，再批量写入，避免语法错误导致部分加载。
- `Secret` 的 `String/GoString/fmt`、`slog.LogValue`、JSON 和 Text 输出均固定脱敏为 `[REDACTED]`；零值可安全判断；明文只能通过显式 `Value()` 访问。
- 对应单测覆盖类型解析、无效值脱敏、必填值、Secret 格式化/JSON/slog/Text、零值和 Dotenv 优先级/原子解析行为。

边界说明：
- `Secret.Value()` 是有意开放的明文出口，不能从类型层阻止调用方将返回值写入日志或普通字符串字段；因此需要同时核对消费者是否在装配边界保留 Secret 包装，及明文进入下游对象后的使用范围。
- Dotenv 只负责进程启动时环境注入，不是独立配置源优先级/热更新系统；本轮不把这些未规划能力列为缺陷。

### 20.3 Server 与模块配置消费者映射

| 消费者 | 入口/配置对象 | Secret/快照观察 | 结论 |
|---|---|---|---|
| Server | `internal/server/config.LoadProcess`、`LoadSecurity`、`LoadHTTP`、`LoadMigration`；Bootstrap先加载Dotenv，再构造Env并加载Process | Process为值快照；OwnerToken保留 `platformconfig.Secret`；HTTP/Worker角色条件加载安全和HTTP配置 | Platform API真实接入；当前Process字段主要为标量/Secret，未发现公开序列化边界 |
| News | `internal/news/config.Load` | ProviderConfig字段私有，API Key保留 `platformconfig.Secret`；Config通过值返回，外部不能直接改写私有字段 | Secret包装和配置封装较完整；配置由News自己校验Provider endpoint/key配对及URL约束 |
| Market Data HTTP Adapter | `internal/adapters/marketdata.LoadConfig` | 从Platform Env读取，但将 `Secret.Value()` 转成公开 `Config.APIKey string` | Secret类型保护在Adapter配置边界丢失；未发现本轮证据证明实际日志已泄漏，但普通格式化/序列化该Config可暴露明文 |
| Storage Adapter | `internal/adapters/storage/mysqlredis.LoadConfig` | 从Platform Env读取，但 `DBPassword/RedisPassword` 存为普通string | 同样存在Secret类型保护丢失；连接器确需明文，但应限制在构造连接选项的边界，不应让整个Config可无意格式化输出 |
| 旧 Market Data 模块配置 | `internal/marketdata/config.Load/LoadForInProcess` | 独立读取进程环境；Config含TushareToken、HithinkAPIKey、XueqiuCookie、TickFlowAPIKey、BaostockPassword、MXAPIKey、RedisPassword、MySQLDSN等普通string字段，并含map/slice | 这是旧/独立模块配置入口；是否仍有生产调用方须由Server/迁移维度按真实装配核实。本轮不将其误判为当前Root Server唯一配置路径 |
| Python API Adapter | `internal/adapters/pythonapi.ConfigFromEnv` | 直接使用 `os.LookupEnv/os.Getenv`，BearerToken/Cookie为普通string | 存在绕过Platform Env抽象的兼容配置入口；当前调用方与部署启用状态未在本切片确认，列为接入边界待核，不据此断言活跃生产路径违规 |

Research、Researchflow、Quant、LLM、Notification目录中，本轮固定基线文件树未发现与上述相同形式的独立进程环境 Config loader。Notification的ChannelConfigSnapshot属于运行期持久化的领域配置版本，不等同于PLAT-CTR-007启动环境配置；不将其混入本轮。

### 20.4 不可变快照与校验责任

- Server Process和News Config均在Load时解析并返回值对象；Server Process字段为标量/Secret，News对外字段私有，未发现调用方能通过原始环境变化回写已加载配置的路径。
- Market Data/Storage Adapter Config公开字段中存在普通Secret字符串；旧Market Data Config另含map/slice。对这些对象是否深拷贝及是否允许调用方修改，应由各自模块配置契约负责。本轮不把“Go struct可赋值”误判为热更新，也不将其等同于不可变保证。
- Server `LoadProcess` 的分析超时、Worker/Queue、Role、Migration、Security、HTTP配置均在启动阶段解析；Bootstrap随后基于同一个Env加载Storage和Market Data Adapter配置，未见Handler/Domain通过Env直接读取配置的证据。
- News Loader对Credential Provider要求Endpoint与Secret成对出现；SearXNG仅要求合法Endpoint。Server/Storage/Market Data Adapter各自实施范围校验。不同配置项的必填性和默认值由对应模块所有，不应搬到Platform统一定义。
- `internal/marketdata/config` 与 `internal/adapters/pythonapi/config` 的独立环境读取入口需在后续Server/迁移维度核实真实调用路径；本轮只登记入口事实，不扩大为全仓环境变量静态扫描。

### 20.5 Schema、Golden 与机器测试适用性

- Platform `Env`、`Secret`、Server Process Config及模块启动 Config均为进程内Go配置API，不是HTTP请求/响应或跨模块JSON DTO；不适用公开JSON Schema/OpenAPI，也不应为Secret明文创建Golden。
- 对应机器证据应采用：Env类型/错误脱敏Contract Test；Secret所有输出路径脱敏测试；模块Config Loader默认值、必填、范围、跨字段校验测试；配置快照隔离测试；真实Bootstrap配置注入测试。
- 若某模块未来将配置快照持久化或通过管理API导入/导出，则该持久化/HTTP边界应另设版本化DTO、Schema和Golden；不能直接序列化当前内部Config结构。
- 当前没有发现Platform Config本身缺少公开Schema/Golden的缺陷；缺少模块独立配置Schema文件不等于缺少公共HTTP契约，因为契约要求的配置Schema属于模块内部配置所有权，是否需要机器可读文件需结合模块自身配置复杂度与测试决定。

### 20.6 差异与关闭条件

| 编号 | 发现 | 影响与性质 | 关闭所需证据 |
|---|---|---|---|
| PLAT2-D2-012 | Market Data HTTP Adapter与Storage Adapter从 `Secret.Value()` 读取后，将凭证保存为普通公开Config字符串字段；旧Market Data配置也大量使用普通字符串承载Token/Password/DSN | Secret的显式取值本身合理，但类型级脱敏保证在配置对象边界消失。尚无证据证明当前生产日志实际泄漏；属于消费者安全封装/误记录风险，不直接宣称已发生泄漏 | 责任模块逐项确认明文必须暴露给哪些驱动/Provider构造点；尽可能在Config内保留Secret或提供窄化凭证访问方法；对Config的String/GoString/fmt/JSON/slog输出补不泄漏测试，或证明对象不会被格式化/序列化且限制访问边界；禁止记录完整Config |
| PLAT2-D2-013 | `internal/adapters/pythonapi.ConfigFromEnv` 直接访问进程环境，绕过Platform Env；旧Market Data模块也有独立os环境读取入口 | 当前调用方和活跃部署状态未确认，不能将历史/兼容入口直接判为线上违规；属于生产接入边界证据缺口 | Server/迁移维度核实实际构造调用点、部署入口和是否仍启用；活跃入口应在Bootstrap边界使用Platform Env，非活跃入口明确Deprecated/移除计划 |
| PLAT2-D2-014 | Adapter Config的Secret脱敏、配置快照可变性及配置对象输出策略缺少统一的消费者级Contract Test证据 | Platform Secret测试只证明Secret本体，不覆盖已转成string的下游配置对象；不重复新增独立CTR | 对实际活跃消费者补Config输出脱敏、快照/切片Map隔离及关键默认/边界测试；未活跃兼容入口记录不适用证据 |

以上差异均并入既有PLAT-CTR-007及本矩阵，不新增契约编号。PLAT2-D2-013的生产影响需在Server/迁移维度依据真实调用点进一步定性，不能在本切片越权裁决。

### 20.7 本切片结论

**PLAT-CTR-007 Platform基础解析与Secret本体脱敏实现基本符合设计；Server与News接入较完整；Market Data/Storage Adapter存在Secret包装丢失，旧/兼容配置入口仍需确认生产使用范围。整体保持 `pending_evidence`，不满足冻结条件。**

- 未发现需要新增公开JSON Schema/Golden的Platform Config DTO。
- 已发现的差异归入PLAT-CTR-007，不新增CTR。
- 本轮未修改业务实现、Schema、Golden或测试，未执行CI。
- Platform维度2的PLAT-CTR-001～007已完成首轮机器契约适用性切片；各契约仍保留各自开放证据，不能据此宣称Platform整体契约冻结。
- 下一步回到阶段B十模块六维核验，进入 **Server — 维度2：DTO、Schema、Golden**，先审Server对外HTTP DTO与真实Handler/OpenAPI/Golden消费者映射；不重做Platform维度2，不扩展到Server其他五个维度。
