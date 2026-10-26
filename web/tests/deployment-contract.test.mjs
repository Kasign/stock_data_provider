import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [compose, envExample, dockerfile, nginxConfig, nginxTemplate, readme] = await Promise.all([
  readFile(new URL("../../docker-compose.yml", import.meta.url), "utf8"),
  readFile(new URL("../../.env.example", import.meta.url), "utf8"),
  readFile(new URL("../Dockerfile", import.meta.url), "utf8"),
  readFile(new URL("../nginx.conf", import.meta.url), "utf8"),
  readFile(new URL("../default.conf.template", import.meta.url), "utf8"),
  readFile(new URL("../README.md", import.meta.url), "utf8")
]);

test("the root Compose file is the single deployment entry point", () => {
  for (const name of ["stock-analysis-data", "stock-analysis-server", "stock-analysis-web"]) {
    assert.ok(compose.includes(`  ${name}:`));
    assert.ok(compose.includes(`    container_name: ${name}`));
  }
  assert.ok(readme.includes("docker-compose.yml"));
});

test("WebUI upstream defaults match the canonical service names", () => {
  assert.ok(compose.includes("MARKET_DATA_SERVICE_NAME: ${MARKET_DATA_SERVICE_NAME:-stock-analysis-data}"));
  assert.ok(compose.includes("ANALYSIS_SERVICE_NAME: ${ANALYSIS_SERVICE_NAME:-stock-analysis-server}"));
  assert.ok(envExample.includes("MARKET_DATA_SERVICE_NAME=stock-analysis-data"));
  assert.ok(envExample.includes("ANALYSIS_SERVICE_NAME=stock-analysis-server"));
  assert.ok(readme.includes("docker compose --env-file .env up -d --build"));
  assert.ok(readme.includes("不要覆盖"));
});

test("Nginx envsubst only rewrites configured upstream names", () => {
  assert.ok(dockerfile.includes("COPY default.conf.template /etc/nginx/templates/default.conf.template"));
  assert.ok(dockerfile.includes("NGINX_ENVSUBST_FILTER='^(MARKET_DATA_SERVICE_NAME|ANALYSIS_SERVICE_NAME)$'"));
  assert.ok(nginxConfig.includes("include /etc/nginx/conf.d/*.conf;"));
  assert.ok(nginxTemplate.includes("proxy_pass http://${MARKET_DATA_SERVICE_NAME}:8091/;"));
  assert.ok(nginxTemplate.includes("proxy_pass http://${ANALYSIS_SERVICE_NAME}:8092/;"));
  assert.ok(nginxTemplate.includes("proxy_set_header Host $host;"));
  assert.ok(nginxTemplate.includes("proxy_set_header X-Forwarded-Proto $scheme;"));
});
