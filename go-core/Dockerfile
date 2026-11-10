FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN test -z "$(gofmt -l ./cmd ./internal)" && go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/analysisd ./cmd/analysisd

FROM alpine:3.21
RUN addgroup -S dsa && adduser -S -G dsa dsa && apk add --no-cache ca-certificates tzdata
COPY --from=build /out/analysisd /usr/local/bin/analysisd
USER dsa
ENV HTTP_ADDR=:8088 LOG_LEVEL=info
EXPOSE 8088
ENTRYPOINT ["/usr/local/bin/analysisd"]
