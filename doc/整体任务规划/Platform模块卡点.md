# Platform 模块卡点

- 仓库：`Kasign/stock_data_provider`
- 模块分支：`feat/platform`
- 记录日期：2026-11-18
- 记录基线：`3d8b968b1f2b218f9daadf919f15e988a2c026d0`
- 模块代码范围：`internal/platform/**`
- 模块文档范围：`doc/platform/**`
- 状态口径：区分 Platform 本地阻塞、消费者接入阻塞和生产环境验收阻塞；不得将其他 Owner 的待办记为 Platform 代码缺陷。

## 1. 当前结论

**Platform 本地没有已确认的 P0/P1/P2 代码阻塞项。**

Platform 已交付的公共基础能力包括 Config/Secret、Fault、Request ID、Diagnostics Schema/Runner、受控 HTTP Client、Security/Owner Session/CSRF/RateLimit/Audit、Storage/Migration、Time。当前未发现需要为满足现有消费者而新增公共接口、字段、通用队列或领域能力的证据。

Platform 代码树与已通过专项 CI 的代码基线 `b2166838b51eb1ca051fb9f07fd068d2b31a751e` 一致：`internal/platform/**` 共 62 个文件，路径集合和 Blob SHA 完全一致。对应 Actions Run `44263167831` 的 Platform packages、Storage integration 均成功。该证据证明对应代码树的专项测试结果，不等于当前文档提交 SHA 有新 CI，也不等于生产 A3 验收通过。

因此，以下事项是跨模块/部署验收卡点，不是要求 Platform 继续增加代码的理由。

## 2. 尚未关闭的卡点

| 编号 | 卡点 | 当前证据与影响 | 责任 Owner | 解除条件 | 是否阻塞 Platform 本地开发 |
|---|---|---|---|---|---|
| PLAT-BLK-01 | Owner Session 审计失败后的业务恢复闭环 | InitializeOwner/ChangePassword 等业务状态可能已提交，但成功审计写入失败时调用仍返回错误；现有测试固定了部分失败语义。调用方若把错误解释为业务未生效，会造成误操作或重复操作 | Server | 明确用户可见错误语义；提供审计缺口可观测、幂等核对和恢复方案；必要时由 Server 评估事务性 Outbox/补偿，不把领域状态塞进 Platform Audit Writer | 否 |
| PLAT-BLK-02 | 生产审计运维控制未验收 | 审计保留/清理、归档、备份恢复、生产访问权限隔离缺少真实部署验收证据 | Server + 部署/运维 | 提交保留策略、权限矩阵、备份恢复演练和运行证据 | 否 |
| PLAT-BLK-03 | HTTP Client 生产出口安全未验收 | Platform 已有目标地址、DNS/IP、重定向等代码级约束；生产 Gateway/代理/出口网络仍需在真实环境验证，尤其 DNS rebinding、私网/保留地址、跨 Host 重定向、代理绕过及 IPv4/IPv6 | 部署/运维 + Server；各 Provider Owner 配合 | 在真实部署网络执行 SSRF 攻击矩阵并保存请求/拒绝证据；确认代理和 egress 策略不能绕过 DestinationPolicy | 否 |
| PLAT-BLK-04 | MariaDB/Redis 生产依赖与恢复验收 | Platform 有连接生命周期、Ping 和 Migration Runner，专项 Storage integration 已有成功记录；生产凭据、网络、断连/重连、超时、恢复和真实数据持久化仍需部署验收 | Server + 部署/运维；各领域 Owner 验证自有 Repository | 在目标环境验证 MariaDB/Redis 连通、超时、断连恢复、关闭生命周期；Redis 不得承载唯一事实 | 否 |
| PLAT-BLK-05 | Migration 聚合执行与失败恢复 | Platform 提供 Runner/namespace 隔离原语；各领域 Migration 清单注册、启动顺序、部分失败后的恢复和重复执行语义属于 Root Server 装配与领域 Migration 所有者 | Server + 各领域 Owner | 根启动入口注册所有当前模块 Migration；验证重复执行、版本冲突、部分失败和重启恢复 | 否 |
| PLAT-BLK-06 | 领域 Diagnostics 未全部接入 | Platform Runner 已提供执行和聚合能力；领域 Check 的实现、语义映射及 Server 注册不是 Platform 职责。未注册的领域检查会影响相应领域诊断覆盖，但不影响 Platform Runner 本身完成 | 各领域 Owner 实现 Check；Server 注册并配置 Required/Optional 策略 | 各领域按自身任务验收 Check；Server 提供依赖注入、注册、访问控制及 Readiness 策略证据 | 否 |
| PLAT-BLK-07 | Worker Request ID 恢复链路 | Platform 提供 Request ID Context/传播原语；后台任务创建时持久化、Worker 执行入口恢复属于任务创建方和 Worker Owner | Researchflow/各 Worker Owner；Server 负责装配 | 任务持久化创建时 Request ID；Worker 启动时恢复并在日志/出站调用中关联，覆盖缺失值和重试 | 否 |
| PLAT-BLK-08 | 项目级公共平台验收清单存在历史状态冲突 | `doc/整体任务规划/03-模块接口契约/Platform-五模块公共依赖与集成验收清单.md` 仍保留“Platform 公共能力尚未通过”“GOV-004 仍未冻结”等较早状态描述，与当前 `doc/platform/当前任务状态和进度.md` 对 Platform L1 本地责任已完成的口径不一致 | 项目规划 Owner（`feat/design_doc`）；Integration 只提供当前聚合证据 | 对照当前各模块精确 SHA、CI 和真实依赖证据，修订该清单的状态口径；保留真实未通过的 A2/A3 门禁，不将历史快照当作当前状态 | 否 |

