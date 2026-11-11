# PLAT-002 五项公开契约待决裁决建议稿

- 状态：`proposal_pending_project_owner_confirmation`
- 适用范围：仅 QNT-CTR-004、QNT-CTR-005、RSC-CTR-002 的五项已登记待决
- 维护分支：`feat/design_doc`
- 代码审计基线：`origin/integration @ b6dfc85ba214fe16268c8cba780641a8bfe15086`
- 目的：将现有冲突收敛为可一次确认的明确语义；本建议稿不是正式决议，不改变任何正式 CTR 状态

## 裁决原则

1. 只解决已登记的五项公开契约语义，不扩展到算法、存储、测试、CI、部署或模块实现。
2. 不新增 CTR，不改变生产方领域所有权。
3. 与既有已接受定义一致的部分不重开；只补足字段身份、引用方向和适用条件。
4. 项目负责人确认前，五项继续保持 `pending-decision`；不得将建议稿视为已接受决议或冻结证据。

## 一、QNT-DEC-01：Envelope 与 Payload 的版本字段

**归属：** QNT-CTR-004。

**现有冲突：**
- Quant-Research 字段契约第 2.7 节将 QuantResult 的 `schema_version` 定义为 Envelope Schema 版本。
- 同一节又要求以 `result_type + schema_version` 选择 Payload 子 Schema。
- QNT-CTR-005 Result Manifest 的 `schema_version` 参与结果身份指纹，但没有说明它是 Manifest、Envelope 还是 Payload 的版本。
- QuantResultRef 的 `schema_version` 又被定义为 Ref DTO 自身版本。三个不同版本身份不能继续共用一个字段名。

**建议裁决：**
1. QuantResult Envelope 的 `schema_version` 只表示 Envelope DTO/Schema 版本。
2. 新增明确字段 `payload_schema_version`，表示强类型 Payload 的版本；Payload Schema 的唯一选择键为 `(result_type, payload_schema_version)`。
3. QuantResultRef 的 `schema_version` 只表示 QuantResultRef DTO 版本。当前没有明确消费者必须仅凭 Ref 预先选择 Payload Validator 的需求，因此本次不为 Ref 增加 `payload_schema_version`；如未来出现该需求，应另行评估 Ref 契约变更，不能复用 Ref DTO 版本。
4. QNT-CTR-005 Manifest 1.0.0 的 `schema_version` 明确定义为 Payload 版本，即 `payload_schema_version` 的历史字段名；它既不是 Manifest Schema 版本，也不是 QuantResult Envelope 版本。Manifest Schema 自身版本继续由 JSON Schema `$id` 管理。后续 Manifest Schema 新版本应改用明确字段名 `payload_schema_version`，并通过版本迁移处理，不得在 1.0.0 中静默改名或改变语义。
5. Payload Schema 的唯一选择键统一为 `(result_type, payload_schema_version)`。未知 Envelope 版本或未知 Payload 版本均拒绝，不得回退到其他版本。Envelope 版本升级不自动意味着 Payload 版本升级，反之亦然。

**取舍：**
- 优点：保留现有 Envelope `schema_version` 和 Manifest 1.0.0 的既有字段，不引入双重解释；版本身份可独立演进。
- 代价：Envelope/Ref DTO 需要增加 `payload_schema_version`，Schema/Golden 的后续更新需体现该字段。
- 不建议：让一个 `schema_version` 同时代表 Envelope 与 Payload；也不建议把 Schema 文件名中的版本当作运行时 Payload 版本而不在 DTO 中表达。

## 二、QNT-DEC-02：白名单纯函数的空输入快照引用

**归属：** QNT-CTR-005。

**现有冲突：**
- QuantAnalysisRequest 字段契约将 `input_snapshot_refs` 定义为必填数组，但允许白名单纯函数传入空数组。
- QuantResult Envelope 允许相同纯函数场景使用空数组。
- QNT-CTR-005 Manifest 1.0.0 Schema 当前要求 `input_snapshot_refs.minItems = 1`。
- 因此 Request、Envelope 与 Manifest 对空输入的表达不完整一致，且 Manifest Schema 会拒绝已被上层允许的合法结果。

