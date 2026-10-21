FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN test -z "$(gofmt -l ./cmd ./internal)" && go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/market-data ./cmd/market-data

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
