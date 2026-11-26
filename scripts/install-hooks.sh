#!/usr/bin/env bash
# 一键启用仓库内的 Git hooks（本地配置，不随仓库推送）。
set -euo pipefail

cd "$(dirname "$0")/.."
chmod +x .githooks/pre-commit .githooks/commit-msg .githooks/pre-push
git config core.hooksPath .githooks

echo "已启用: git config core.hooksPath .githooks"
echo "临时跳过提交检查: git commit --no-verify"
echo "临时跳过推送检查: SKIP_SLOW_HOOKS=1 git push"
echo "卸载: git config --unset core.hooksPath"
