# 后端镜像；构建上下文为仓库根目录（需要根目录 sql/ 的初始化脚本）：
#   docker build -f scaffold-go/Dockerfile -t scaffold-backend .
# 国内构建可加 --build-arg GOPROXY=https://goproxy.cn,direct
FROM golang:1.26-alpine AS build
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY CGO_ENABLED=0
WORKDIR /src
COPY scaffold-go/go.mod scaffold-go/go.sum ./
RUN go mod download
COPY scaffold-go/ ./
RUN go build -trimpath -ldflags="-s -w" -o /out/scaffold-server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/scaffold-server ./scaffold-server
COPY sql/ ./sql/
ENV SQL_DIR=/app/sql TZ=Asia/Shanghai SERVER_PORT=8080
USER app
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=30s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1
ENTRYPOINT ["./scaffold-server"]
