# Server 模块卡点

更新时间：2026-11-18
维护分支：feat/server
当前代码 HEAD：81e0ddf020cb69a050a9e0c7f3f509378de49989
当前主线：SRV-009 Data/MarketData M3/M4 Root 生产装配

## 1. 当前阻塞：真实 Root Readiness 未通过

状态：BLOCKED（待定位具体 Required Check）

证据：
- Server 专项 CI Run：44295762065
- Job：Test Server-owned packages
- Job ID：134772255903
- 精确检出 SHA：81e0ddf020cb69a050a9e0c7f3f509378de49989
- gofmt、Server package tests、race tests、真实 MariaDB/Redis storage integration、Server vet、镜像构建及 Notification schemas 均通过。
- 容器启动日志显示 Data Module Migration 成功，Server Migration Registry 报告 succeeded，异步分析 Worker 启动，Root Server HTTP 监听 `[::]:8092`。
- 随后的真实容器验收在 120 秒内未能通过带 Owner Bearer 的 `/readyz`，HTTP 返回 503；`/healthz` 探测已成功，因此容器进程和 HTTP 服务并非未启动。
- 该失败 SHA 所使用的旧 smoke 脚本通过 `curl --fail` 请求 `/readyz`，HTTP 503 时没有保留响应体，因此 Run 44295762065 无法直接确认具体失败的 Required Check。
- 随后的当前分支版本 `.github/workflows/server-test.yml` 已改为记录 HTTP 状态码和响应体，并在失败时请求 `/diagnostics`；但这只是诊断能力改进，尚无新的成功 CI 证据，根因仍未定位。

原因边界：
- 已确认：Readiness 检查未通过。
- 尚未确认：究竟是 Data Diagnostics.Ready、其他 Required Diagnostics Check、配置缺失还是检查适配错误。不能仅凭 Data Module Migration 成功就推断 Data Ready 必然健康，也不能未经证据归因于 Data 模块。
- 本次失败发生在 Ready 探测阶段；SIGTERM 清洁退出步骤未执行，因此不能声称真实容器关闭验收通过。

下一步：
1. 验证当前分支已更新的 Server smoke 验收脚本：确认下一次运行会保留 HTTP 状态码和响应体，并输出 `/diagnostics`；若新日志仍缺少 Required Check 细节，再补充容器日志或报告输出。
2. 在相同真实 MariaDB/Redis Bootstrap 环境中复现，读取每个 Required Check 的 name/status/reason_code/evidence，定位唯一失败项。
3. 若失败属于 Server 装配/配置/检查适配，在 `internal/server/**` 修复并补充回归测试；若属于 Data/Platform 模块公开契约或实现缺陷，记录为对应模块前置，不在 Server 分支复制或绕过其实现。
4. 修复后重新运行精确 SHA 的 Server CI，要求 Readiness 成功并完成 SIGTERM 退出码 0。

## 2. 跨模块前置：Data Migration 定义与并发安全

状态：BLOCKED（等待 Data / Platform Owner 前置）

已核实问题：
- Data Store.Migrate 在 Platform ApplyMigrations 的 namespace 锁保护范围之外，仍执行兼容性 `ensureNonUniqueHistoryIndex`、`ensureColumn` DDL；多实例并发启动存在竞态。
- Root 当前直接调用 Data Module 的 MigrationRunner，再运行 Server Registry；Data 尚未以完整静态 Migration 定义注册进统一 Registry。
- `MIGRATION_ENABLED=false` 时当前只读校验不覆盖 Data checksum/物理 Schema 漂移；Data Diagnostics.Ready 的 DB/Redis Ping 不能替代 Schema 校验。

期望前置：
- Data 暴露完整、不可变的 Migration 定义，保留已发布版本 checksum，并将兼容 DDL 纳入后续版本。
- Platform 提供与 Apply 共用 checksum preparation 的只读 ValidateAppliedMigrations 能力。
- Server 仅负责注册、调度和真实启动验收，不复制 SQL、checksum 算法或 Migration Runner。

解除条件：
- Data/Platform 对应改动合入当前集成基线；Server 接入统一 Registry；验证迁移开启/关闭、多实例并发启动、checksum 不一致和 Schema 漂移行为。

## 3. 尚未闭环的 Server 自身验收

- Data Handler 的 DATA-CTR-006 端到端行为：Owner 身份上下文、请求体限制、Request ID、Fault 投影和实际路由分发。
- API 与 Worker 两种角色的真实 Root Bootstrap、启动失败回滚和生命周期关闭。
- 启用 Provider 配置矩阵及 HTTP/TCP 受控出口注入；真实 Provider 连通性没有真实上游证据时标记 NOT_COMPARABLE。
- Data Module Start/Stop/Close 与 Root 共享资源的关闭顺序、幂等性和并发边界。

## 4. 当前不应做的事

- 不重做已经接入的 Data Root 装配，不以重构代替当前 Readiness 根因定位。
- 不把本次 CI 归类为全通过，也不把失败笼统归因于 Data。
- 不在 Server 分支直接修改 Data/Platform 私有实现或复制公共能力。
- 不提前删除旧 Analysis API/Worker；SRV-013 仍按依赖保持 deferred。
