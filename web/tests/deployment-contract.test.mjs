import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [compose, dockerfile, nginxConfig, nginxTemplate, readme] = await Promise.all([
  readFile(new URL("../docker-compose.yml", import.meta.url), "utf8"),
  readFile(new URL("../Dockerfile", import.meta.url), "utf8"),
  readFile(new URL("../nginx.conf", import.meta.url), "utf8"),
  readFile(new URL("../default.conf.template", import.meta.url), "utf8"),
  readFile(new URL("../README.md", import.meta.url), "utf8")
]);

test("WebUI has the agreed stable container name", () => {
  assert.match(compose, /^  stock-analysis-web:\s*$/m);
  assert.match(compose, /^    container_name: stock-analysis-web$/m);
  assert.ok(readme.includes("容器名和 Compose 服务名均为 **`stock-analysis-web`**"));
});

test("upstream Docker service names are injected from the root env file", () => {
  assert.match(compose, /MARKET_DATA_SERVICE_NAME: \$\{MARKET_DATA_SERVICE_NAME:-market-data\}/);
  assert.match(compose, /ANALYSIS_SERVICE_NAME: \$\{ANALYSIS_SERVICE_NAME:-stock-analysis-go\}/);
  assert.match(readme, /根目录已存在 Go 服务使用的 `\.env\.example`/);
  assert.match(readme, /MARKET_DATA_SERVICE_NAME/);
  assert.match(readme, /ANALYSIS_SERVICE_NAME/);
  assert.match(readme, /--env-file \.env -f web\/docker-compose\.yml/);
  assert.match(readme, /不要覆盖它/);
});

test("Nginx envsubst only rewrites configured upstream names", () => {
  assert.match(dockerfile, /COPY default\.conf\.template \/etc\/nginx\/templates\/default\.conf\.template/);
  assert.ok(dockerfile.includes("NGINX_ENVSUBST_FILTER='^(MARKET_DATA_SERVICE_NAME|ANALYSIS_SERVICE_NAME)$'"));
  assert.match(nginxConfig, /include \/etc\/nginx\/conf\.d\/\*\.conf;/);
  assert.match(nginxTemplate, /proxy_pass http:\/\/\$\{MARKET_DATA_SERVICE_NAME\}:8091\//);
  assert.match(nginxTemplate, /proxy_pass http:\/\/\$\{ANALYSIS_SERVICE_NAME\}:8092\//);
  assert.match(nginxTemplate, /proxy_set_header Host \$host;/);
  assert.match(nginxTemplate, /proxy_set_header X-Forwarded-Proto \$scheme;/);
});
