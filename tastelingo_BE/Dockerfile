FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tastelingo ./cmd/server

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates tzdata && addgroup -S tastelingo && adduser -S -G tastelingo tastelingo
WORKDIR /app
COPY --from=build /out/tastelingo ./server
USER tastelingo
EXPOSE 8104
ENTRYPOINT ["/app/server"]
CMD ["-env", "/dev/null"]
