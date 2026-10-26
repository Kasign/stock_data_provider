FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
# 国内网络可能无法稳定访问 proxy.golang.org；用镜像链，并在请求出错（含超时）时尝试下一个源。
ARG GOPROXY=https://goproxy.cn|https://goproxy.io|direct
ENV GOPROXY=${GOPROXY}
RUN go mod download
COPY . .
# 在镜像构建中统一 Go 源码格式，避免开发机编辑器差异导致部署构建被格式检查阻断。
RUN gofmt -w ./cmd ./internal && go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/market-data ./cmd/market-data

FROM alpine:3.21
RUN addgroup -S marketdata && adduser -S -G marketdata marketdata && apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/market-data /usr/local/bin/market-data
COPY --from=build /src/configs /app/configs
USER marketdata
ENV MARKET_DATA_HTTP_ADDR=:8091 LOG_LEVEL=info TZ=Asia/Shanghai
EXPOSE 8091
HEALTHCHECK --interval=20s --timeout=5s --retries=3 --start-period=15s CMD wget -q -O - http://127.0.0.1:8091/health/live >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/market-data"]
