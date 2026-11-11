# PLAT-CTR-007 Typed Config 与 Secret 生命周期契约

- 契约编号：`PLAT-CTR-007`
- 契约版本：`1.0.0`
- 状态：`freeze-ready`
- Platform 责任：环境变量读取、基础类型解析、Secret 脱敏包装和基础值校验辅助
- 模块责任：定义自身 Config struct、默认值、字段/跨字段约束、业务配置语义和配置装载器
- Server 责任：在 Bootstrap 边界装载各模块配置、注入依赖并控制启动失败/重启生效
- 唯一权威 API：`internal/platform/config`
- 设计依据：`doc/platform/08-公共平台契约规范.md` 第6节
- 代码核对基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`

## 1. 目标与边界

Platform 提供最小的、强类型的配置读取与 Secret 防泄漏原语，不定义全局业务配置模型。

- 每个服务/模块拥有自己的 Config struct 和配置 Schema。
- Platform 只提供 Env 基础读取、类型解析、Secret 包装和基础校验；不决定业务配置名称、默认值、范围、组合关系或业务含义。
- Server/模块 Bootstrap 负责读取配置、组合默认值、执行字段及跨字段校验，并在配置非法时终止启动。
- 环境变量只允许在 Bootstrap/config 装载边界读取；Handler、Domain、Application Use Case 不得直接读取进程环境变量。
- 配置完成装载后作为不可变快照传递；Platform 不提供全局 Config、热更新、远程配置中心或自动 Secret 轮换。
- Secret 的明文只允许在构造需要凭证的下游依赖时通过显式 Value() 读取；不得日志、错误回显或序列化泄漏。

## 2. 公共 Go API

```go
type Env struct { /* lookup source is private */ }

func NewEnv() Env
func NewEnvWithLookup(lookup func(string) (string, bool)) Env

func (e Env) String(key, fallback string) string
func (e Env) RequiredString(key string) (string, error)
func (e Env) Bool(key string, fallback bool) (bool, error)
func (e Env) Int(key string, fallback int) (int, error)
func (e Env) Duration(key string, fallback time.Duration) (time.Duration, error)
func (e Env) Secret(key string) Secret
func (e Env) RequiredSecret(key string) (Secret, error)

type Secret struct { /* value is private */ }

func (s Secret) Value() string
func (s Secret) Configured() bool
func (s Secret) String() string
func (s Secret) GoString() string
func (s Secret) Format(state fmt.State, verb rune)
func (s Secret) LogValue() slog.Value
func (s Secret) MarshalJSON() ([]byte, error)
func (s Secret) MarshalText() ([]byte, error)
func (s Secret) IsZero() bool

func ValidateRequired(key, value string) error
func ValidatePositiveDuration(key string, value time.Duration) error
func ValidatePositiveInt(key string, value int) error
func JoinValidationErrors(errs ...error) error

func LoadDotEnvIfExists(path string) error
```

`Env.lookup` 和 `Secret.value` 为私有字段；调用方不能通过结构体字段直接读取原始环境源或 Secret 明文。

## 3. Env 读取语义

### 3.1 环境源与 Key

- `NewEnv()` 使用进程 `os.LookupEnv` 作为读取源。
- `NewEnvWithLookup` 接受显式 Lookup；nil Lookup 视为始终未配置。该入口用于测试或受控 Bootstrap 数据源，不得用于让业务层绕过统一配置装载。
- Key 必须为非空 ASCII 字母、数字、下划线、点或连字符；非法 Key 在底层按未配置处理。
- Env 不暴露公开原始 Lookup 方法；敏感 Key 必须使用 Secret/RequiredSecret 读取。

### 3.2 字符串与必填项

- `String(key, fallback)`：Key 未配置时返回 fallback；Key 已配置时返回原始字符串，即使显式配置为空字符串也不使用 fallback。
- `RequiredString(key)`：Key 必须存在且 TrimSpace 后非空；成功返回原始值，不自动裁剪内容；失败只报告 Key 名称，不包含配置值。
- 调用方若要求规范化空白、URL、标识符或其他业务格式，必须在模块 Config.Validate 中完成，不由 Env 隐式改写。

### 3.3 基础类型解析

- `Bool`：对配置值 TrimSpace 后使用 Go `strconv.ParseBool` 解析；缺失时返回 fallback,nil；解析失败返回 fallback 和只包含 Key/期望类型的错误。
- `Int`：对配置值 TrimSpace 后使用 Go `strconv.Atoi`（十进制整数）解析；缺失时返回 fallback,nil；解析失败返回 fallback 和类型错误。数值范围、正负和业务含义由模块校验。
- `Duration`：对配置值 TrimSpace 后使用 Go `time.ParseDuration` 解析；缺失时返回 fallback,nil；解析失败返回 fallback 和类型错误。
- 解析失败不得将原始配置值写入错误消息。
- 基础解析器不替模块执行字段范围、跨字段一致性、枚举或业务语义校验。

## 4. Secret 契约

### 4.1 读取与访问

- `Secret(key)` 读取可选 Secret；缺失时返回零值 Secret。
- `RequiredSecret(key)` 要求 Key 存在且 TrimSpace 后非空；失败只报告 Key 名称。
- `Configured()` 与 `IsZero()` 只判断 Secret 是否配置；纯空白字符串视为未配置。
- `Value()` 是唯一明文访问入口；只允许在构造需要认证/签名的下游客户端或协议头时显式调用。
- 调用方不得将 Value() 放入日志、Fault、错误消息、诊断 attributes、HTTP 响应、普通 Config dump 或持久化明文记录。

### 4.2 强制脱敏

Secret 的以下行为必须始终返回脱敏标记 `[REDACTED]`，不受 Secret 是否配置、格式化 verb 或序列化方式影响：
- String()；
- GoString()，包括 `%#v`；
- Format()，覆盖 fmt 格式化入口；
- slog.LogValue()；
- MarshalJSON()；
- MarshalText()。

