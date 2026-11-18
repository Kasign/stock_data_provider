# FLOW-005：Context Assembly 与 Tool Policy

## 阶段结论

Context 分区规划、稳定排序、Token Budget、必选/受保护内容 fail-closed、Research DTO 投影、Tokenizer Port/Provenance、最终输入复核、Tool Registry/Scope/参数 Schema/调用预算和 LLM Tool Call 意图适配已有模块实现。它们目前是纯函数/适配切片，不代表生产 Context Stage 已接入。

## 已有证据

- Researchflow：`internal/researchflow/contextassembly/**`、`internal/researchflow/toolpolicy/**`
- 实施说明：`doc/researchflow/05-FLOW-005上下文装配实施说明.md`
- Research 分支：`feat/research` 已有 Context Domain、Schema/Golden、Application Port（`CreateContext` / `GetContext`）和 Storage Repository。
- LLM 分支：`feat/llm` 已有 ModelRequest、Provider、Schema 校验和 Tool Call 结构；当前分支树未见生产 Tokenizer 实现。
- Server 装配检查：Researchflow 当前 Server bootstrap/composition 文件没有导入 `contextassembly` / `toolpolicy`，也没有调用 Research Context Application Port；当前没有生产 Context Stage 接线证据。

## 生产接入前必须解决

1. **Research Owner 当前授权**：由资源 Owner Adapter 获取并复核当前可见的 Evidence/Section；Researchflow 不访问 Research Repository，也不把未经授权正文传给模型。
2. **Run PIT 绑定**：`ResearchContextSnapshot.SystemAsOf` 当前只校验非零 UTC，不会自动与 Run.SystemAsOf 比较。Server/Worker Adapter 在 CreateContext 前必须比较二者相等；读取 Context 后再次验证 Context.SystemAsOf 与 Run.SystemAsOf 一致。Research Context 自身 Validate 已要求每个 SectionRef.SystemAsOf 与 Context.SystemAsOf 一致。
3. **Run Deadline 绑定**：构造 LLM ModelRequest 时，SystemAsOf/DeadlineAt 必须直接继承同一个 Run；不能用 Context.CreatedAt、模型调用时刻或本机时钟重新生成。LLM ModelRequest Validate 只能验证 UTC/基本时间边界，不能证明它关联的是正确 Run。
4. **生产 Tokenizer**：当前仅有 Tokenizer 接口与测试替身。必须由目标模型/Provider 提供真实 Tokenizer，绑定 Model ID、Tokenizer Version，并验证 Section、固定预留和最终序列化 JSON 的准确计数。
5. **Server/Worker 接线**：注册真实 Context Stage，注入 Research Port、Owner Adapter、LLM Tokenizer/Port 和 Run Context；补跨模块 Contract Test 与真实模型调用证据。

## 职责边界

- Research：拥有 Context 领域模型、质量语义、Schema/Golden、Application Port、持久化。
- Researchflow：拥有一次 Run 的分区选择、排序、预算、裁剪、Research DTO 投影、模型输入组合和 Tool Policy。
- LLM：拥有 Provider/ModelRequest 和目标模型 Tokenizer 实现。
- Server：作为 Composition Root 装配 Researchflow、Research、LLM 和各 Owner Adapter；Worker 负责执行绑定 Run/Stage/Attempt 的流程。

## 验收状态

- 纯函数/DTO 投影/预算逻辑：已有实现，按当前模块 CI 验收。
- 生产 Research Owner 授权 Adapter：未完成。
- Run SystemAsOf/Deadline 端到端绑定：未完成。
- 目标模型生产 Tokenizer：未完成。
- Server/Worker Context Stage 接线：未完成。
- 真实 LLM 调用和预算准确性：未完成。

## 2026-11-18 Review 结论

Research Context 的持久化不是 Researchflow 缺失能力，Research 模块已有该能力；当前真正缺口是跨模块 Adapter/授权/Run 时间绑定/Tokenizer/Server Worker 接线。不得在 Researchflow 内重复建设 Context Repository，也不得以测试 Tokenizer 代替生产模型计数。


## 2026-11-18 Context Assembly UTF-8 输入边界补强

复核发现 Context Assembly 将 Research Question、SubjectRefs、Section 内容等字段传入 JSON 序列化；Go JSON 编码器会将无效 UTF-8 替换为 U+FFFD，可能导致上下文正文、引用或模型输入失真。原有空白、重复项和预算校验不能代替编码有效性校验。

本轮增加有效 UTF-8 校验：

- ResearchContextSnapshot 的 ResearchQuestion、Scope、BuilderVersion、SubjectRefs、QualitySummary.ReasonCodes。
- SectionCandidate 的 ID、SourceOwner、SourceRefs、EvidenceRefs、ProtectedEvidenceRefs。
- SectionMetadata.Version。
- Tokenizer 的 ModelID、Version，以及 SectionContent 的 SectionID/Text。
- BuildPlan、Research DTO 投影、Token 估算和最终 ModelInput 序列化入口均执行对应校验，避免只在某一条调用路径上拒绝。

新增负向测试覆盖候选字段、快照字段、Metadata 和 Section 内容。代码 SHA：`e6e941c71c9960b6675ce846df1b621a16064f06`；Run [44311895491](https://github.com/Kasign/stock_data_provider/actions/runs/44311895491) 最终结论 success：Researchflow format/tests/race/vet、WorkerStore unit/race/vet、MariaDB lifecycle integration、根模块 Test/Race/Vet、go-core、Research MariaDB 和 Notification MariaDB 均通过；Market Data、News 专项集成 Job skipped。

该修复只负责输入编码完整性，不替代 Research Owner 授权、Run PIT/Deadline 绑定或 LLM 生产 Tokenizer。
