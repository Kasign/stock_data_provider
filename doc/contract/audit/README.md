# PLAT-002 接口级审计

本目录记录十模块项目级接口契约审计的过程证据、差异、补证和三轮 Review 结论。它是审计工作区，不取代 `doc/contract/` 中的契约语义、状态登记和决策权威。

## 审计范围

正式十模块：Platform、Server、Data、News、Research、Researchflow、Quant、LLM、Notification、WebUI。

六个核验维度：
1. Application Port 与真实调用点。
2. DTO、Schema、Golden 与字段语义。
3. Fault、Diagnostics、Request ID 映射。
4. AuthorizationRef、Secret 和资源授权。
5. Repository、Migration、Worker 和生命周期所有权。
6. 模块间依赖边界。

## 基线与执行规则

- 固定代码基线：`origin/integration` @ `b6dfc85ba214fe16268c8cba780641a8bfe15086`。
- 审计文档维护分支：`origin/feat/design_doc` @ `671b76db8879766f3b43990406eb0a1216a53424`（开始本轮文档写入前）。
- 不修改业务代码，不修改其他分支，不合并其他分支。
- 四个正在修改的模块：Server、Data、Researchflow、WebUI。它们的未合入改动不属于固定基线；只登记观察，不阻塞其余模块核验。
- 若后续变更触及 Port、DTO/Schema、Fault、Diagnostics、Request ID、AuthorizationRef、Secret、Repository、Migration、Worker、生命周期或模块依赖，必须建立差异记录并判断是否需要补审。
- 所有结论必须标注代码 SHA、文件路径和符号/行范围或测试证据。抽样不能表述为全量完成。
- 设计决议、实现存在、测试通过和契约冻结分别记录。

## 文档索引

- [审计基线](./PLAT-002-审计基线.md)
- [十模块核验总进度](./PLAT-002-十模块核验总进度.md)
- 后续差异矩阵、待裁决清单、补证清单和三轮 Review 记录在本目录逐步建立。
