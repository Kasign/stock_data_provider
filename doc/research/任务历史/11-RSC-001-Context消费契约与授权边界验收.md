# RSC-001：Context 消费契约与授权边界验收

- 日期：2026-11-18
- 分支：`feat/research`
- 代码范围：`internal/research/application/application_test.go`
- 代码验收 SHA：`31a5e0f7cbab3b0cefa39dd9789a7220e598d386`
- CI Run：[44286491873](https://github.com/Kasign/stock_data_provider/actions/runs/44286491873)

## 1. 本轮结论

Research 模块的 Context Application Port 消费契约测试已补齐，Research 专项格式、单测、race、vet 和 MariaDB Repository/Migration 集成 Job 均通过。RSC-001 在 Research 模块职责范围内完成。

这不等于真实生产授权链完成：Server 仍须在认证后从可信 Principal 注入 CallerRef，并装配真实 Context Owner Authorizer。当前没有生产装配证据，因此不标记 L3 或生产授权验收通过。

## 2. 既有实现复核

本轮没有重复实现已有能力，确认现有实现已具备：

- `ContextRef` 只定位不可变 Context ID/Version，不表达授权。
- `ContextReadRequest` 将 CallerRef、PurposeCode 与资源 Ref 分开传入。
- `GetContext` 校验 Context ID、正版本、CallerRef 和允许的 Purpose。
- `GetContext` 读取指定版本后复核存储记录身份与 Domain 不变量，再调用 ContextAuthorizer。
- 未装配 ContextAuthorizer 时 fail-closed；该门禁不会阻断 CreateContext 等不需要读取授权的其他能力。
- 明确拒绝、依赖不可用、超时和未知错误映射为不同稳定 Fault 语义。

因此本轮只增加 Application Port 消费契约测试，没有改动生产 Application 实现或公开接口。

## 3. 新增测试覆盖

- 精确读取 Context ID + Version；Repository 和 Authorizer 均收到请求的同一不可变版本。
- 授权器收到原样 CallerRef、PurposeCode、Scope 和 SubjectRefs。
- 三种已允许用途均可通过：`research_analysis`、`risk_review`、`user_display`。
- 缺少/空白 CallerRef、不支持的 Purpose、缺少 ContextVersion，在访问 Repository/Authorizer 前被拒绝。
- 无 ContextAuthorizer 装配时，GetContext 在 Repository 读取前 fail-closed。
- 授权结果为 allowed 时返回指定 Context；denied、unknown、unavailable、timeout 和未分类故障均不返回 Context。
- 未知授权错误按内部故障拒绝，不降级为允许；依赖不可用和超时保留各自错误类别。

这些测试使用 Fake Repository/Authorizer，属于模块 Application Port 契约测试，不冒充真实 Owner Adapter、Server 身份认证或生产策略存储验收。

## 4. CallerRef 与生产装配边界

`CallerRef` 在 Research Port 中仍是显式字符串输入。Research 不负责认证调用方，也不能仅凭字符串内容证明主体可信。调用方必须遵守以下边界：

1. Server 完成认证后，从可信 Principal Context 取得主体引用。
2. Server 构造 ContextReadRequest 时注入该 CallerRef；不得直接绑定客户端 DTO、Header 或用户可控字段。
3. ContextAuthorizer 由 Server 注入真实 Owner Adapter，针对当前主体、用途和 Context 可见范围作出当前授权判断。
4. Authorizer 缺失、unknown、拒绝、超时或依赖不可用均不得返回 Context。

本轮不修改 Server，不自创 Context 专用 resource_ref 拼接语法、策略注册表或授权服务；生产身份映射与 Adapter 运行验收留在 Server/Integration 组合根。

## 5. 验收证据

Run `44286491873` 的 head SHA 与代码验收 SHA 完全一致：

- Research module validation：gofmt、`go test ./internal/research/...`、`go test -race ./internal/research/...`、`go vet ./internal/research/...` 全部 success。
- Research MariaDB Migration/Repository integration：success。
- 根模块 `Test .` 的 gofmt 与 `go test ./...` 已通过；本记录只将 Research 专项 Job 作为 RSC-001 的直接验收门禁，不把其他模块 skipped 解释为通过。

## 6. 未完成事项与后续任务

- **外部待验收：** Server 的认证后 CallerRef 注入、真实 Context Owner Authorizer 生产装配和生产调用链验证。
- **Research 下一任务：** RSC-003 Event 时间证据与派生关系规则级复核。RSC-008 仍等待 Researchflow 提供真实 Facade/Check 调用点。
- 真实来源时间、多来源归并/拆分样本和 `asserted_by` 的来源语义，仍需 News/Data Owner 的可审计证据；Fixture 不可替代。
