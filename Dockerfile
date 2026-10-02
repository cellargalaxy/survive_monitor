FROM golang:1.27-alpine AS builder
ENV GOPROXY="https://goproxy.cn,direct"
ENV GO111MODULE=on
WORKDIR /
COPY . .
RUN if [ -s survive_monitor ]; then \
        echo "Binary already exists, skipping build"; \
        chmod +x survive_monitor; \
    else \
        echo "Binary not found, building from source"; \
        go mod download && \
        CGO_ENABLED=0 GOOS=linux go build -o /survive_monitor; \
    fi

FROM golang:1.27-alpine
COPY --from=builder /survive_monitor /survive_monitor
RUN chmod 755 /survive_monitor
ARG TZ="Asia/Shanghai"
ENV TZ=${TZ}
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.tuna.tsinghua.edu.cn/g' /etc/apk/repositories
RUN apk update
RUN apk --no-cache add ca-certificates
RUN apk --no-cache add tzdata && cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

ARG UID=1000
ARG GID=1000

RUN if getent group ${GID} >/dev/null 2>&1; then \
        group_name=$(getent group ${GID} | cut -d: -f1); \
    else \
        group_name="survive_monitor"; \
        addgroup -g ${GID} -S ${group_name}; \
    fi && \
    if getent passwd ${UID} >/dev/null 2>&1; then \
        user_name=$(getent passwd ${UID} | cut -d: -f1); \
    else \
        user_name="survive_monitor"; \
        adduser -u ${UID} -G "${group_name}" -S -D -H ${user_name}; \
    fi && \
    mkdir -p /log /resource /home/survive_monitor && \
    chown -R ${UID}:${GID} /log /resource /home/survive_monitor && \
    chmod 777 /log /resource /home/survive_monitor

# 微信SDK(PowerWeChat)的access_token缓存要在$HOME/.ArtisanCloud下建文件，建不出来缓存就是nil，取token时直接空指针panic，告警一条都发不出去。
# 上面的用户是-H建的，没有家目录，UID已存在时家目录也不一定可写，所以显式指定一个可写的HOME；
# 只是缓存文件，丢了重启后重新取token即可，不必挂卷
ENV HOME=/home/survive_monitor

VOLUME /log
VOLUME /resource
WORKDIR /
USER ${UID}:${GID}
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget --spider -q http://127.0.0.1:4343/api/ping || exit 1
CMD ["/survive_monitor"]
