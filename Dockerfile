# Production image: Go API + built frontend served from one origin.
FROM node:24-alpine AS web
RUN corepack enable && corepack prepare pnpm@11.1.3 --activate
WORKDIR /web
COPY tastelingo_FE/package.json tastelingo_FE/pnpm-lock.yaml tastelingo_FE/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY tastelingo_FE/ ./
RUN pnpm build

FROM golang:1.26-alpine AS api
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /src
COPY tastelingo_BE/go.mod tastelingo_BE/go.sum ./
RUN go mod download
COPY tastelingo_BE/cmd ./cmd
COPY tastelingo_BE/internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tastelingo ./cmd/server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && addgroup -S tastelingo && adduser -S -G tastelingo tastelingo
WORKDIR /app
COPY --from=api /out/tastelingo ./server
COPY --from=web /web/dist ./web
ENV STATIC_DIR=/app/web \
    PORT=8080 \
    RATE_LIMIT=true \
    TRUST_PROXY=true
USER tastelingo
EXPOSE 8080
ENTRYPOINT ["/app/server"]
CMD ["-env", "/dev/null"]