**建议裁决：**
1. 允许满足以下全部条件的纯函数生成可持久化、可引用的 QuantResult 和 Manifest：Quant Registry 明确登记该计算为无外部事实输入的纯函数；ResolvedTemplate 明确声明 `allow_empty_inputs=true`；实际执行的算法、参数和模板均来自已解析且受控的定义。调用方不得自行声明或覆盖上述授权。
2. `QuantAnalysisRequest`、QuantResult Envelope 与 Result Manifest 必须对输入模式采用一致语义：`snapshot` 模式要求 `input_snapshot_refs` 至少包含一个有效不可变 Snapshot Ref；`none` 模式要求该数组为空，且只能由上述白名单纯函数触发。字段必填性与条件约束应在各层保持一致，不能出现 Request/Envelope 接受而 Manifest Schema 拒绝的情况。
3. 以必需的 `input_mode`（`snapshot` / `none`）明确表达模式；若现有权威 DTO 已有等价且无歧义的机器字段，可复用该字段，不重复造字段。不得仅凭调用方提交空数组推断 `none`。
4. Envelope 与 Manifest 的 `input_mode` 必须一致。`input_mode`、模板身份及版本、算法身份及版本、规范化参数指纹和 `system_as_of` 等既有结果身份要素必须共同参与结果身份/指纹计算；空数组是明确的无外部事实输入，不代表缺失、未知或未采集。
5. 非纯函数、依赖行情/新闻/研究证据或其他外部事实的计算，仍必须提供相应 Snapshot 引用；缺少引用时拒绝生成可引用结果。Manifest Schema 应通过条件约束表达 `snapshot` 与 `none` 的数组约束，而不是全局放宽后失去校验能力。

**取舍：**
- 优点：保留已接受的白名单纯函数能力，同时不放松有事实输入计算的可追溯要求。
- 代价：Manifest 机器契约需表达条件约束，且消费者必须校验输入模式与 ResolvedTemplate/Registry 声明一致。
- 不建议：全局把 `minItems` 改为 0 而不增加条件约束；也不建议禁止所有无输入纯函数持久化，因为这会推翻已接受的 Template 白名单规则。

## 三、RSC-OPEN-003：ResearchEvent 时间规范化规则版本

**归属：** RSC-CTR-002。

**现有缺口：**
ResearchEvent 已保留 `source_times[]`，并将 `occurred_at`、`published_at`、`available_at` 定义为规范化时间；但 DTO 未标识用于从来源时间选择/归一化规范时间的规则身份。该规则身份不能与 Event 版本、DTO Schema 版本或 Provider 版本混用。

**建议裁决：**
1. ResearchEvent 增加必需字段 `time_normalization_rule_id` 与 `time_normalization_rule_version`，分别标识规则集身份及其版本；规则 ID 由 Research 领域定义和管理。
2. 该规则集覆盖规范时间字段的选择优先级、来源冲突处理、时区/精度处理及无法确定时输出 null 的规则；不表示某个 Provider 的解析器版本。
3. `source_times[]` 保留逐来源原始时间证据；规范化结果不得覆盖或删除来源值。
4. 即使没有可靠来源时间、规范化时间为 null，也必须记录所应用的规则 ID 与版本；不得因结果为 null 而省略。
5. 规则发生语义变化时升级对应规则版本；规则身份改变时使用新的规则 ID。历史 Event 版本保留原规则身份及版本，不因规则升级而回写。

**取舍：**
- 优点：历史 Event 可解释、可复核，避免把当前规则误用于历史记录。
- 代价：ResearchEvent DTO 增加一个必需版本字段；规则内容由 Research 维护。
- 不建议：复用 `schema_version`、`event_version` 或 Provider `source_version` 表达该语义。

## 四、RSC-OPEN-004：ResearchEvent 合并/拆分来源关系

**归属：** RSC-CTR-002。

**现有定义与缺口：**
ResearchEvent 已有 `evidence_refs`，语义是当前 Event 的支持/反证依据；`source_times[]` 保存来源级时间证据。项目字段契约第 9.9 节另有 `source_refs` 与 `derived_from_refs` 的表述，但 DTO 字段表未定义其类型和引用方向。若 `source_refs` 只是重复列出 Evidence 来源，将形成两套来源事实；若指任意对象，又会形成无类型约束的多态引用。

