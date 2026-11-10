# WebUI 维度 2：第一轮完整性 Review

**审计代码基线：** `origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`  
**文档维护分支：** `feat/design_doc`  
**Review 范围：** WebUI 维度 2 数据集覆盖、API Client 消费映射、差异编号和跨数据集契约问题  
**Review 性质：** 局部完整性检查；不是 PLAT-002 全项目阶段 F 完成声明，也不是正确性/冻结 Review。

## 1. 主线范围确认

上一阶段明确的剩余数据集已经全部完成独立切片：
- Stock Search：`WebUI-维度2-Stock-Search.md`
- Analysis Run：`WebUI-维度2-Analysis-Run.md`
- Provider Probe History：`WebUI-维度2-Provider-Probe-History.md`

本轮不增加新的数据集，不回头重复已完成切片。当前进入对已有证据和差异登记的一致性复核。

## 2. API Client 与消费映射完整性

依据 `WebUI-维度2-API-DTO全量盘点.md`：
- `web/src/api.js` 共 44 个导出函数：3 个本地设置函数，41 个远端请求/健康检查入口。
- 40 个远端入口在主要页面模块找到引用；`getMarketSentiment` 是唯一已确认没有页面模块消费者的入口。
- API Client 有调用点，不等于页面完成产品消费；`existing-api-tools.js` 中的工具模块调用不能直接算作默认页面闭环。
- 既有数据集切片覆盖 API 盘点中主要的 Market Data 数据集、Analysis Task/Run 和 Provider Probe History。对没有页面消费者的接口，保持“产品范围待确认”，不因无页面而自动登记缺陷。
- `getMarketSentiment` 是否为预留 API 仍需产品范围确认；不扩展成新的本轮审计任务。

结论：API 入口盘点已完成第一轮，尚不能宣称所有 API 都具有完整产品消费闭环；这是页面产品范围与维度 1/3/4 等其他维度的交叉问题，不通过本轮重复扫描解决。

## 3. 差异编号与重叠检查

### 3.1 CTR-003 与 CTR-046

- CTR-003 最初登记：`GET /api/v1/analysis/runs/{run_id}` 未进入正式 Analysis OpenAPI。
- CTR-046 复核扩展：Run 读取和 Run Events 两条路径、RunResult 等响应 Schema 缺失，并补充 HTTP Handler 测试缺口。
- 两者核心问题相同，CTR-046 的范围更完整。
- 处理：CTR-003 标记为历史初审编号，由 CTR-046 承接；不作为独立活动差异重复计数。保留旧编号以维持审计记录可追溯。

### 3.2 CTR-005 与 CTR-047

- CTR-005 初审指出 AnalysisRequest 与 domain.RunRequest 的描述不一致，缺少 TaskType 枚举、required 约束，以及 as_of_mode/parameters。
- CTR-047 进一步核实 as_of_mode、parameters 和服务端生成 run_id 的实际行为。
- 本轮复查固定基线：`domain.TaskType` 确有四个常量，Runner 对未知 task_type 拒绝；AnalysisRequest Schema 目前只声明 task_type 为普通 string，且没有 required 列表。
- 处理：CTR-047 承接字段映射和异步行为差异；CTR-005 保留为初审编号，但其剩余的 TaskType enum 与必填约束必须纳入 CTR-047 的关闭条件，不能把 CTR-005 整体标为已解决。

### 3.3 CTR-010 的多处引用

- CTR-010 是跨 DTO 的统一空值语义差异登记，后续切片将各自的 omitempty 字段追加为影响范围。
- 多次引用是影响字段清单扩充，不是多个同编号独立缺陷。
- 关闭时必须按 DTO/字段逐项核对 Go JSON 实际序列化、OpenAPI nullable/required 和 Golden；不能只修复最早登记字段就关闭整个 CTR-010。

### 3.4 CTR-038 与 CTR-048

- 两者都涉及历史接口 total 不能表达全量记录数/截断状态，但分别属于 Market Sentiment History 和 Provider Probe History。
- DTO、Repository、Handler 和页面消费不同，不合并为同一缺陷；可共享“total/returned/has_more 语义规范”作为修复设计参考。

## 4. P1 与测试门禁回看

