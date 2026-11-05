# 平台整体 Review 与首轮安全审计记录

- 仓库：Kasign/stock_data_provider
- 分支：feat/platform-foundation
- 审查基线：7a292199e6b4e55fceed60647ecaf7fd342270ca
- 日期：2026-11-05
- 范围：PLAT-003、PLAT-005、PLAT-006、PLAT-007、PLAT-008、PLAT-009 公共实现；不包含其他分支的生产部署和渗透测试。

## 1. 结论摘要

本轮完成静态代码与契约审查。此前发现的 Security Router 根级通配符缺口已在审查基线修复，并通过平台专项 CI。当前没有基于本轮静态证据需要继续扩大平台公共抽象的理由。

本记录不是生产安全验收或渗透测试报告。服务级路由、配置装载、日志链路、领域诊断 Adapter、真实网络出口和真实数据库仍需由对应集成方验收。

## 2. 逐项审查结果

### 2.1 PLAT-003 身份认证与路由授权

已确认：
- owner 与 service 凭证独立；service Scope 白名单校验，禁止 service 获得 owner 专属 admin Scope。
- 凭证在构造时转为 SHA-256 摘要；认证遍历所有配置摘要并调用常量时间比较。
- Principal 从 Bearer 凭证生成并进入请求 Context，不信任客户端身份 Header。
- 缺失/无效凭证为 401，身份或 Scope 不满足为 403；未注册路由保持 404。
- RoutePolicy 不允许零值策略注册。
- 根级 /、host 根路径和 /{path...} 类全路径兜底模式被拒绝；精确根路由可使用 /{$}。

本轮修复：
- 提交 7a292199e6b4e55fceed60647ecaf7fd342270ca 扩展 isRootCatchAllPattern，补充 method/host 组合形式的回归测试。
- Platform Go checks：Actions 42028080673 success；gofmt、平台包单测、race test、vet 全部通过。

剩余风险/验收：
- 最小 32 字节长度不能证明 Token 具备 256 bit 随机熵；部署 Secret 生成流程必须使用密码学安全随机数。
- Authenticator 是进程内不可变快照，轮换需要协调调用方与服务重启；不支持即时撤销和新旧凭证并存。
- 尚未看到所有实际服务路由统一接入 Security Router 的证据，不能宣称系统已开启默认拒绝。
- 需在部署环境验证旧 Token 失效、Gateway 入口隔离和绕过 Gateway 直连策略。

### 2.2 PLAT-005 Config / Secret

已确认：
- Env 的类型解析错误仅包含配置键和预期类型，不回显输入值。
- Secret 的 fmt、slog、JSON/Text 序列化输出脱敏标记。
- dotenv 先完整解析校验，再写入环境变量；进程环境优先，非法文件不会产生部分写入。

剩余风险/验收：
- Secret.Value() 是有意开放的显式读取边界，调用方仍可能把返回的普通 string 写入日志或错误；需要集成层审查调用点。
- 现有 Analysis/Adapter 仍存在独立环境变量读取和普通 string Secret 字段，属于服务集成整改，不在 Platform 分支修改。
- 需在真实启动入口验证配置错误时启动失败、错误不回显值，且业务层不再直接读取环境变量。

### 2.3 PLAT-006 Request ID / Fault

已确认：
- Request ID 作为 Context 元数据传播，不承担认证、授权或主体识别。
- Fault.Cause 不参与 JSON 序列化；平台错误使用稳定 Code 和固定 SafeMessage。
- 已有文档明确公网 Gateway 信任边界、异步 correlation_id 与 request_id 的区分。

剩余风险/验收：
- 平台原语不自动清洗调用方提供的 Code、SafeMessage、Dependency 或普通日志字符串；领域 Adapter 必须按白名单构造。
- 历史 Analysis HTTP transport 仍有私有 Request ID 和底层错误直接返回风险；需由 Server/Analysis 集成分支整改。
- 需验证 Access Log 不记录 Query、Body、凭据、原始路径参数和未经白名单处理的错误。

### 2.4 PLAT-007 Diagnostics

已确认：
- Runner 限制单次 Check 数量、最大并发、单项 Timeout 和总 Deadline。
- 单项失败不会阻断其他检查；required skipped 转 failed；optional skipped 不降低总体状态。
- Check panic payload 不进入 Fault.Cause、Report 或普通日志。
- Observation 限制字段长度和 Attributes 总量。

剩余风险/验收：
- Runner 无法强制终止忽略 Context 的 Check，领域实现必须遵守取消约定。
- Observation.Validate 只校验结构和大小，不负责识别任意业务字符串中的 Secret；领域 Adapter 必须按字段白名单构造 Attributes、Dependency、SafeMessage 和 EvidenceRef。
- 尚无真实领域 Adapter 和受保护的诊断 HTTP 路由接入证据。
- 不将 Data 的 Provider Probe、质量维度和历史记录迁入 Platform。