**建议裁决：**
1. 本次不新增 `source_refs`。现有 `evidence_refs` 继续作为当前 Event 的支持/反证依据及其来源材料引用入口；不得再定义一个语义相同的来源数组。若后续发现某类直接来源材料不属于 Evidence、且不能由 Evidence 引用表达，应以具体消费场景和对象类型另行提出，不预先增加泛化字段。
2. 增加 `derived_from_refs`，仅表示当前 ResearchEvent 版本直接派生自的前序 ResearchEvent 不可变版本引用；不得把 Evidence 或原始来源材料放入该字段。
3. 合并：新 Event 的 `derived_from_refs` 可包含多个前序 Event 版本；每个前序 Event 均保留原记录，不做覆盖或删除。
4. 拆分：每个新 Event 的 `derived_from_refs` 指向被拆分的前序 Event 版本；多个子 Event 可引用同一前序版本。各子 Event 的 `evidence_refs` 只保留实际支持或反证该子事件的证据。
5. `derived_from_refs` 为去重 Ref 数组，引用必须精确到不可变 Event 版本。空数组表示不存在直接前序 Event 派生关系；若关系未被计算/提供，不得用空数组伪装确认无关系，必须由契约已有的可用性/完整性语义表达。
6. 普通状态迁移或描述修订不自动构成合并/拆分；只有实际发生 Event 派生关系时才填充该字段。

**取舍：**
- 优点：避免 `source_refs` 与 `evidence_refs` 重复，且把 Event 血缘限定在单一、强类型身份空间。
- 代价：ResearchEvent Schema 需要定义 `derived_from_refs` 的强类型 Ref 结构和引用合法性。
- 不建议：使用一个可指向 News Article、ResearchEvent 和任意对象的多态来源数组；也不建议把合并/拆分关系只写在自由文本中。

## 五、RSC-OPEN-005：Claim/Hypothesis 的 supersedes_version

**归属：** RSC-CTR-002。

**现有缺口：**
项目字段契约第 9.9 节已要求修正声明 `supersedes_version`，但 Claim/Hypothesis DTO 字段表未列出该字段，也未明确引用作用域。

**建议裁决：**
1. Claim/Hypothesis DTO 增加可选整数 `supersedes_version`。
2. 字段只引用同一 `claim_id` 的一个既有版本，不得引用其他 Claim/Hypothesis 的版本；必须满足正整数且小于当前 `claim_version`。
3. 当新版本是对既有陈述/结论的修正或替代时必须提供；仅追加证据、补充限制或状态变化且不替代原陈述时可以省略。
4. 初始版本不得提供该字段；不得通过该字段删除、覆盖或改写历史版本。
5. ResearchEvent 继续使用已有的 `supersedes_event_version`，不强行统一不同领域 DTO 字段名；两者语义相同但引用身份空间不同。

**取舍：**
- 优点：落实既有版本治理原则，避免 Claim/Hypothesis 的修订关系丢失。
- 代价：Claim/Hypothesis Schema 需要增加字段及版本关系约束。
- 不建议：把 `supersedes_version` 定义为跨对象任意引用，或把它等同于 `claim_version`。

## 一次性确认清单

| 编号 | 推荐结论 | 当前状态 |
|---|---|---|
| QNT-DEC-01 | Envelope `schema_version` 与 Payload `payload_schema_version` 分离；Manifest 1.0.0 的 `schema_version` 明确为 Payload 版本 | pending-decision |
| QNT-DEC-02 | 仅白名单纯函数允许空输入；Request、Envelope、Manifest 统一表达 input_mode 和条件约束 | pending-decision |
| RSC-OPEN-003 | 增加必需 `time_normalization_rule_id` + `time_normalization_rule_version` | pending-decision |
| RSC-OPEN-004 | 不新增与 `evidence_refs` 重复的 `source_refs`；新增强类型 `derived_from_refs` 指向前序 ResearchEvent 版本 | pending-decision |
| RSC-OPEN-005 | Claim/Hypothesis 增加可选 `supersedes_version`，修正/替代时必需且只指向同一对象较早版本 | pending-decision |

## 确认后的处理边界

项目负责人确认后，才把接受的条目转为正式决议，并更新本登记簿、差异矩阵和十模块进度中的对应状态及字段定义引用。责任模块的 Schema/DTO/实现更新不属于本分支本轮任务，不在此处执行。

项目负责人如不接受某项建议，只需指出该项编号及替代语义；该项保持 pending-decision，其他已确认项可单独收敛。
