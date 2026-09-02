# 预编译打包版：context 目录需含 clawsynapsed 二进制 + docker-entrypoint.sh（sh 版）。
# 用途：legacy docker build（不支持 --build-context）下的可靠打包路径。
# 流程：先跑上一级 Dockerfile 的 builder 阶段（或 golang 容器）编译出二进制，
#       提取到本目录后用本文件打包 alpine 运行时。
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata su-exec \
  && addgroup -S clawsynapse \
  && adduser -S -G clawsynapse clawsynapse \
  && mkdir -p /var/lib/clawsynapse \
  && mkdir -p /var/lib/trustmesh-transfers \
  && chown -R clawsynapse:clawsynapse /var/lib/clawsynapse \
  && chown -R clawsynapse:clawsynapse /var/lib/trustmesh-transfers

WORKDIR /app

COPY clawsynapsed /usr/local/bin/clawsynapsed
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod 755 /usr/local/bin/docker-entrypoint.sh /usr/local/bin/clawsynapsed

VOLUME ["/var/lib/clawsynapse"]
VOLUME ["/var/lib/trustmesh-transfers"]

EXPOSE 18080

ENTRYPOINT ["docker-entrypoint.sh"]
