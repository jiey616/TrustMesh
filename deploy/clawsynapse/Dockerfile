FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# 使用本地源码构建（context 指向 D:/AiWorkspace/clawsynapse），不使用远程 clone
COPY . .
RUN go env -w GOPROXY=https://goproxy.cn,direct && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/clawsynapsed ./cmd/clawsynapsed

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata su-exec \
  && addgroup -S clawsynapse \
  && adduser -S -G clawsynapse clawsynapse \
  && mkdir -p /var/lib/clawsynapse \
  && mkdir -p /var/lib/trustmesh-transfers \
  && chown -R clawsynapse:clawsynapse /var/lib/clawsynapse \
  && chown -R clawsynapse:clawsynapse /var/lib/trustmesh-transfers

WORKDIR /app

COPY --from=builder /out/clawsynapsed /usr/local/bin/clawsynapsed
# entrypoint 必须用本目录（deploy/clawsynapse）的 sh 版脚本：
# runtime 是 alpine（无 bash/python3/hermes），clawsynapse 仓库根的 bash 版不适用。
# 构建时传 --build-context entry=<本目录>。
COPY --from=entry docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod 755 /usr/local/bin/docker-entrypoint.sh

VOLUME ["/var/lib/clawsynapse"]
VOLUME ["/var/lib/trustmesh-transfers"]

EXPOSE 18080

ENTRYPOINT ["docker-entrypoint.sh"]
