.PHONY: fmt test vet race check build run storage-check demo

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

demo:
	go run ./cmd/demo