## 3. 已明确不属于 Platform 卡点的事项

1. Data/News/Research/Researchflow/Quant/LLM/Notification 的领域诊断规则和 Check 实现。
2. Server 对领域 Diagnostics Check 的注册、Required/Optional 策略和 HTTP 路由装配。
3. Root Server 对 Data Module、领域 Migration、Worker 和其他模块的依赖注入与生命周期装配。
4. 生产 Gateway、DNS、egress、Secret、MariaDB/Redis 运维策略和恢复演练。
5. 历史 `internal/platform/httpapi`、`internal/platform/taskqueue` 中 Analysis 领域耦合的迁移；该迁移由 Server/Analysis Owner 负责，Platform 不把它扩展成通用 API/队列。

## 4. 当前执行顺序

| 顺序 | 工作 | 执行方 | Platform 分支动作 |
|---|---|---|---|
| 1 | 校准项目级 Platform 集成清单的历史状态，明确 L1/A2/A3 分层 | 项目规划 Owner | 提供本文件和当前状态证据；不代改其他模块 |
| 2 | 收口 Owner Session 审计失败恢复语义 | Server | 仅在发现 Audit 公共原语缺陷时修复 |
| 3 | 补齐领域 Diagnostics Check 和 Server 注册 | 各领域 + Server | 不实现领域规则 |
| 4 | 完成 Migration 汇总注册和失败恢复验收 | Server + 各领域 | 仅修复 Runner 可复现缺陷 |
| 5 | 完成 MariaDB/Redis、HTTP 出口、审计运维 A3 | 部署/运维 + Server | 提供现有原语，不虚报生产验收 |
| 6 | 完成 Worker Request ID 持久化恢复 | 各 Worker Owner | 仅处理公共传播原语缺陷 |

## 5. Platform 重新打开开发的条件

只有满足以下任一条件，才新增 Platform 本地开发任务：

- 存在可复现的 Platform 公共原语缺陷，且调用链证明影响至少一个真实消费者；
- 已冻结契约存在实现偏差或安全漏洞；
- Platform 自身测试/CI 在当前代码树上回归；
- 两个或以上真实消费者提出相同、无法通过领域 Adapter 解决的公共能力缺口，并完成最小 API 评审。

不以“其他模块还没接入”“集成清单尚有未勾选项”或“需要保持 Platform 持续开发”为由新增抽象。

## 6. 相关权威文档

- `doc/platform/当前任务状态和进度.md`：Platform 本地任务状态与代码验收证据。
- `doc/platform/模块总任务规划和进度.md`：Platform 模块总任务。
- `doc/platform/12-PLAT-013领域接入差距矩阵.md`：领域接入差距的历史证据和责任边界。
- `doc/整体任务规划/03-模块接口契约/Platform-五模块公共依赖与集成验收清单.md`：项目级跨模块公共依赖验收清单；其中历史状态需按 PLAT-BLK-08 校准。