- CTR-041（Limit Pool）仍为 P1、待固定基线 CI 复核：DTO 指针 JSON、OpenAPI nullable/required、Golden 断言存在三方矛盾。本轮未执行 CI，不得降级或关闭。
- CTR-043（Dragon Tiger）为 P1：Hithink source_returned 与 Service 当前页计数约束不一致，重复 symbol 计数也不闭合。源码已确认逻辑冲突，但本轮没有执行固定基线测试，不将源码审计描述成运行时测试失败。
- 其余本轮新增 CTR-045 至 CTR-049 均登记 P2；P2 不代表可以在未决情况下冻结。
- 多个数据集的 API Client 或 DOM 测试缺失是独立补证项；只有当缺失测试掩盖字段语义/错误处理/用户误读时，才与对应行为差异一起登记，不把“没有测试”本身自动扩大为缺陷。

## 5. 跨数据集语义一致性观察

| 主题 | 涉及差异 | Review 结论 |
|---|---|---|
| OpenAPI nullable 与 Go omitempty | CTR-010、CTR-011 等 | 跨多个 DTO 的同类问题，关闭必须逐字段验证 |
| 历史 total/截断语义 | CTR-038、CTR-048 | 不同接口的独立问题，统一规范可复用 |
| Provider 缺少时间/覆盖证据 | CTR-015、CTR-025、CTR-027、CTR-031、CTR-034 等 | 各数据集的证据来源和业务含义不同，不合并为一个通用字段修复 |
| Provider 金额单位不明 | CTR-016、CTR-042、CTR-044 等相关记录 | 保持 provider_defined/不可比较，不可为了 UI 统一而猜测单位 |
| 页面消费者/DOM 测试缺口 | CTR-030、033、036、041/042 等对应记录 | 区分产品未接入与已有功能缺少回归测试，不能自动扩大产品范围 |
| Run 与 Task 状态 | CTR-046/047 | 必须保持异步执行状态与分析业务结果状态分离 |

## 6. 本轮 Review 结论

<badge color="warning">partial · 继续复核</badge>

已完成：
- 明确三个最后数据集均有独立审计记录。
- 对照 API Client 总盘点，确认没有因为最后三个数据集而遗漏新的数据集入口。
- 识别并处理 CTR-003/046 的重复登记关系。
- 明确 CTR-005/047 的部分重叠及 CTR-005 剩余 TaskType enum/required 约束。
- 确认 CTR-010 的重复出现是影响字段追加；CTR-038/048 是不同 API 的同类语义问题。
- 回看 P1 门禁，未把未执行 CI 的问题误标为通过。

尚未完成：
- 逐个复核所有 CTR-001 至 CTR-049 的原始证据、关闭条件和责任模块；本轮只处理编号重叠与跨数据集主题。
- 对每个数据集逐字段重跑 DTO/OpenAPI/Golden 比对。
- 固定基线 Go/Node CI 和依赖 MariaDB 的集成测试。
- WebUI 页面交互覆盖与无消费者 API 的产品范围决议。
- 与 WebUI 维度 1、3、4、5、6 的交叉检查。

因此 WebUI 维度 2 仍为 partial；本文件只完成第一轮完整性 Review 的一个子批次，不宣布维度 2 或 PLAT-002 完成。下一批继续在本维度内核对差异关闭条件和证据链，不增加数据集范围。


### 3.5 CTR-005 的 required 语义进一步收敛

再次对照 `internal/platform/httpapi/tasks.go:handleCreateTask`、`internal/platform/httpapi/server.go:fillDefaults`、`internal/analysis/app/runner.go` 和 AnalysisRequest Schema 后，不能简单把 RunRequest 所有非 omitempty 字段都列为 HTTP required：

- HTTP Handler 会为缺省 task_type、as_of、universe_id、portfolio_id、market_id、strategy_ids 及配置版本填充服务端默认值。
- historical_replay 是条件例外：必须由调用方显式提供 as_of；Handler 会在缺省时返回 400。
- Runner 会校验 task_type 仅允许 post_market_scan、watchlist_scan、position_review、historical_replay 四个值。
- 因此 CTR-005/047 的正确关闭要求是：将 HTTP CreateTaskRequest 与 domain.RunRequest 的字段映射和默认行为明确分开；OpenAPI 为 task_type 补四值枚举及默认值，为其他服务端默认字段描述默认行为，并表达 historical_replay 对显式 as_of 的条件约束。不能机械地将所有领域字段标成 required。
