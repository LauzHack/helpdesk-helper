# Build
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /lauzhack-bot .

# Runtime
FROM alpine:3.22

RUN adduser -D -u 1000 appuser

WORKDIR /app

COPY --from=builder /lauzhack-bot /usr/local/bin/lauzhack-bot
COPY server/ui ./server/ui

VOLUME ["/data"]

EXPOSE 8080

CMD ["lauzhack-bot", "/data/config.json"]
