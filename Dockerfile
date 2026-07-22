# --- Stage 1: Builder ---
FROM golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
ENV CGO_ENABLED=0
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" -o uptop ./cmd/uptop

# --- Stage 2: Runner ---
FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
WORKDIR /app
RUN apk add --no-cache ca-certificates && apk upgrade --no-cache
RUN addgroup -g 1000 -S uptop && adduser -u 1000 -S uptop -G uptop
RUN mkdir -p /data/.ssh && chown -R uptop:uptop /data

COPY --from=builder /app/uptop .
COPY --chmod=755 docker-entrypoint.sh /usr/local/bin/

ENV UPTOP_DB_TYPE=sqlite
ENV UPTOP_DB_DSN=/data/uptop.db
ENV UPTOP_KEYS=/data/authorized_keys
ENV UPTOP_SSH_HOST_KEY=/data/.ssh/id_ed25519
ENV UPTOP_PORT=23234

EXPOSE 8080 23234
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/api/health || exit 1
USER uptop
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./uptop"]