### 2.5 PLAT-008 安全 HTTP Client

已确认：
- 每次新建连接前解析并校验 DNS 结果，实际连接使用校验后的 IP。
- 私网、环回、链路本地、特殊用途和混合公网/私网 DNS 结果被拒绝。
- 默认关闭环境代理；默认禁止重定向；显式启用后限制同源跳转、跳转次数和 HTTPS 降级。
- 限制请求总时长、响应体大小、依赖并发和幂等重试。

剩余风险/验收：
- DNS/IP 校验发生在新建连接时；复用已经建立的连接不会为每个请求重新解析 DNS。此边界已在接入清单明确。
- 特殊用途地址清单需要随 Go 运行时和 IANA 注册表变化复核；本轮静态检查不替代生产网络出口验证。
- Provider endpoint allowlist 不等于 News 来源 URL 授权；必须由 News 领域单独实施来源授权。
- 尚未完成真实部署 DNS、出口网络策略和公网 SSRF 演练。

### 2.6 PLAT-009 Storage

已确认：
- SQL/Redis 生命周期原语有连接池边界、Ping Deadline 和失败关闭行为。
- Redis MaxActiveConns 有显式硬上限；该上限是每个 Redis Client 的限制，不是进程内所有 Client 的全局总量。
- Migration 使用 namespace/version、checksum、GET_LOCK 和失败恢复门禁；失败不保存 Driver 原始错误文本。

剩余风险/验收：
- MariaDB 历史表新增列使用 ADD COLUMN IF NOT EXISTS，必须在目标 MariaDB 版本执行真实升级验证。
- fake Driver 测试不能代替真实 MariaDB DDL 行为；需执行旧表升级、重复执行和失败恢复验证。
- 需在真实 Redis 环境验证连接池活动连接上限和压力行为。
- 历史 Analysis Store 尚未迁移到平台存储原语，需由服务端集成方接线。

## 3. 平台契约与模块边界

本轮未发现 Diagnostics、Security、HTTP Client、Storage 等基础原语之间的循环依赖。已知历史领域例外仍是：
- internal/platform/httpapi：Analysis HTTP transport。
- internal/platform/taskqueue：Analysis Task/Worker 和 Analysis 专属表。

二者不得被视为已验收的平台公共能力。Platform 分支不移动领域 Handler/Worker，也不修改服务启动装配。

## 4. 当前风险分级

| 等级 | 风险 | 责任边界 |
|---|---|---|
| 高 | 实际服务路由尚未统一接入认证，包级认证测试不能证明生产 API 受保护 | Server/集成方 |
| 高 | 生产网络出口、DNS 和 News 来源授权尚未完成 SSRF 验收 | 部署/News |
| 中 | Config/Secret、Request ID、Fault 尚未贯通实际入口和后台任务 | Server/领域集成方 |
| 中 | 真实 MariaDB 旧表升级与 Redis 连接池验证未完成 | Server/部署 |
| 中 | 领域 Diagnostics Adapter 和受保护诊断路由尚未接入 | Data/News/Research |
| 低 | 32 字节 Token 长度校验不能证明随机熵 | Secret 生成/部署流程 |

## 5. 阶段性决策

1. 暂停扩展平台公共原语，不新增通用 IAM、配置中心、Metrics 平台、诊断历史库或通用 Task Queue。
2. PLAT-003/005/006/007 保持“原语完成、服务接入待验收”的状态。
3. PLAT-008 保持“受控测试通过、生产网络与领域授权待验收”。
4. PLAT-009 保持“平台实现和专项 CI 通过、真实 DB/Redis 验证待验收”。
5. PLAT-012 首轮静态审查完成，但凭证轮换、入口隔离、日志泄漏和部署网络证据未齐，不能标记完成。
6. 下一阶段优先由 Server/集成分支接通 bootstrap、认证路由、Request ID、Fault 和存储生命周期；Platform 分支只处理审查中发现的公共原语缺陷。
7. PLAT-014 故障演练应在上述服务接入完成后执行，避免对尚未接入的抽象做无效演练。

## 6. 验收证据清单

- [ ] 所有服务路由访问矩阵与实际注册表一致。
- [ ] 凭证由安全随机数生成；完成旧 Token 失效和轮换演练。
- [ ] 公网 Gateway 覆盖 Request ID，绕过 Gateway 直连受到网络或应用层限制。
- [ ] 错误响应、访问日志、诊断 JSON 不包含 Token、DSN、SQL、URL Query、原始上游响应。
- [ ] Diagnostics 领域 Adapter 保留领域质量维度，并通过受保护路由访问。
- [ ] News Provider endpoint 与来源授权分离；生产 DNS/出口 SSRF 演练通过。
- [ ] 目标 MariaDB 旧历史表升级、Migration 重复执行和失败恢复验证通过。
- [ ] 真实 Redis 连接池活动连接上限和压力验证通过。
- [ ] 对应提交 SHA、自动化 CI 和部署环境记录可追溯。

