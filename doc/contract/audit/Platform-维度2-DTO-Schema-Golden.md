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

Platform维度2的主要独立公共DTO/调用API切片已核完。下一步不继续扩展Platform维度2；回到阶段B，转入Server维度2：HTTP公开错误Envelope与Diagnostics/Health响应DTO。Server切片应核路由实际输出、Fault到HTTP状态码/公开错误体映射、Request ID响应Header、Schema/Golden/消费者证据；不要重审Platform Fault内部字段、Observation DTO或HTTP Client底层实现。