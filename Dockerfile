FROM golang:1.22-alpine AS builder
RUN apk add --no-cache ca-certificates git
WORKDIR /src
COPY go.mod go.sum ./
# 国内网络可能无法稳定访问 proxy.golang.org；失败（包含超时）时自动尝试备用模块代理。
ARG GOPROXY=https://goproxy.cn|https://goproxy.io|direct
ENV GOPROXY=${GOPROXY}
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/analysis-api ./cmd/analysis-api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/analysis-api /app/analysis-api
USER app
EXPOSE 8092
ENTRYPOINT ["/app/analysis-api"]
