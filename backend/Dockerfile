FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/trustmesh-server ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates su-exec tzdata \
  && addgroup -S trustmesh \
  && adduser -S -G trustmesh trustmesh

WORKDIR /app

COPY --from=builder /out/trustmesh-server /usr/local/bin/trustmesh-server
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["trustmesh-server"]
