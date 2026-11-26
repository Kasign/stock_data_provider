# 项目长期记忆

- 仓库强制协作规则：`/Users/Walg/CodeGit/Private/stock_data_provider/AGENTS.md`（2026-11-26 新增）。在本仓库执行任何工作前必须读取并遵守；与既有治理文档冲突时以该文件为准。
- 当前工作模式：`repair_all` 集中开发；只允许更新当前分支，禁止切换分支、合并其他分支、rebase、强制推送或重置历史。
- 规则文档关联：根目录 `README.md`、`doc/整体任务规划/05-治理与验收/GOV-005模块独立开发与集成治理规范.md`、`doc/整体任务规划/06-联调收尾/README.md` 已建立指向。
- Quant 输入错误分类约定（2026-11-26 起）："输入存在但不可用"→ 各 Facade 的 `ErrXxxSnapshotQuality` + `*InputNotApplicableError`（唯一 reason code，Stage 层投影为 `quant_{sma20|factor|trend}_input_*`）；"Snapshot 自身不自洽"→ `ErrInvalidXxxSnapshot` 且不带 reason code。两类都 fail-closed，不要合并。
