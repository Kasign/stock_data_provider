undefined

## 9. Python 运行时下线约束（2026-11-02）

- 本仓库后续目标是移除 Python 运行时、Python 源码及 Go→Python 调用，不再新增 Python Provider、脚本或 Python CI。
- Pytdx 是 Go 原生通达信协议客户端（`github.com/injoyai/tdx`），不依赖 Python，继续保留；后续文档统一称“Go 通达信协议 Provider（历史名称 Pytdx）”。
- Python bridge 当前仍被旧 `go-core` 分析流程用于 LLM、搜索、通知、持久化和部分辅助数据回退。必须先完成 Go 能力迁移，再删除 bridge 和 Go 适配器。
- 数据质量约束迁移到 `doc/Go数据质量契约.md`，不再依赖 Python/Go 对照 CI。
- 分阶段清单以 `doc/Python运行时下线计划.md` 为准。