本记录只证明静态审查覆盖到的代码与文档事实，不替代上述未完成的运行验收。


## 7. 第二轮安全审计：Analysis HTTP 错误与日志泄漏路径

审查对象为当前分支中的历史 Analysis HTTP 实现。以下是源码中可直接确认的风险点，不代表 Platform 公共包自身存在相同缺陷。责任归属为 Server/Analysis 集成方；Platform 分支只记录证据，不修改领域 Handler。

### 7.1 高风险：内部错误文本直接进入 HTTP 响应

**internal/platform/httpapi/server.go**：
- handleReady 将 s.store.Health(ctx) 的原始错误传给 writeError，HTTP 503 的 message 直接使用 err.Error()。
- handleCreateRun 将 Runner 的 err.Error() 同时写入日志和 HTTP 502 响应。
- handleGetRun 的 events/result 查询失败分支将 Store 错误直接写入 HTTP 500 响应。
- writeJSON 序列化失败时记录原始序列化错误。

风险：底层驱动、上游服务或领域错误可能包含 DSN、主机名、SQL、文件路径、上游响应片段或其他内部实现细节。当前代码没有在这些边界统一转换为 Fault 的稳定 Code 和 SafeMessage。

整改要求：
1. 面向客户端只返回固定安全消息和稳定错误码；详细错误仅进入受控内部诊断，并且先做字段级脱敏。
2. 对 errors.Is / errors.As 的已知错误进行显式映射；未知错误统一映射 internal。
3. Readiness 只返回依赖标识和固定状态，不返回底层错误字符串。
4. 日志记录错误类型、稳定错误码、request_id 和依赖名；不得以截断 err.Error() 代替脱敏。

### 7.2 中高风险：启动与关闭日志记录原始错误

**cmd/analysis-api/main.go**：
- 数据库/缓存关闭失败、HTTP 优雅关闭失败、HTTP 停止错误均直接记录 err.Error()。
- logStartupError 也直接记录传入错误文本。
- 启动日志输出 market_data_base_url；该 URL 若意外包含用户信息或敏感 Query，可能进入日志。

整改要求：
- 先将启动配置错误映射为配置键、错误类别和固定描述。
- URL 日志只允许记录规范化后的 scheme/host/port，不记录 userinfo、query、fragment、path 中的凭证。
- Driver 错误仅保留内部受控诊断，禁止直接输出原始文本。

### 7.3 中风险：Adapter Secret 仍使用普通 string

已确认字段：
- **internal/adapters/storage/mysqlredis/config.go**：DBPassword、RedisPassword。
- **internal/adapters/pythonapi/config.go**：BearerToken、Cookie。
- **internal/adapters/marketdata/config.go**：APIKey。

这些配置由 Adapter 自行调用 os.Getenv / os.LookupEnv 读取。普通 string 本身不必然造成泄漏，但无法获得平台 config.Secret 的格式化脱敏保护，也绕开了统一 bootstrap 配置边界。

整改要求：
- 由服务 bootstrap 统一读取配置并注入 Adapter Config。
- Secret 字段使用 config.Secret 或等价的不可意外格式化类型；仅在构造连接/请求认证头时显式取值。
- 禁止将 Cookie、BearerToken、APIKey、DB/Redis 密码写入日志、错误或诊断 Attributes。

### 7.4 中风险：历史 Request ID Middleware 与平台契约不一致

**internal/platform/httpapi/server.go** 的 requestIDMiddleware 使用 Header.Get("X-Request-ID")，没有像平台 observability.Middleware 一样拒绝重复 Header 值。重复值可能导致代理、日志和应用对同一请求关联 ID 的解释不一致。

整改要求：
- 服务入口统一使用 observability.Middleware，不保留第二套 Request ID Context。
- 信任边界由公网 Gateway 定义；客户端传入 ID 只作为关联标识，不作为身份或授权依据。
- 验证重复 Header、非法字符、超长值和跨服务传播行为。

### 7.5 本轮优先级与验收门禁

| 优先级 | 整改项 | 责任方 | 验收方式 |
|---|---|---|---|
| P1 | HTTP 错误响应不暴露底层错误 | Server/Analysis | 注入包含 DSN、SQL、Token 的错误，响应只含固定消息和稳定 Code |
| P1 | 启停日志不输出原始错误和敏感 URL | Server | 日志捕获测试与部署日志抽查 |
| P1 | Adapter Secret 类型和配置注入统一 | Server/Adapter | 配置快照、格式化、日志和 JSON 脱敏测试 |
| P2 | Request ID Middleware 收敛到平台实现 | Server | 重复 Header 与跨服务传播测试 |

以上属于当前代码可复现的静态风险，优先级高于新增平台抽象。修复应在 Server/Analysis 集成分支完成；Platform 分支不越界修改领域 Handler、Adapter 或启动装配。