不得通过 fmt、slog、JSON、Text 或常见结构化日志格式意外暴露 Secret.value。

### 4.3 内存与轮换边界

- Secret 是防止意外输出的类型封装，不是加密内存容器；Go string 的底层内存不承诺可可靠零化。
- Platform 不提供 Secret 版本管理、远程 Secret Manager、动态热轮换或旧值自动撤销。
- 首期 Secret 由受控部署环境注入；轮换通过更新部署 Secret 并重启/重建持有该配置快照的服务实例生效。
- 需要在线轮换、双密钥过渡或即时撤销的具体业务能力，必须由相应领域/Server 单独定义，不得假设 Secret 类型自动提供这些能力。

## 5. DotEnv 辅助加载语义

`LoadDotEnvIfExists(path)` 是启动前的本地环境辅助工具，不是业务配置中心：
- 文件不存在时返回 nil；其他打开/读取错误返回错误。
- 支持空行、整行注释、可选 `export ` 前缀、单引号/双引号和行内注释。
- 双引号内容支持 `\n`、`\t`、`\"` 转义；不支持变量插值、命令替换或递归文件加载。
- 先解析并验证完整文件，再修改进程环境；非法条目不得导致解析到一半就开始写入环境。
- 已存在的进程环境变量优先于文件值；同一文件中重复 Key 以第一次出现为准。
- 文件中的 Key 使用与 Env 相同的字符集规则；非法行、未闭合引号或 NUL 值必须报错并指出行号，不得回显 Secret 值。
- 生产 Secret 的权威来源仍是受控部署注入；不得把提交到仓库的 .env 文件当作 Secret 管理机制。

## 6. 基础校验辅助

- `ValidateRequired(key, value)`：TrimSpace 后非空。
- `ValidatePositiveDuration(key, value)`：duration 必须大于 0。
- `ValidatePositiveInt(key, value)`：整数必须大于 0。
- 校验错误只包含 Key 名称和期望条件，不包含实际值。
- `JoinValidationErrors` 使用 `errors.Join` 合并独立错误，保留 errors.Is/errors.As 检查能力。
- 上述函数不替代模块 Config.Validate；复杂范围、依赖关系、条件必填和安全策略由模块定义。

## 7. 配置加载、不可变快照与生命周期

- Bootstrap 在进程启动阶段创建 Env 并读取配置；各模块将解析结果组装为自己的 Config struct。
- 默认值、必填项、字段范围、跨字段校验、环境分层和依赖构造顺序由模块/Server 负责。
- 配置完成校验后作为不可变快照注入 Application、Adapter、Worker 和 HTTP Handler；业务执行期间不得再次读取进程环境变量。
- 缺少必填配置、解析失败或 Validate 失败时，Bootstrap 必须终止启动，不得用空值、静默默认值或内存 fallback 掩盖配置错误。
- Secret 的明文读取必须局限在下游依赖构造边界；构造后的依赖负责自身凭证使用，不得将 Secret 明文反向写入普通 Config、日志或诊断。
- 配置/Secret 更新在首期通过服务实例重建生效；Platform 不提供热更新一致性或并发切换语义。

## 8. 明确不属于 PLAT-CTR-007

- 各模块 Config struct、环境变量名称清单、业务默认值、字段范围和跨字段校验。
- Server 各环境部署文件、Secret 存储后端、密钥轮换流程、可信代理配置和生产部署权限。
- 动态配置中心、热更新、Secret Manager、自动轮换、版本双写/撤销和密钥生命周期编排。
- 业务凭证、Provider API Key、Owner 密码或 Session Token 的领域语义；这些分别归对应领域/PLAT-CTR-003。

## 9. 版本与兼容

- 契约版本：1.0.0。
- 本契约为纯 InProcess Go API；JSON Schema 不适用，强类型 Go API 与本规则文件为唯一权威定义。
- 增加不改变既有解析/脱敏语义的可选辅助能力可按 GOV-CTR-004 递增 MINOR。
- 改变空值/fallback 语义、Key 字符集、Secret 脱敏覆盖、Value 访问边界、DotEnv 优先级或配置快照/轮换语义属于破坏性变更，必须递增 MAJOR 并先完成项目级变更审批。
