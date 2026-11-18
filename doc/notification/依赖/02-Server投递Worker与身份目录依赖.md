# 依赖 02：Server 投递 Worker 与身份目录

状态：BLOCKED（责任方 Server）

## Server 需要完成

- 提供 Recipient Directory，按 recipient_ref 解析授权目标，不向 Notification 传递任意明文地址。
- 装配 Fanout Worker 与 Delivery Worker，明确并发、批次、轮询/唤醒、退避、启动与优雅停止。
- 注入 Notification Repository、Template Repository、Channel Registry、RecipientResolver 和 Platform Secret/HTTP Client。
- 将 Notification v1～v6 Migration 纳入根 Migration 汇总入口。
- 为 Worker 提供统一 Request/Correlation ID、Diagnostics、日志脱敏和审计上下文。
- 在真实服务环境验证进程崩溃、租约恢复、优雅关闭和多实例并发。

## Notification 责任

提供 Application Service、Repository Port、Channel/Recipient SPI 和 Migration；不在本分支创建 Server Bootstrap、HTTP 路由或通用 Worker 框架。

## 验收证据

Server 分支提交、根模块集成测试、真实服务启动/停止与 Worker 恢复记录。Fake Worker 不能替代 Server 运行验收。
