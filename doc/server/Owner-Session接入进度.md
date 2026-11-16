# Owner Session 接入进度

## 当前阶段：Bootstrap 依赖组合

Server Bootstrap 通过共享 MariaDB / Redis 资源组合以下已有能力，不创建独立连接池，也不复制 Platform 安全实现：

- Platform SQL Credential Store
- Platform Redis Session Store
- Platform Redis Login Rate Limiter
- Platform Argon2id Hasher
- Server SQL Security Audit Writer
- Server SessionLifecycleService
- Server SessionAuthenticator

Owner Session 默认关闭；启用时必须提供显式配置与经过目标硬件基准测试的 Argon2id 成本参数。依赖缺失或构造失败时启动 fail-closed。

## 本阶段新增测试

`internal/server/bootstrap/owner_session_test.go` 覆盖：

- Session 关闭时不创建依赖；
- 缺少共享 MariaDB / Redis 时拒绝组合；
- Argon2id 参数无效时拒绝组合；
- Integration CI 提供 MariaDB / Redis 时验证完整 Lifecycle 与 Authenticator 组合。

完整组合测试只构造依赖，不执行登录、不要求创建业务表；MariaDB 与 Redis 连通性及迁移仍由 Bootstrap / 现有集成测试负责。

## 尚未完成

- 当前提交尚未获得新的 CI 验收结果。
- Session 登录、登出、轮换、密码变更 HTTP Handler 未实现。
- 一次性 Owner 初始化的受限运维入口未实现。
- SessionAuthenticator 尚未接入受保护路由；Origin / CSRF / Cookie 刷新尚未进入 HTTP 请求链。
- Bearer 兼容与 Session HTTP 集成测试未完成。

因此 Owner Session 目前仍不是可供 WebUI 使用的登录态；不得接入 Researchflow Owner API。

## CI 汇总步骤待查

Run `43866953446` 的工作流最终结论为 failure，失败步骤显示为 `Summarize all checks`。由于各检查步骤启用了 `continue-on-error`，GitHub UI 中步骤的最终结论不能单独证明命令成功；必须以步骤原始 outcome 和日志为准。当前没有足够日志证据将失败归因于汇总脚本自身，也不能将该 Run 记为全量通过。
