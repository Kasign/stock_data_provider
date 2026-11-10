

## 8.7 Delivery Unknown 到渠道查询请求的受控绑定

渠道查询使用独立可选的 `ChannelDeliveryLookup` Port，不扩展必需的 `ChannelAdapter` 发送接口。只有 Channel 快照声明 `supports_delivery_lookup=supported)，且该精确版本 Adapter 实现查询 Port 时，Registry 才允许解析；声明与实现不一致、能力为 unknown/unsupported、版本不存在时均失败关闭。

`BuildChannelDeliveryLookupRequest` 只接受状态为 unknown 的 Delivery、同一 channel_ref/config_version 的 Channel 快照，以及该 Delivery 最后一次已完成且结果为 ambiguous 的 Attempt。请求身份由这三份快照构造，调用方不得自行替换 DeliveryID、AttemptID 或配置版本。只有 `supports_idempotency=supported` 时才携带 Delivery 稳定幂等键；Attempt 已有 Provider Message Ref 时可作为辅助查询键。请求不携带收件人明文地址、模板正文或完整发送请求。

`ChannelRegistry.LookupUnknownDelivery` 是当前查询到对账证据的受控入口：它通过精确版本 Registry 调用 Adapter，校验结果身份与请求一致后，才转换为 `UnknownDeliveryEvidence`。查询超时、认证失败、限流、解析失败等错误原样返回，不能转换为 `not_accepted`；三态结果分别映射为 accepted/not_accepted/indeterminate。Adapter 对 Provider 最终一致性窗口的判断属于具体渠道实现责任；窗口内“暂未查到”必须返回 indeterminate。

该入口只保证模块内部调用边界与身份绑定，不宣称 Go 类型系统能证明 Adapter 查询事实真实或阻止同进程内部代码伪造证据。权威性仍依赖受控 Adapter 实现、正式 Provider 查询语义和逐渠道 Contract Test。查询结果后续仍由既有 `AssessUnknownDelivery`、RetryPolicy 与 Delivery.version CAS 收敛状态；本 Port 不直接修改 Delivery 状态。
