# Stock Data Provider WebUI

## 当前个股研究与设置中心
- 研究列表的“查看详情”在同一研究工作区切换标的；Quote 与 MA5/MA20 都关联当前股票，指标状态区分未加载、读取中和历史日 K 数据不足。
- 公司资料、行业/概念、季度财务、估值、资金流、筹码与公告由专业详情按需读取；分区导航可定位并同步当前分区。完整图表与历史日 K 继续复用个股行情视图。
- 设置中心支持浏览器本地工作区 JSON 导入/导出（主题、同源 API 路径、自选股、最近任务引用），不会导出 API Key；导入保留当前 API Key。服务器配置、密码与 .env 备份仍未接入。
- 本次改动未经过测试、构建、CI 或真实接口验收。

轻量级浏览器控制台，独立于 Go 与 Python 业务目录，无前端第三方运行依赖。UI 只展示 API 返回的数据和质量状态，不会自行补值、修正质量等级或生成交易结论。

## 页面与当前规划

### 当前已有的页面能力

- **首页/市场总览**：核心指数、市场广度、市场情绪即时指标、市场成交额、行业/概念排行、涨跌停池、人气/飙升榜、龙虎榜与交易日历；市场情绪历史、全市场资金净流入、多维板块排序和全球市场已先建立占位，等待数据层接口。
- **个股查询与研究**：股票搜索、实时行情、日 K、技术指标、公司资料、财务、估值、公告、筹码和资金流等实际接入的数据域。
- **自选股**：浏览器本地清单、快捷查询/分析和跟踪摘要；行情快照只在已加载并匹配当前标的时显示，最近任务/Run 结果只显示浏览器本地引用能确认的信息，不支持跨设备同步。
- **分析任务**：创建异步任务、查询单个 Task、读取阶段事件和 RunResult；最近任务引用保存在当前浏览器，不等于完整服务端任务历史。
- **数据中心**：支持“立即检查”WebUI 当前页面路径、行情/分析服务存活与就绪，并读取分析服务返回的 MariaDB/Redis 依赖状态和 Provider 能力声明；Provider 上游实际探测尚待后端诊断 API。
- **设置中心**：管理显示主题、连接与服务检查、浏览器本地工作区导入/导出；行情 API Key 仅保存在当前浏览器，不写入服务器 `.env`。本地导出不含 Key、行情缓存或完整分析报告；服务器设置读写仍未接入。

### 当前有效的排版与数据能力计划

- [产品信息架构与逐页排版计划](./UI_LAYOUT_PLAN.md)
- [数据能力缺口与服务巡检设计](./DATA_CAPABILITY_GAPS.md)
- [实施任务清单](./PRODUCT_BACKLOG.md)
- [当前 API 契约](./API_CONTRACT.md)

当前已完成首页市场模块排版、个股研究列表与专业资料整合、研究详情分区导航、自选股/任务状态摘要、分析任务工作区、数据中心基础巡检及十组设置页面。当前轮新增 MA 未加载/读取中/数据不足状态区分，以及仅限当前浏览器的工作区导入/导出。剩余工作主要是统一的真实浏览器与 NAS/1Panel 验收，以及市场情绪历史、全市场/板块资金流、多维板块历史、全球市场、批量股票概览、逐 Provider 实际诊断和服务端任务历史等后端能力。UI 不使用示例行情冒充真实数据。

## 部署（Docker / 1Panel）

仓库使用根目录唯一的 `docker-compose.yml` 部署整套服务，包含 `stock-analysis-data`（行情 API）、`stock-analysis-server`（分析 API）和 `stock-analysis-web`（WebUI）。三个服务都加入外部网络 `1panel-network`，共享现有 MariaDB/Redis；不再分别执行 WebUI 与 Go 服务的 Compose 文件。

根目录 `.env.example` 是唯一模板。首次创建 `.env` 时复制模板；若 `.env` 已存在，**不要覆盖整份文件**，只核对并修改下列配置，保留数据库/Redis 密码和 API Key：

```dotenv
MARKET_DATA_BASE_URL=http://stock-analysis-data:8091
MARKET_DATA_SERVICE_NAME=stock-analysis-data
ANALYSIS_SERVICE_NAME=stock-analysis-server
WEBUI_BIND_ADDR=127.0.0.1
WEBUI_PORT=8093
```

