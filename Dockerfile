FROM golang:1.24 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /novel-assistant ./cmd

FROM debian:bookworm-slim

WORKDIR /app
COPY --from=builder /novel-assistant /usr/local/bin/novel-assistant
COPY web ./web
COPY data ./data
COPY .env.example ./.env.example

EXPOSE 8080

CMD ["novel-assistant"]
