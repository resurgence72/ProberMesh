FROM golang:1.19-alpine as builder

ARG APPNAME="probermesh"

# 镜像设置必要的环境变量
ENV GOPROXY=https://goproxy.cn,direct \
    GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
        && apk add --no-cache upx tzdata

WORKDIR /usr/src/app

COPY . .

RUN cd ./cmd/${APPNAME}/ \
        && gofmt -s -w . \
        && go build -ldflags "-s -w" -o ${APPNAME} . \
        && upx -q -9 ${APPNAME}


# 分布构建
FROM boker-hub-registry.cn-shanghai.cr.aliyuncs.com/ops/sre-alpine:3.13 as runner

ARG APPNAME="probermesh"

# 拉取二进制
COPY --from=builder /usr/src/app/cmd/${APPNAME}/${APPNAME} /opt/app/

# 拉取配置
COPY --from=builder /usr/src/app/cmd/${APPNAME}/${APPNAME}.yaml /opt/app/


# 安装bash
RUN alpine_version=`cat /etc/issue | head -1 | awk '{print $5}'` \
        && echo "https://mirrors.aliyun.com/alpine/v${alpine_version}/main/" > /etc/apk/repositories \
        && apk update \
        && apk upgrade \
        && apk add --no-cache bash bash-doc bash-completion \
        && rm -rf /var/cache/apk/*

EXPOSE 6000 6001
ENTRYPOINT ["/opt/app/probermesh"]
