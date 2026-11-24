import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const [compose, envExample, dockerfile, nginxConfig, nginxTemplate, readme, deployScript] = await Promise.all([
  readFile(new URL("../../docker-compose.yml", import.meta.url), "utf8"),
  readFile(new URL("../../.env.example", import.meta.url), "utf8"),
  readFile(new URL("../Dockerfile", import.meta.url), "utf8"),
  readFile(new URL("../nginx.conf", import.meta.url), "utf8"),
  readFile(new URL("../default.conf.template", import.meta.url), "utf8"),
  readFile(new URL("../../doc/开发部署说明.md", import.meta.url), "utf8"),
  readFile(new URL("../../scripts/deploy-go.sh", import.meta.url), "utf8")
]);

test("the root Compose deployment contains only Root Server and WebUI", () => {
  for (const name of ["stock-analysis-server", "stock-analysis-web"]) {
    assert.ok(compose.includes(`  ${name}:`));
    assert.ok(compose.includes(`    container_name: ${name}`));
  }
  assert.equal(compose.includes("stock-analysis-data"), false);
  assert.equal(compose.includes("services/market-data/Dockerfile"), false);
  assert.ok(readme.includes("docker-compose.yml"));
});

test("Root Server is the only API upstream and Owner Session is mandatory", () => {
  assert.ok(compose.includes('OWNER_SESSION_ENABLED: "true"'));
  assert.ok(compose.includes("OWNER_SESSION_PUBLIC_ORIGIN: ${OWNER_SESSION_PUBLIC_ORIGIN:?"));
  assert.ok(compose.includes("OWNER_SESSION_ARGON2_MEMORY_KIB: ${OWNER_SESSION_ARGON2_MEMORY_KIB:?"));
  assert.ok(compose.includes("OWNER_SESSION_ARGON2_ITERATIONS: ${OWNER_SESSION_ARGON2_ITERATIONS:?"));
  assert.ok(compose.includes("OWNER_SESSION_ARGON2_PARALLELISM: ${OWNER_SESSION_ARGON2_PARALLELISM:?"));
  assert.ok(envExample.includes("OWNER_SESSION_ENABLED=true"));
  assert.ok(envExample.includes("OWNER_SESSION_COOKIE_PATH=/api"));
  assert.ok(envExample.includes("OWNER_SESSION_PUBLIC_ORIGIN="));
  assert.ok(envExample.includes("OWNER_SESSION_ARGON2_MEMORY_KIB="));
  assert.equal(envExample.includes("MARKET_DATA_API_KEY="), false);
  assert.equal(envExample.includes("MARKET_DATA_BASE_URL="), false);
  assert.ok(readme.includes("docker compose --env-file .env up -d --build"));
  assert.ok(readme.includes("不要覆盖"));
});

test("deployment script matches Root Server, explicit migrations and authenticated readiness", () => {
  assert.ok(deployScript.includes("build stock-analysis-server stock-analysis-web"));
  assert.equal(deployScript.includes("build stock-analysis-data"), false);
  assert.ok(deployScript.includes("for migration_key in DB_AUTO_MIGRATE MIGRATION_ENABLED"));
  assert.ok(deployScript.includes("curl_ready http://127.0.0.1:8092/readyz"));
  assert.ok(deployScript.includes("PLATFORM_ADMIN_TOKEN"));
  assert.ok(envExample.includes("DB_AUTO_MIGRATE=false"));
  assert.ok(envExample.includes("MIGRATION_ENABLED=false"));
  assert.ok(readme.includes("owner-init"));
  assert.ok(readme.includes("DB_AUTO_MIGRATE=true"));
  assert.ok(readme.includes("MIGRATION_ENABLED=true"));
  assert.ok(readme.includes("不要覆盖"));
});

test("Nginx proxies the same-origin API path to Root Server without cookie rewriting", () => {
  assert.ok(dockerfile.includes("COPY default.conf.template /etc/nginx/templates/default.conf.template"));
  assert.ok(nginxConfig.includes("include /etc/nginx/conf.d/*.conf;"));
  assert.ok(nginxTemplate.includes("location ^~ /api/"));
  assert.ok(nginxTemplate.includes("proxy_pass http://stock-analysis-server:8092;"));
  assert.ok(nginxTemplate.includes("proxy_pass http://stock-analysis-server:8092/healthz;"));
  assert.ok(nginxTemplate.includes("proxy_pass http://stock-analysis-server:8092/readyz;"));
  assert.equal(nginxTemplate.includes("/market-api"), false);
  assert.equal(nginxTemplate.includes("/analysis-api"), false);
  assert.equal(nginxTemplate.includes("proxy_cookie_path"), false);
  assert.ok(nginxTemplate.includes("proxy_set_header Host $host;"));
  assert.ok(nginxTemplate.includes("proxy_set_header X-Forwarded-Proto $scheme;"));
});
