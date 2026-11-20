# ---- 构建阶段 ----
FROM golang:1.25-alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o cmdb-server ./cmd/server

# ---- 运行阶段 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
WORKDIR /app
COPY --from=builder /app/cmdb-server .
COPY --from=builder /app/config.yaml .
EXPOSE 8080
ENTRYPOINT ["./cmdb-server", "-config", "config.yaml"]
