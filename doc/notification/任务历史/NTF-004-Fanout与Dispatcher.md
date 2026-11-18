# NTF-004：Fanout / Dispatcher

状态：DONE（模块核心）；Server Worker、真实 RecipientDirectory 和业务 Filter Matcher 待装配  
复核代码基线：`0ab21f4516c2fb961823736b5ea0cc033d8b39bd`

## 已实现/复核范围

FanoutPlanFactory、Dispatcher、有界分页、固定 Subscription revision 快照、FanoutWorkItem Lease/Fencing/CAS、DeliveryPlan 生成、MySQL WorkItem Repository、Channel/Template Version Selector。

## Review 结论

- Inbox 接收与 FanoutWorkItem 创建共用事务，事件接收成功后不会缺少对应展开任务。
- 首次 Claim 固定 Subscription revision 高水位；retry_wait 和过期 processing 继续使用既有快照，快照异常丢失时 fail closed，不重新读取最新 revision。
- Subscription 分页按 ID keyset 推进；SQL 先在快照高水位内选每个 Subscription 的最新版本，再过滤 active/event_type，避免旧 active 版本覆盖新停用版本。
- Dispatcher 单次处理一个 WorkItem 的一个有界页面；数据库时钟、租约、Worker ID、fencing token 和 CAS 保护并发领取及状态推进。
- 持久化结果不确定时不使用当前 Claim 继续 Retry/Fail；DeliveryPlan 的确定性身份和唯一约束支持安全重放。
- 非空 Filter 没有精确版本 Matcher 时 fail closed；ChannelConfig 与兼容 Template 按版本解析；RecipientDirectory 授权结果以 opaque target 和策略证据冻结。
- 未发现需要立即修复的确定性代码缺陷。

## 剩余边界

Server 负责 Fanout/Delivery Worker 生命周期和真实 RecipientDirectory；非空 Filter Matcher 等待业务 Owner 冻结语义。Notification 不自行启动后台 Worker，也不虚构身份目录。

## 当前验收证据

代码复核基线 Run [44285380574](https://github.com/Kasign/stock_data_provider/actions/runs/44285380574) 的根模块测试、Research/Researchflow 测试和 Notification MariaDB lifecycle integration 均 success。真实 Server Worker 运行验收待 Server 装配。

原始逐轮提交、Review、失败记录和测试证据完整保存在 [00-历史完整执行记录](./00-历史完整执行记录.md) 的阶段 4、FanoutWorkItem/Dispatcher 增量及对应增量章节中。
