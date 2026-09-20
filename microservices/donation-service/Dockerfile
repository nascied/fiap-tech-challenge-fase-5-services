FROM golang:1.22.12-alpine3.20 AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /app/donation-service .

FROM alpine:3.20 AS runner

ARG PORT=8082
ENV PORT=${PORT}

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=build --chown=app:app /app/donation-service /app/donation-service

USER app
EXPOSE ${PORT}

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD ["sh", "-c", "wget -qO- http://127.0.0.1:${PORT}/health >/dev/null 2>&1 || exit 1"]

ENTRYPOINT ["/app/donation-service"]
