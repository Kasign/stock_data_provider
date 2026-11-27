.PHONY: fmt test vet race check build run storage-check llm-gateway-check demo

fmt:
	gofmt -w ./internal ./cmd

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

check: fmt test vet race

build:
	mkdir -p bin
	go build -trimpath -o bin/analysis-api ./cmd/analysis-api

run:
	go run ./cmd/analysis-api

storage-check:
	go run ./cmd/storage-check

# 独立 LLM 网关真实验收：只读 .env 的 SERVER_LLM_* 配置，发一次真实 Responses 调用并校验收尾身份字段。
llm-gateway-check:
	go run ./cmd/llm-gateway-check

demo:
	go run ./cmd/demo
