# QNT-CTR-004 RiskDecision DTO

## 实现范围

实现 Quant 风险决策公开 DTO 的强类型结构、基础字段校验、Draft 2020-12 Schema 和 Golden。依据总规划中的 RiskDecision / RiskReason 字段表，不修改跨模块契约。

## 字段与约束

- Decision：decision_id、status、rule_set_version、evaluated_at、system_as_of、reasons、input_refs。
- status：pass、reject、unknown。
- Reason：code、severity、status、safe_message、evidence_refs。
- severity：block、warn、info；Reason status：triggered、not_triggered、unknown。
- 时间采用规范 UTC RFC3339Nano；Schema 与 Go DTO 同步拒绝非 UTC、冗余小数零和超过纳秒精度的时间表示；引用按不透明非空字符串处理。
- Go 字符串可承载任意字节，但 JSON 字符串必须是有效 UTF-8；DTO 对所有非空字符串字段（身份、消息和引用）拒绝非法 UTF-8，避免序列化时替换字节导致内容失真。
- required 数组必须显式存在；契约未规定最少元素数，因此允许空数组。
- Schema 禁止未声明字段，不接受未冻结的 override 扩展。

## 领域边界

DTO Validate 只校验结构、枚举、必需字段和时间格式，不推导 status 与 reasons 的业务组合。风险规则的 fail-closed 决策由 QNT-010 Application 执行：数据缺失、规则无法评估或输入质量不足时必须 unknown 或 reject，不能 pass；硬门禁不允许评分或 LLM override。

## 验收

Go 强类型 DTO、单元测试、Draft 2020-12 Schema 编译测试和 Golden 校验已加入源码，并由 Quant 独立 Workflow `42598371072` 在 SHA `471dae4e8b55e999719a691c7761df3cee8f8e8c` 上执行通过（覆盖 Quant 全包测试、gofmt、Manifest 30 秒随机 Fuzz、Race 和 Vet）。当前分支 HEAD `41fc0dc0dcffae90f3ba82a4179729d0d348ceb8` 后续只有文档提交，没有 Quant 代码变化；该证据不是对当前 HEAD 直接触发的 CI。本阶段不代表 QNT-010 具体领域规则、真实数据风险评估、可信授权或跨模块集成验收完成。
