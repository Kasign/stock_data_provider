# Go 数据质量契约

- 适用范围：所有 Go 原生行情 Provider、归一化、API、缓存、存储和分析调用链。
- 生效分支：`feat/full-go-data-providers-api-switch`
- 更新日期：2026-11-02

## 1. 必须保留的质量约束

**Go 原生数据服务已经实现的单位统一、数据质量校验和质量元数据能力必须保留。不得为了清理旧 Python 实现、简化代码或兼容旧返回值而删除、弱化或绕过。**

1. **单位规范化**：按 Provider/端点真实契约，将成交量统一为 shares、成交额统一为 CNY；保留必要的来源专属转换。单位无法确认时标记 unknown/degraded，禁止猜测换算。
2. **缺失值语义**：区分 null、真实数值 0 和未知值。不得将缺失字段填为 0，不得伪造 CNY 或 shares。
3. **复权语义**：明确区分 none、forward_adjusted、backward_adjusted、unknown，保留 adjustment_method。未知基准或算法不明确的数据不得静默混入规范序列。
4. **质量门禁**：非有限数值、非法 OHLC、重复/越界日期、数量级异常、分页/覆盖不完整等检查必须保留。
5. **质量元数据**：quality_status、quality_reasons、source、source_version、fallback、provider_time、fetched_at、normalized_at 等信息不得在 Provider、API、缓存、存储或分析层丢失。
6. **回归测试**：单位、金额/成交量校验、复权、null/zero、Provider fallback 和质量状态传播的修改必须有 Go 测试保护，覆盖成功、缺失、未知、异常尺度和错误复权场景。
7. **迁移边界**：下线 Python 代码不能成为删除 Go 质量能力的理由。旧系统曾经补零、忽略质量或使用未知单位的行为，不得成为 Go 回归目标。

## 2. CI 与测试原则

- 数据质量契约使用 Go 单元测试和集成测试验证。
- 不再运行 Python/Go 样本对照脚本，不再安装 Python 运行时作为 Go 数据质量 CI 的前置条件。
- 纯 Go 测试必须覆盖标准单位、缺失值、复权、OHLC、日期、重复记录、非有限数值和数量级校验。
- 真实 Provider 联网探测与本地数据质量测试分离；无凭据 Provider 应标记 skipped，不得误报为失败或可用。

## 3. Pytdx 命名说明

当前 `services/market-data/internal/provider/pytdx.go` 是 Go 原生实现：

- 使用 `github.com/injoyai/tdx` 和 `github.com/injoyai/ios`。
- 直接通过 Go TCP 客户端实现通达信协议。
- 不启动 Python 进程，不调用 Python bridge，不依赖 Python 解释器或 pytdx Python 包。

因此保留该 Go Provider；不得仅因名称含有 `py` 或 `Pytdx` 将其误判为 Python 运行时依赖。后续代码注释、文档统一称为“Go 通达信协议 Provider（历史名称 Pytdx）”。

## 4. Python 下线完成标准

Python 下线以运行时和代码依赖为准，而不是只删除 `.py` 文件：

- 仓库不再包含 Python bridge 运行服务、Python 适配器、Python smoke 工具和 Python 依赖清单。
- Go 服务启动、readiness、业务流程、部署配置不再依赖 Python URL、Token 或 Python 容器。
- Docker Compose、Dockerfile、CI、环境变量示例、OpenAPI 和文档不再要求 Python。
- 原 Python bridge 提供的 LLM、搜索、通知、持久化及辅助数据能力，必须迁移到 Go 或明确移除对应业务能力后，才能删除最后的桥接调用。
- 所有核心 Go 模块测试、race、vet、镜像构建和目标环境验收通过。


## 5. Go 通达信协议 Provider 诊断边界

Go 通达信协议客户端返回强类型 Kline/Quote，不是 Python pytdx 包。Adapter 可对解码后的字段、交易日期、OHLC、成交量和成交额执行结构化诊断；协议库在二进制解码阶段失败时，目前只返回协议级 error，无法可靠恢复具体字段字节位置。
