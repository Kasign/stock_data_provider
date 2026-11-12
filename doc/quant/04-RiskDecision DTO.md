# QNT-CTR-004 RiskDecision DTO

## 实现范围

实现 Quant 风险决策公开 DTO 的强类型结构、基础字段校验、Draft 2020-12 Schema 和 Golden。依据总规划中的 RiskDecision / RiskReason 字段表，不修改跨模块契约。

## 字段与约束

- Decision：decision_id、status、rule_set_version、evaluated_at、system_as_of、reasons、input_refs。
- status：pass、reject、unknown。
- Reason：code、severity、status、safe_message、evidence_refs。
- severity：block、warn、info；Reason status：triggered、not_triggered、unknown。
- 时间采用规范 UTC RFC3339Nano；Schema 与 Go DTO 同步拒绝非 UTC、冗余小数零和超过纳秒精度的时间表示；引用按不透明非空字符串处理。
- Go 字符串可承载任意字节，但 JSON 字符串必须是有效 UTF-8；DTO 对所有非空字符串字段（身份、消息和引用）拒绝非法 UTF-8 与首尾空白，避免序列化替换字节或同一逻辑身份出现多种文本表示。
- `reasons` 必须显式存在且至少包含一条 Reason；`input_refs` 必须显式存在，但允许空数组；每条 Reason 的 `evidence_refs` 必须显式存在，允许空数组。
- Schema 禁止未声明字段，不接受未冻结的 override 扩展。

## 领域边界

DTO Validate 校验结构、枚举、Reason.code 唯一性、`evaluated_at >= system_as_of` 和 status 聚合结果；不替代 RuleSet 对 Reason severity/顺序/覆盖率的校验。风险规则的 fail-closed 执行由 QNT-010 Application 与 RuleSet 执行入口共同保证：数据缺失、规则无法评估或输入质量不足时必须 unknown 或 reject，不能 pass；硬门禁不允许评分或 LLM override。

## 验收

Go 强类型 DTO、单元测试、Draft 2020-12 Schema 编译测试和 Golden 校验已加入源码，并由 Quant 独立 Workflow `42618742241` 在代码 SHA `cda200a1e4a3673bb4a34cd4b914d32f0a3be823` 上执行通过（覆盖 Quant 全包测试、gofmt、Manifest 30 秒随机 Fuzz、Race 和 Vet）。当前分支 HEAD `c104fb71ebcf9360b3306a8a89ca40b26131fbc6` 在该代码验收 SHA 之后只有文档提交，没有 Quant 代码变化；该证据不是对当前文档 HEAD 直接触发的 CI。本阶段不代表 QNT-010 具体领域规则、真实数据风险评估、可信授权或跨模块集成验收完成。
