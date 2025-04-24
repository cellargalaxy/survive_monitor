FROM golang:1.24-alpine AS builder
ENV GOPROXY="https://goproxy.cn,direct"
ENV GO111MODULE=on
WORKDIR /
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /survive_monitor

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