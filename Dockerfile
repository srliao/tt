# syntax=docker/dockerfile:1

# ── frontend: build the React SPA ────────────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /app/web
RUN npm install -g pnpm@9
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

# ── backend: embed SPA and compile the static binary ─────────────
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Stage the freshly built SPA into the //go:embed directory.
COPY --from=frontend /app/web/dist/ ./internal/web/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o /out/tt ./cmd/tt

# ── litestream binary source ─────────────────────────────────────
FROM litestream/litestream:0.3.13 AS litestream

# ── runtime ──────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY --from=backend    /out/tt                    /usr/local/bin/tt
COPY docker/litestream.yml /etc/litestream.yml
COPY docker/entrypoint.sh  /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
