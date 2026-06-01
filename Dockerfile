# --- Stage 1: Builder ---
FROM golang:1.26-alpine3.23 AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
ENV CGO_ENABLED=1
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" -o uptop ./cmd/uptop/main.go

# --- Stage 2: Runner ---
FROM alpine:3.23
WORKDIR /app
RUN apk add --no-cache ca-certificates && apk upgrade --no-cache
RUN addgroup -g 1000 -S uptop && adduser -u 1000 -S uptop -G uptop
RUN mkdir /data && chown uptop:uptop /data

COPY --from=builder /app/uptop .
COPY docker-entrypoint.sh /usr/local/bin/

ENV LIPGLOSS_RENDERER_HAS_DARK_BACKGROUND=true
ENV UPTOP_DB_TYPE=sqlite
ENV UPTOP_DB_DSN=/data/uptop.db
ENV UPTOP_KEYS=/data/authorized_keys
ENV UPTOP_PORT=23234

EXPOSE 23234
USER uptop
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./uptop"]