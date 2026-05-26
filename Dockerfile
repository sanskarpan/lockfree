FROM golang:1.26.3-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/lockfree-visualizer ./web

FROM alpine:3.22

RUN addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=build /out/lockfree-visualizer /app/lockfree-visualizer
COPY web/static /app/web/static

RUN mkdir -p /app/data /app/config && chown -R app:app /app

USER app

ENV HOST=0.0.0.0 \
    PORT=8081 \
    LOCKFREE_DATA_DIR=/app/data \
    LOCKFREE_USERS_FILE=/app/config/users.json

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8081/readyz >/dev/null || exit 1

ENTRYPOINT ["/app/lockfree-visualizer"]