从旧版拆分 Compose 升级时，旧容器可能仍占用端口或保留旧 DNS 别名。按根目录 README 的一次性升级步骤停止并移除旧 Go 应用容器；此操作不删除外部 MariaDB/Redis 数据。

在仓库根目录执行：

```bash
docker compose --env-file .env up -d --build
docker compose --env-file .env ps
docker compose --env-file .env logs --tail=100 stock-analysis-web
docker ps --filter name=stock-analysis-web
curl -fsS http://127.0.0.1:8093/
```

运行中的容器名和 Compose 服务名均为 **`stock-analysis-web`**。默认宿主机端口只绑定 `127.0.0.1:8093`；可在根目录 `.env` 中调整 `WEBUI_BIND_ADDR` 和 `WEBUI_PORT`。分析 API 只绑定 `127.0.0.1:8092`，行情 API 只在 Docker 网络内部提供 `8091`，无需把后端端口映射到公网。

两个 Nginx 上游服务名由根目录 Compose 注入，并在容器启动时生成站点配置：

- `MARKET_DATA_SERVICE_NAME=stock-analysis-data`：将 `/market-api/` 转发到 `服务名:8091`，浏览器请求附带 `X-API-Key`。
- `ANALYSIS_SERVICE_NAME=stock-analysis-server`：将 `/analysis-api/` 转发到 `服务名:8092`。

变量只填写 `1panel-network` 中可解析的 Docker 服务名/网络别名，不填协议、路径或端口。修改后重建 `stock-analysis-web` 容器，使 Nginx 重新生成代理配置。通过同一外部网络访问时，OpenResty 可使用 `http://stock-analysis-web/` 代理页面。不要把 API Key 硬编码进 Nginx 配置或前端源文件。


## 本地调试

任何静态 HTTP 服务器都可以运行此页面。例如从仓库根目录：

~~~bash
python3 -m http.server 8080 --directory web
~~~

该方式只提供静态文件，不会建立 API 代理。若要同时联调后端，请使用 Docker Compose，或在开发环境自行为 /market-api 与 /analysis-api 配置同源代理。

## 安全和边界

- 默认不公开监听宿主机公网网卡，也不内置 API Key。
- 浏览器 localStorage 适用于个人内网工具，不是密钥保险箱；同一浏览器的脚本可读取它。不要把含密钥的会话交给他人使用。
- 最近分析列表只保存 task/run ID、标的、类型、状态和更新时间；下载的 Markdown/JSON 文件由浏览器生成，用户应自行妥善保管。
- Go 分析服务当前没有内置用户认证；公网发布前必须在 OpenResty/身份认证层限制访问，并启用 HTTPS。仅有 HTTPS 并不能代替访问控制。
- 反向代理与后端 API 的真实可达性取决于部署时的 Docker 网络和 Go 服务状态；CI 只验证前端契约、静态资源和镜像构建，不宣称已连接生产实例。
- 行情字段、复权口径、来源和 quality status 均原样呈现后端 API 的规范化结果。质量降级或数据不足时，不能据此推定数据适合交易。

## 验证

~~~bash
cd web
npm test
npm run check
docker build -t stock-analysis-web:local .
~~~


## API 与数据保存位置

WebUI 本身是静态 HTML/CSS/JavaScript，由 Nginx 提供页面和同源反向代理；它不连接数据库，也不会通过网页代码直接写 MariaDB/Redis。

- 浏览器本地：行情 API Key、最多 50 个自选股、最近 10 条任务引用存于该浏览器的 `localStorage`。最近任务只保存 task/run ID、标的、类型、状态和更新时间，不保存完整报告。换浏览器或清除站点数据后这些本地项不会自动同步。
- Go 行情服务：通过 `/market-api` 提供健康状态、Provider 列表、实时行情、日 K、筹码分布和资金流等端点。行情服务使用自己的 MariaDB/Redis 配置；具体请求是否会写入持久化表，由后端端点逻辑决定。
- Go 分析服务：通过 `/analysis-api` 创建任务、读取状态与阶段事件、读取结构化 RunResult。分析任务、阶段事件、运行结果和快照由后端持久化到 MariaDB；Redis 用于结果/快照缓存，MariaDB 是最终事实源。

