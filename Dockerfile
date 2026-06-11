# --- Stage 1: Builder ---
FROM golang:1.26-alpine3.23@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS builder
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

ENV LIPGLOSS_RENDERER_HAS_DARK_BACKGROUND=true
ENV UPTOP_DB_TYPE=sqlite
ENV UPTOP_DB_DSN=/data/uptop.db
ENV UPTOP_KEYS=/data/authorized_keys
ENV UPTOP_SSH_HOST_KEY=/data/.ssh/id_ed25519
ENV UPTOP_PORT=23234

EXPOSE 23234
USER uptop
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./uptop"]