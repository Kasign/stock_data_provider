# Stock Data Provider WebUI

轻量级浏览器控制台，独立于 Go 与 Python 业务目录，无前端第三方运行依赖。UI 只展示 API 返回的数据和质量状态，不会自行补值、修正质量等级或生成交易结论。

## 页面

- **行情工作台**：Go market-data 服务健康检查、Provider 能力、实时行情、历史日 K 曲线/表格、质量警告。
- **分析任务**：提交 Go 异步任务，读取任务状态与持久化阶段事件。任务状态由服务端返回，前端不计算虚假的百分比。
- **最近分析与报告导出**：当前浏览器内保留最近 10 条 task/run 引用，点击即可从现有接口恢复状态和结果；结果支持下载 Markdown 与 JSON。该列表不是服务端历史，不跨浏览器同步，也不缓存完整报告。
- **连接设置**：配置同源 API 路径、行情服务 API Key。API Key 仅保存在当前浏览器的 localStorage，不写入 HTML、镜像或仓库。

界面参考 Kasign/daily_stock_analysis 的仪表盘组织方式，但保持依赖轻、目录独立与 Provider 数据契约对齐。

## 部署（Docker / 1Panel）

先确认 Go 两个服务已通过现有 docker-compose.go.yml 部署到外部 Docker 网络 1panel-network，并能分别通过网络别名 market-data:8091 和 stock-analysis-go:8092 访问。

在仓库根目录执行：

~~~bash
docker compose -f web/docker-compose.yml up -d --build
docker compose -f web/docker-compose.yml ps
docker compose -f web/docker-compose.yml logs --tail=100 stock_analysis_web
curl -fsS http://127.0.0.1:8093/
~~~

Compose 服务名和容器名统一为 **stock_analysis_web**。如果修改端口或监听地址，可在 shell 环境设置 WEBUI_PORT 和 WEBUI_BIND_ADDR。默认只绑定 127.0.0.1:8093；同在 1panel-network 网络的 OpenResty 可以通过 http://stock_analysis_web/ 访问前端。无需把两个后端 API 端口直接映射到公网。

首次打开页面，点击右上角设置，填写实际部署的 MARKET_DATA_API_KEY。默认 API 路径：

- /market-api：Nginx 转发至 market-data:8091，浏览器请求附带 X-API-Key。
- /analysis-api：Nginx 转发至 stock-analysis-go:8092。

若 Go 服务部署在其他 Docker 网络，需将本容器加入对应网络并将 nginx.conf 中的上游名称改为实际服务名；不要把 API Key 硬编码进 Nginx 配置或前端源文件。

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
docker build -t stock_analysis_web:local .
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

把生成的同一个值写入部署仓库根目录的 `.env` 中的 `MARKET_DATA_API_KEY`，供 `market-data` 与 `stock-analysis-go` 两个 Go 服务共用，并按部署流程重启/重建 Go 服务。随后把完全相同的值填入 WebUI「连接与密钥设置」的行情 API Key 输入框。WebUI 只会把该值保存到当前浏览器的 `localStorage`，并在请求受保护的行情端点时以 `X-API-Key` 发送；它不会创建密钥，也不会修改服务器的 `.env`。不要把真实密钥提交到 Git。
