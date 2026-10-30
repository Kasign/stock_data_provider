# Go 数据服务迁移状态

- 适用分支：`feat/full-go-data-providers-api-switch`
- 静态盘点基线：`4c603172448102058f95e6bc625494436a8402c9`
- 更新日期：2026-10-30
- 状态口径：只记录当前代码/配置可证明的状态，不把静态实现当作真实生产可用。

## 1. 总体结论

Go 原生行情服务已覆盖国内 A 股多个主要数据域，具有独立 Provider 接口、按能力注册、配置开关、服务层回退、数据质量归一化、缓存和 MariaDB 持久化。

当前不能宣布 Python → Go 全量迁移完成，原因包括：

1. 原 Python DataFetcherManager、Efinance、AkShare 的完整源码和固定版本未纳入当前仓库，尚不能逐函数还原真实旧调用链。
2. 当前只完成了代码/配置静态盘点，尚未完成所有数据域的同样本字段差分。
3. 部分 Go Provider 代码已实现但默认关闭，权限、限流和字段覆盖尚未统一验收。
4. 市场情绪历史、市场级资金流、全市场批量概览、严格 PIT 等仍有明确缺口。
5. Python bridge、旧 Python API 适配器和 Go Core 兼容桥仍保留，不能据此认定 Python 运行时已全面下线。

## 2. 当前实现基线

| 层级 | 代码位置 | 当前判断 |
|---|---|---|
| Go 行情服务入口 | `services/market-data/cmd/market-data/main.go` | Provider 实例化、按配置启用、按能力接口注册 |
| Provider 接口 | `services/market-data/internal/provider/provider.go` | 已拆分 quote、daily bars、财务、估值、资金流、筹码、指数等独立能力 |
| Provider 配置 | `services/market-data/configs/providers.json`、`services/market-data/internal/config/config.go` | 支持 Provider 优先级和逐能力开关；妙想为独立补充源配置 |
| 服务层 | `services/market-data/internal/service` | 已有多数据域服务和 fallback 测试 |
| HTTP API | `services/market-data/internal/httpapi`、`services/market-data/openapi.yaml` | 已提供主要行情 API；以路由、Handler、OpenAPI 联合确认能力 |
| 质量归一 | `services/market-data/internal/normalize`、各 Provider 实现 | 已有单位、复权、空值、数值边界及质量状态保护 |
| Python bridge | `python_bridge/capabilities/market_data.py` 等 | 仍保留兼容/辅助调用；行情数据主链已有 Go 实现，但不能称 Python 全面下线 |

## 3. 阶段一静态盘点结论

### 已实现且默认启用的主要能力

- 实时行情：东方财富、腾讯、新浪、网易、Pytdx。
- 日 K：东方财富、BaoStock、腾讯、新浪、网易、TickFlow、Pytdx。
- 股票搜索：东方财富、腾讯。
- 指数行情、市场宽度、板块排行：东方财富。
- 交易日历、财务：BaoStock。
- 涨跌停池、龙虎榜、公告、个股资金流、筹码、人气榜：东方财富为主要默认来源；妙想需独立配置。

### 已实现但默认关闭/需要权限验证

- TickFlow：实时行情、指数行情、市场宽度、板块排行。
- 同花顺：行情、日 K、指数、交易日历、人气榜、涨跌停池、龙虎榜、财务、估值、指数目录等。
- Tushare：证券基础资料、交易日历、交易状态、财务、估值、涨跌停池、龙虎榜、公告、指数目录及历史成分等。
- 雪球：人气榜。
- Yahoo Finance：海外行情能力，当前范围暂缓。

具体能力与开关以 `PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md` 为准。不得只因为方法存在就直接打开配置。

## 4. 尚未完成事项

| 优先级 | 事项 | 当前状态 | 下一步 |
|---|---|---|---|
| P0 | Python/Go 固定样本字段对账 | 缺少统一样本与完整旧版源码证据 | 固定旧版上游、建立脱敏样本和差异报告 |
| P1 | 已实现但关闭的 Provider 能力 | 代码可见，权限/真实可用性未完成统一验收 | 分 Provider、分 capability 验证后再启用 |
| P1 | 全市场批量概览 | 仅有指定股票批量行情，非全市场扫描 | 定义证券范围、分页、字段、MA 和质量契约 |
| P1 | 市场情绪历史 | 当前市场宽度快照不能替代历史情绪 | 定义指标、覆盖分母、采集时点和持久化 |
| P1 | 市场级资金流 | 当前主要是个股资金流 | 明确净流入口径、范围、时间和历史序列 |
| P1 | 板块多维排行 | 已有基础排行，维度覆盖未完成 | 定义今日涨幅、成交额、5 日涨幅、资金净流入排序 |
| P1 | Provider 主动诊断 | 当前诊断主要来自真实请求形成的熔断记录 | 主动探测、能力级健康状态和限流 |
| P2 | 诊断历史 | 进程内状态不具备完整历史 | 持久化探测结果和状态变化 |
| P2 | 严格 PIT | 财务修订、历史成分、公司行动和 available_at 不完整 | 建立版本化历史数据和生效区间 |
| 暂缓 | 全球市场数据 | 不属于当前国内 A 股优先范围 | 国内数据验收后再评估 |

## 5. 执行顺序

1. 完成阶段一静态盘点文档和矩阵，确认所有“已实现/已关闭/缺失”的依据。
2. 进入阶段二，优先验证已有但关闭的国内 Provider，避免重复开发。
3. 对确实缺失的数据能力再新增 Provider 或服务层能力。
4. 完成阶段三市场级 API。
5. 最后集中进行固定样本对账、真实请求、部署和 CI 验收。

## 6. 当前验收状态

- [x] 核对目标分支远程 HEAD。
- [x] 清点 Provider 接口和主要实现文件。
- [x] 核对启动注册与默认 Provider 配置。
- [x] 清点 Python bridge 行情调用入口。
- [x] 形成迁移矩阵和缺口清单。
- [ ] 原 Python 上游版本固定。
- [ ] Python/Go 真实样本差分。
- [ ] 已关闭 Provider 的权限和真实请求验收。
- [ ] 市场级新增能力开发。
- [ ] 部署环境验收。

本文件只更新阶段状态，不替代真实运行验收记录。
