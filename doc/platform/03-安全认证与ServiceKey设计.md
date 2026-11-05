# 安全认证与服务凭证设计

## 1. 目标与威胁模型

系统由单一所有者使用，但 API 可能经过公网反向代理，也存在多个内部服务调用。首期目标是明确边界、默认拒绝、凭证隔离和可轮换，不建设独立凭证管理系统。

重点防止未授权调用、伪造身份 Header、凭证泄露、Provider Token 冒充内部服务，以及管理接口暴露。

## 2. 首期身份模型

| 身份 | 用途 | 凭证来源 |
|---|---|---|
| anonymous | 明确允许的 liveness 等少数接口 | 无凭证，路由显式白名单 |
| owner | 所有者管理操作 | 独立 `PLATFORM_ADMIN_TOKEN` |
| service | 服务间调用 | 每个调用方独立的环境 Secret |

不实现用户注册/登录、组织、租户和独立 job 身份。后台任务记录创建来源、Request ID 和 task_id；执行身份来自服务端任务记录，不接受客户端自报身份。

## 3. 首期凭证方案

采用部署 Secret 注入，不建立 Service Key 数据库表，不提供创建/禁用/撤销 API，不建设管理页面。

- 每个调用方使用独立 Secret，不能让所有内部服务共用一个万能 Token。
- owner Token 与任意 service Token 分离。
- Secret 使用密码学安全随机数生成，建议至少 256 bit 随机熵。
- 通过 Authorization: Bearer 传递；禁止通过 URL Query 或普通自定义身份 Header 传递。
- 服务端只从启动配置读取 Secret；不散落读取环境变量。
- 使用常量时间比较验证，避免普通字符串比较泄漏比较时序。
- Secret 不写日志、错误、诊断、响应体或普通配置快照。
- 轮换通过部署 Secret 更新和服务重启/滚动切换完成；切换后旧凭证拒绝。
- Provider Token 与内部服务 Secret 完全分离。

对于少量固定调用方，部署 Secret 已能满足独立凭证和轮换要求。数据库摘要、Key ID、动态撤销和管理 API 会额外引入表、迁移、缓存一致性和管理流程，当前没有足够收益。

## 4. 未来动态凭证的触发条件

只有出现以下需求，才重新启动动态 Service Key 设计：
- 调用方数量明显增长，部署 Secret 管理开始不可维护；
- 需要不重启服务地创建、禁用或撤销某个调用方；
- 需要凭证到期策略、独立审计或无停机轮换；
- 多用户/多租户正式成为产品目标。

触发后再评估 Key ID + 高熵 Secret、DB 摘要存储、权限 Scope、撤销缓存和审计。当前不预建相关表或 package。

## 5. 最小授权

首期采用少量固定权限组，按真实路由裁剪：
- `platform:admin`：owner 管理接口；
- `data:read`：行情读取；
- `data:diagnose`：行情诊断；
- `research:run`：触发研究任务；
- `research:read`：读取研究结果。

无凭证、权限不足和未声明路由默认拒绝。不支持通配符、隐式继承或动态策略引擎。身份凭证只证明调用方身份，Scope 决定其允许访问的能力。

## 6. 路由访问策略

| 路由 | 访问要求 |
|---|---|
| liveness | 可匿名，只返回进程存活 |
| readiness | 由部署边界保护；仅返回最小依赖状态 |
| 普通业务读取 | 根据真实用途明确 owner/service |
| 业务写入、研究任务 | owner 或具备对应权限的 service |
| 管理和诊断明细 | owner；必要时允许指定 service 权限 |

所有路由显式声明策略；未声明默认拒绝。不能通过内网 IP、反向代理路径或客户端自定义身份 Header 绕过认证。

## 7. 管理入口与审计

管理入口只接受 owner Token，并优先限制在私有入口。首期不建设管理 UI 或凭证 CRUD API。

只记录必要的敏感管理事件，例如关键配置变更和未来动态凭证操作。日志记录主体类型、动作、结果、时间和 Request ID，不记录凭证明文。普通业务读取不做逐请求审计。

## 8. 失败策略与测试

- 凭证缺失、错误或配置为空：受保护路由拒绝。
- owner Token 不得访问仅 service 可访问的接口，反之亦然，除非路由明确允许。
- Key/Token 存储不依赖 Redis；Redis 故障不改变认证结果。
- 配置加载失败时服务启动失败，不以空 Token 启动受保护服务。
- 日志不得输出 Authorization 和 Secret。
- 测试覆盖有效/无效凭证、身份混用、伪造 Header、路由默认拒绝、常量时间比较调用路径和日志脱敏。



## 9. 平台认证原语实现补充（2026-11-05）

实现位于根 Module 的 `internal/platform/security`：

- owner 与 service 凭证通过 `config.Secret` 注入；owner 和每个 service 使用不同 Token。
- Token 至少 32 字节；服务启动时拒绝空值、弱长度、重复凭证和重复 service 名。部署侧应使用密码学安全随机数生成至少 256 bit 随机熵的 URL-safe Token；长度检查不能替代随机熵生成。
- 服务端只保存 SHA-256 摘要用于匹配；每次认证遍历全部凭证摘要并执行固定长度常量时间比较。高熵随机 Token 使摘要被离线猜测的风险保持在可接受边界。
- service 必须配置至少一个固定 Scope；拒绝未知 Scope、重复 Scope 和 owner 专属 `platform:admin` Scope。
- `Guard` 只信任 Bearer 凭证生成的 Principal，忽略客户端身份 Header；Principal 通过 Context 提供给 handler。
- `Router.Handle/HandleFunc` 要求每条注册路由显式传入策略；未配置策略不能注册，未注册路由由 ServeMux 返回 404。
- `OwnerOrServiceAccess` 表示 owner 被路由显式允许，service 必须同时满足该路由声明的全部 Scope。owner 不会被自动映射成 service，也不会隐式继承业务 Scope。
- 匿名路由仅使用 `AnonymousAccess()`；若匿名请求携带错误 Bearer 凭证，仍返回 401，避免错误凭证被静默忽略。

该包不负责启动入口和现有服务路由改造。集成方必须将所有路由统一注册到受策略约束的 Router，不能保留旁路 ServeMux 或未保护的 Handler 注册路径。


## 10. PLAT-003 代码验证记录

最终验证提交：`f9334281ba0f90406fcd7de0f239afe24845bb23`。Platform packages workflow 已通过 gofmt、平台包单测、race test 和 go vet。

首次提交的 gofmt 检查未通过，已修正后重跑，最终提交 CI success。服务入口和真实路由接入尚未实施；凭证轮换的部署演练也未执行，因此本任务仍处于开发中。
