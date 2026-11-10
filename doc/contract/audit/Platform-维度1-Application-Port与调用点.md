# Platform 维度 1：Application Port 与真实调用点

## 1. 核验范围

- 代码基线：`origin/integration`，`b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 审计文档分支：`feat/design_doc`
- 本批范围：Platform 对外技术能力的接口形态、依赖方向，以及已确认的 Platform 内部反向依赖。
- 本批不宣称已完成全仓消费者调用点扫描；未检查的消费者不得视为不存在。

## 2. 契约判断

《公共平台最小契约规范》明确 Platform 只提供公共技术原语，不承载领域业务语义。依赖方向为：

```
cmd / transport / bootstrap
   -> platform/*
   -> application -> domain / ports
adapters -> ports
```

因此，Platform 不需要也不应该为 News、Research、Researchflow、Quant、LLM、Notification 等领域定义统一 Application Port。领域 Application Port 应由领域模块定义，Platform 仅提供可被组合和注入的技术能力。

## 3. 已核验的实现证据

| 能力 | 代码证据 | 已确认的接口形态 | 判断 |
|---|---|---|---|
| HTTP Client | `internal/platform/httpclient/client.go`（blob `5e62857b6f85fe0f8e88fd5e5e07dde06a2c4508`） | `Resolver` 标准 Go interface；`Config`、`DestinationPolicy` 配置结构；`New(Config)` 构造 `*Client`；`Client.Do` 接受请求和显式 DestinationPolicy | 技术能力通过标准 Go 类型和构造函数注入，不是领域 Application Port |
| HTTP Client 适配 | `internal/platform/httpclient/bound_doer.go`（blob `ae140df0b83330e86a7965e883d3e659a626b9fb`） | `Client.Bind(policy)` 返回窄接口适配器 `BoundDoer`，提供 `Do(*http.Request)` 和 `OpenStream` | 固定目标策略，避免消费者在单次调用时漏传或替换策略 |
| Diagnostics | `internal/platform/diagnostics/check.go`（blob `33f263b258a931e3d43a5b9d25ea3bdc83dddda2`） | `CheckFunc` 回调和 `Check` 描述独立诊断项；领域方提供执行函数 | 领域诊断逻辑由调用方提供，Platform 负责执行约束 |
| Diagnostics Runner | `internal/platform/diagnostics/runner.go`（blob `b93f6ef40f525f86f29e29eb9250e36d2d890cdf`） | `Runner.Run(ctx, checks)` | 执行器是通用技术原语，不替代领域诊断服务 |
| Request ID | `internal/platform/observability/request_id.go`（blob `b9e2d9ddc298ba23bead6ed1fcc9d6ec58ef707b`） | HTTP Middleware、Context 读写及出站传播函数 | 作为边界中间件/Context 原语使用，不是领域 Port |

## 4. 已确认的依赖边界问题

### AUD-PLAT-PORT-001：Platform Task Queue 反向依赖旧 Analysis Domain

<badge color="warning">待裁决 · P1 候选</badge>

证据：

- `internal/platform/taskqueue/queue.go`（blob `d438d934c6267ee1d7a14d482d37741cf3d1c795`）直接 import `example.com/stock-analysis-flow/internal/analysis/domain`。
- `Runner` 接口签名使用 `domain.RunRequest` 和 `domain.RunResult`。
- `Event` 暴露 `domain.StageEvent`。
- `Queue.Create` 直接接收 `domain.RunRequest`。
- 对应测试 `internal/platform/taskqueue/queue_test.go`（blob `6ff46bc8410cd72f1adb0077b21dc257f0a5583e`）同样直接依赖旧 Analysis Domain。

判断：

该实现不是领域无关的通用 Task Port，而是旧 Analysis Worker/Queue 的领域实现放置在 Platform 路径下，与公共契约“Platform 不依赖领域包”的方向相反。契约第 1、11 节明确禁止 Platform import 领域模块，并要求领域拥有自己的长任务语义。

处理建议：

1. 不在本次审计文档分支直接修改业务实现。
2. 由旧 Analysis 迁移/Server 责任方确认该 Queue 的最终归属。
3. 若保留为领域实现，应迁出 Platform 命名空间并由对应领域拥有；若抽取通用能力，抽象边界必须不引用领域 DTO/事件/Repository。
4. 在迁移完成前，该项不得作为 Platform 依赖边界通过证据。

## 5. 消费者调用点核验状态

本批已确认 Platform API 的接口形态，但尚未完成所有消费者的生产代码扫描。以下事项保持待核：

- 根 Module 内各模块是否通过构造函数注入 Platform 能力，还是在业务层直接创建/持有技术客户端。
- Server Bootstrap 是否为 HTTP Client、Storage、Diagnostics、Request ID 提供唯一装配入口。
- Data 独立 Module 与迁入根 Module 的边界是否遵守契约；独立 Module 不得跨 Module import 根模块 `internal`。
- 是否存在消费者绕过 `BoundDoer`、直接调用底层 HTTP Client 或重复实现相同基础能力。
- Platform 其他包是否存在未发现的领域 import；本批仅以 taskqueue 的明确证据登记，不代表全量 import 扫描已经完成。

由于 GitHub 代码搜索未返回可用的基线消费者结果，本批不将其作为全仓无调用点的证据。后续需按固定 SHA 对 Server 和各领域消费者逐个追踪真实构造点、注入点和调用点。

## 6. 本批结论

- Platform 不需要定义统一领域 Application Port；现有技术能力采用标准 Go interface、配置结构、构造函数和回调注入，方向符合契约设计。
- `internal/platform/taskqueue` 对旧 `internal/analysis/domain` 的反向依赖是已证实的边界问题，登记为 AUD-PLAT-PORT-001，等待责任模块裁决。
- 消费者追踪、Server 装配入口、Data 独立 Module 边界和全量 Platform import 扫描尚未完成。

**维度状态：partial。不得标记为 verified。**
