# Researchflow 模块卡点

更新时间：2026-11-18  
对应分支：`feat/researchflow`  
当前代码 HEAD：`dc603d97eea867d56aa4ff0600ae37d496805b77`  
维护范围：仅记录 Researchflow 推进所依赖的跨模块决策、契约同步和验收阻塞；不替代模块任务状态文档。

## 1. 当前卡点

### RF-BLOCK-001：项目级时间规则修订尚未同步到 Researchflow 分支

- **现状**：项目级契约修订已提交在 `feat/design_doc`，HEAD 为 `bc0d11a3bb067d097195b345a32dbc93bef7525b`。GOV-CTR-002 和 GOV-CTR-004 均升级至 1.1.0，决策记录新增 DEC-PLAT-002-054。
- **原因**：Researchflow 当前分支仍基于较早的项目级契约登记版本；项目级权威契约必须由契约 Owner 合并/同步，模块分支不能自行复制或修改 `doc/contract/**`。
- **影响**：Researchflow 可依据本轮已确认的规则方向继续实现 UTC 输出规范化，但在项目级契约进入共同集成基线前，模块最终验收仍需核对统一版本及引用关系。
- **处理责任**：项目契约 Owner / Integration。
- **解除条件**：将 `feat/design_doc` 的契约修订纳入项目共同基线，并确认 GOV-CTR-002 1.1.0、GOV-CTR-004 1.1.0、PLAT-002 与 README/变更记录一致。
- **当前状态**：OPEN；不阻断本模块继续做兼容实现和测试。

### RF-BLOCK-002：已发布 Run Schema 1.0.0 的接受范围不能原地收紧

- **现状**：Run/Stage/Attempt Schema 的时间字段使用 JSON Schema `format: date-time`；Run Schema 对终态要求非空 `result_summary_ref`，但没有禁止非终态携带非空引用。
- **原因**：GOV-CTR-004 规定收紧已发布 Schema 的接受范围属于 MAJOR。直接增加 UTC Z 正则或非终态引用约束，会改变 1.0.0 的接受范围。
- **影响**：不能简单修改 1.0.0 Schema/Golden 并仍声称版本不变；也不能把 Schema 当前接受 offset 等同于生产方可以输出 offset。
- **当前绕行**：生产 DTO 映射层已统一将所有绝对时间转为 UTC；新增偏移时区反例测试，保证生产输出为 `Z`。该修改不收紧 Schema 接受范围。
- **待决事项**：是否需要发布 Run/Stage/Attempt Schema 2.0.0，以机器校验 UTC Z 和非终态 `result_summary_ref=null`。若现有生产/消费链路通过 DTO 输出测试与运行时不变量已足够，则可不为形式上的 Schema 正则单独升版。
- **处理责任**：项目契约 Owner 与 Researchflow；按真实消费者兼容影响决策。
- **当前状态**：OPEN；UTC 输出实现已提交，Schema MAJOR 升版不是当前代码修复的前置条件。

### RF-BLOCK-003：FLOW-CTR-004 Envelope 登记状态与模块资产不一致

- **现状**：Researchflow 分支已有 Stage Provider Result Envelope Schema/Golden、精确 Registry 和 Worker 提交前校验；模块总规划记录项目登记簿仍写 Envelope Schema 未登记。
- **原因**：项目级登记簿与模块分支资产的同步责任不在 Researchflow 模块文档中解决。
- **影响**：契约定义资产已有实现，但不能仅凭模块文件存在就宣告项目级登记和跨模块消费者验收全部完成。
- **处理责任**：项目契约 Owner / Integration 核对登记簿与真实冻结版本。
- **解除条件**：核对权威 Schema/Golden 路径、版本、Registry 约束和登记簿记录一致。
- **当前状态**：OPEN；不阻断当前时间规范化任务。

## 2. 本模块已继续推进的工作

- `CreateRunService` 通过 `DatabaseClock.NowUTC` 获取当前时刻，并由同一时刻生成 `system_as_of` 与 `deadline_at`；调用方不传入这两个值。
- `MySQLDatabaseClock` 复用 Platform MariaDB UTC Source，不重复实现数据库时钟。
- Run/Stage/Attempt DTO 投影现对必填时间调用 `UTC()`，对可空时间指针统一复制并转 UTC。
- 新增非 UTC（+08:00）来源时间的 DTO 序列化测试，要求输出均以 `Z` 结尾。
- 前一实现 SHA 的 GitHub Actions Run 44290441307 失败，原因已定位为字符串指针复制函数误改，并已在 `6017867d0652a5d8bd94caef44f71887961bd864` 修复；当前 HEAD CI 尚待完成，在成功前不标记通过。

## 3. 下一步解除顺序

1. 等待当前精确 SHA 的 Go Test Suite 完成；失败则按测试证据修复。
2. 对照 GOV-CTR-002 1.1.0，复核 Run/Stage/Attempt 所有公开时间字段和其他输出入口，确认没有绕过 DTO 映射的序列化路径。
3. 核实 Run 非终态 `result_summary_ref` 的 Domain/Application 不变量；先补运行时测试，不直接改 Schema 1.0.0。
4. 由项目契约 Owner/Integration 同步 GOV-CTR-002/004 1.1.0，并核对 FLOW-CTR-004 Envelope 登记状态。
5. 再处理 FLOW-001 命令/执行上下文机器契约缺口，以及 FLOW-002 生产 Registry 的真实注册盘点。

## 4. 状态定义

- `OPEN`：有明确责任人和解除条件，当前未解除。
- `IN_PROGRESS`：正在按解除条件处理。
- `RESOLVED`：有对应 SHA、测试或权威契约证据证明解除。
- 卡点不等于整个模块停止；只暂停依赖该卡点的具体变更。
