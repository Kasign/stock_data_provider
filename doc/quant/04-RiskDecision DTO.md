# QNT-CTR-004 RiskDecision DTO

## 实现范围

实现 Quant 风险决策公开 DTO 的强类型结构、基础字段校验、Draft 2020-12 Schema 和 Golden。依据总规划中的 RiskDecision / RiskReason 字段表，不修改跨模块契约。

## 字段与约束

- Decision：decision_id、status、rule_set_version、evaluated_at、system_as_of、reasons、input_refs。
- status：pass、reject、unknown。
- Reason：code、severity、status、safe_message、evidence_refs。
- severity：block、warn、info；Reason status：triggered、not_triggered、unknown。
- 时间采用规范 UTC RFC3339Nano；Schema 与 Go DTO 同步拒绝非 UTC、冗余小数零和超过纳秒精度的时间表示；引用按不透明非空字符串处理。
- required 数组必须显式存在；契约未规定最少元素数，因此允许空数组。
- Schema 禁止未声明字段，不接受未冻结的 override 扩展。

## 领域边界

DTO Validate 只校验结构、枚举、必需字段和时间格式，不推导 status 与 reasons 的业务组合。风险规则的 fail-closed 决策由 QNT-010 Application 执行：数据缺失、规则无法评估或输入质量不足时必须 unknown 或 reject，不能 pass；硬门禁不允许评分或 LLM override。

## 验收

Go 强类型 DTO、单元测试、Draft 2020-12 Schema 编译测试和 Golden 校验已加入源码。尚未取得当前 SHA CI 证据；本阶段不代表 QNT-010 规则引擎、真实数据风险评估或运行时授权验收完成。
