FROM golang:1.24-alpine AS builder
ENV GOPROXY="https://goproxy.cn,direct"
ENV GO111MODULE=on
WORKDIR /
COPY . .
RUN if [ -s survive_monitor ]; then \
        echo "Binary already exists, skipping build"; \
        chmod +x survive_monitor && \
        cp survive_monitor /survive_monitor; \
    else \
        echo "Binary not found, building from source"; \
        go mod download && \
        CGO_ENABLED=0 GOOS=linux go build -o /survive_monitor; \
    fi

FROM golang:1.24-alpine
COPY --from=builder /survive_monitor /survive_monitor
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.tuna.tsinghua.edu.cn/g' /etc/apk/repositories
RUN apk update
RUN apk --no-cache add ca-certificates
RUN apk add tzdata && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo "Asia/Shanghai" > /etc/timezone && apk del tzdata
VOLUME /log
WORKDIR /
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget --spider -q http://127.0.0.1:4343/api/ping || exit 1
CMD ["/survive_monitor"]