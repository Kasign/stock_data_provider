

## 8.7 Delivery Unknown 到渠道查询请求的受控绑定

渠道查询使用独立可选的 `ChannelDeliveryLookup` Port，不扩展必需的 `ChannelAdapter` 发送接口。只有 Channel 快照声明 `supports_delivery_lookup=supported)，且该精确版本 Adapter 实现查询 Port 时，Registry 才允许解析；声明与实现不一致、能力为 unknown/unsupported、版本不存在时均失败关闭。

`BuildChannelDeliveryLookupRequest` 只接受状态为 unknown 的 Delivery、同一 channel_ref/config_version 的 Channel 快照，以及该 Delivery 最后一次已完成且结果为 ambiguous 的 Attempt。请求身份由这三份快照构造，调用方不得自行替换 DeliveryID、AttemptID 或配置版本。只有 `supports_idempotency=supported` 时才携带 Delivery 稳定幂等键；Attempt 已有 Provider Message Ref 时可作为辅助查询键。请求不携带收件人明文地址、模板正文或完整发送请求。

`ChannelRegistry.LookupUnknownDelivery` 是当前查询到对账证据的受控入口：它通过精确版本 Registry 调用 Adapter，校验结果身份与请求一致后，才转换为 `UnknownDeliveryEvidence`。查询超时、认证失败、限流、解析失败等错误原样返回，不能转换为 `not_accepted`；三态结果分别映射为 accepted/not_accepted/indeterminate。Adapter 对 Provider 最终一致性窗口的判断属于具体渠道实现责任；窗口内“暂未查到”必须返回 indeterminate。

该入口只保证模块内部调用边界与身份绑定，不宣称 Go 类型系统能证明 Adapter 查询事实真实或阻止同进程内部代码伪造证据。权威性仍依赖受控 Adapter 实现、正式 Provider 查询语义和逐渠道 Contract Test。查询结果后续仍由既有 `AssessUnknownDelivery`、RetryPolicy 与 Delivery.version CAS 收敛状态；本 Port 不直接修改 Delivery 状态。


## 8.8 Unknown 对账的强制 Application 边界

生产状态收敛统一通过 `UnknownDeliveryReconciliationService.Reconcile` 执行，不允许调用方分别提供 Delivery、Attempt、Channel 快照或手工构造的 `UnknownDeliveryEvidence` 后直接持久化。

该用例按以下顺序执行：

1. 通过 `DeliveryConsistencySnapshotReader` 在同一数据库一致性快照中读取 Delivery、完整 Attempt 历史和 StateEvent 历史。
2. 使用 `AssessDeliveryConsistency` 校验两类历史；结果必须为 `pass`，Delivery 必须仍为 `unknown`。fail、degrade、not_comparable 均停止，不查询渠道。
3. 从 Registry 按 Delivery 固定的 `channel_ref + config_version` 读取不可变 Channel 快照，并执行 Lookup。
4. Registry 查询错误或 Context 取消立即返回，不调用状态持久化。
5. 查询结果包装为 application 包内不可构造的 `verifiedUnknownDeliveryEvidence`，绑定 Delivery version、最后一次 Ambiguous Attempt ID 和完整 Channel 快照；只有包内收敛函数能消费该类型。
6. 使用读取快照中的 Delivery version 执行现有 CAS。查询期间 Delivery 被其他 Worker 更新时，CAS 冲突退出；不得把旧查询证据应用到重新读取的新版本。

`AssessUnknownDelivery` 仍是接收普通 Evidence 的纯规则评估函数，可用于单元测试和规则验证，但不具备持久化能力。原先直接接收调用方 Evidence 的导出状态收敛函数已收紧为包内函数。外部调用方必须使用一致性快照服务。

此边界能阻止模块外部调用方伪造 Evidence 绕过 Registry，并避免把不同时间的 Delivery/Attempt/StateEvent 读取拼成一组输入；它不证明 Provider 自身查询语义真实，也不消除查询后到 CAS 前的竞态，后者由版本 CAS 明确拒绝。
