# phase 1, 构建阶段
# 使用 builder 别名
FROM golang:1.24.11-alpine3.23 as builder

WORKDIR /app/code

# 缓存依赖：只复制 go.mod 和 go.sum
COPY ./go.mod /app/code/go.mod
COPY ./go.sum /app/code/go.sum
# 设置代理并下载依赖。这层会在 go.mod/go.sum 不变时被缓存
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download

# 复制所有代码，如果代码改变，将跳过上一层缓存
COPY . /app/code
# 编译：禁用 CGO 以生成静态二进制文件，并使用 -ldflags 减小体积
RUN go mod tidy && \
    CGO_ENABLED=0 go build -ldflags "-s -w" -o main ./main.go


# phase 2, 运行阶段
FROM scratch

# 设置工作目录
WORKDIR /app

COPY --from=builder /app/code/main /app/main

# env --------- start
ENV KAFKA_BROKERS="kafka-dev:9092"
ENV MYSQL_ADDR=""
ENV MYSQL_DB=""
ENV MYSQL_PWD=""
ENV MYSQL_USER=""
ENV REDIS_ADDR=""
ENV SERVER_BIND_ADDR="0.0.0.0:8180"
ENV REDIS_ADDR=""
ENV REDIS_PWD=""
# env --------- end

EXPOSE 8180

# 设置容器启动时运行的命令
CMD ["./main"]