所以，API 是 WebUI 与 Go 服务之间的 HTTP 接口，不是“存在 web 页面里”的数据存储。WebUI 只保留少量浏览器设置/引用；分析结果是否进入数据库，由 Go 后端负责。


### 行情 API Key 的生成与配置

`MARKET_DATA_API_KEY` 不是数据源厂商提供的账号 Key，也不是由 WebUI 自动创建的；它是部署管理员为保护行情服务而生成的内部共享密钥。可在服务器终端运行：

```bash
openssl rand -hex 32
```

把生成的同一个值写入部署仓库根目录的 `.env` 中的 `MARKET_DATA_API_KEY`，供 `stock-analysis-data` 与 `stock-analysis-server` 两个 Go 服务共用，并按部署流程重启/重建 Go 服务。随后把完全相同的值填入 WebUI「连接与密钥设置」的行情 API Key 输入框。WebUI 只会把该值保存到当前浏览器的 `localStorage`，并在请求受保护的行情端点时以 `X-API-Key` 发送；它不会创建密钥，也不会修改服务器的 `.env`。输入框默认掩码显示，点击“显示”可临时核对密钥，关闭设置窗口后会自动重新隐藏。不要把真实密钥提交到 Git。


## 当前尚不能仅靠 WebUI 实现的管理功能

- **密码登录/修改/关闭认证**：当前 Go 服务没有这套管理员认证 API。仅在浏览器端用 JavaScript 比对密码或把密码存在 localStorage 不能保护行情/分析 API，因此 WebUI 不提供伪安全的“设置密码”开关。需要单独设计服务端会话、密码哈希、登录限流及接口鉴权。
- **真实服务端 `.env` 导出/恢复**：浏览器无法直接读取或写入 NAS 上的 `.env`。需要服务端配置管理 API、管理员认证、变量白名单/校验、密钥脱敏策略、原子备份及导入后的安全重载流程；不得把浏览器中的 API Key 和路径配置称作服务端 `.env` 备份。
- 目前可以备份的是后续单独实现的 WebUI 浏览器设置文件，不等价于部署环境变量文件。上述能力需要后端契约与安全审查后才能启用。


## 产品功能规划与任务接力

- [WebUI 产品信息架构与逐页排版计划](./UI_LAYOUT_PLAN.md)：页面职责、布局顺序与状态表现。
- [数据能力缺口与服务巡检设计](./DATA_CAPABILITY_GAPS.md)：已有接口、数据层缺口、立即检查与后端诊断需求。
- [WebUI 产品功能扩展与实施任务清单](./PRODUCT_BACKLOG.md)：当前执行优先级、依赖边界与验收标准。
- [WebUI 任务交接上下文](./HANDOFF.md)：分支/CI 真值、GitHub Issue 列表、下一步命令和跨聊天接力说明。


## 2026-10-30 导航调整

- 左侧导航改为平铺页面列表，不再按 session/工作流分组。
- 服务状态统一在“数据中心”查看，侧栏不再重复展示行情服务与分析服务状态。
- 顶部设置入口进入完整“设置中心”页面；连接与密钥通过设置页内的连接配置入口编辑。


- 连接配置已整合到设置中心“连接与服务”分组，不再弹出独立连接配置窗口。
- 在设置页可直接编辑 API 路径与 API Key、检查已保存/未保存配置、保存或清除浏览器本地连接设置。


## 2026-10-30 设置中心配置项完善

- 设置中心按本地偏好、浏览器连接配置、服务端只读能力和服务端待接入配置明确归属。
- 补齐基础设置、Provider 路由、AI 模型/问股、通知渠道、任务调度、条件选股、安全认证、服务密钥和服务器备份的字段清单与配置说明。
- 只有主题、连接配置和浏览器工作区导入/导出属于当前已实现操作；服务端配置项保持禁用，不在浏览器伪造保存、测试或服务端状态。
- 管理员登录态与行情服务 API Key 分开说明；浏览器工作区备份不等于服务器配置/数据库备份。
