# Tek servis = bot (dashboard API bot process içinde PORT'ta açılıyor)
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/bot

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /bot .
EXPOSE 8080
CMD ["./bot"]
