# Stock Data Provider WebUI

轻量级浏览器控制台，独立于 Go 与 Python 业务目录，无前端第三方运行依赖。UI 只展示 API 返回的数据和质量状态，不会自行补值、修正质量等级或生成交易结论。

## 页面

- **行情工作台**：Go market-data 服务健康检查、Provider 能力、实时行情、历史日 K 曲线/表格、质量警告。
- **分析任务**：提交 Go 异步任务，读取任务状态与持久化阶段事件。任务状态由服务端返回，前端不计算虚假的百分比。
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
