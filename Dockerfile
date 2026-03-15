# Binance Bot API - Render için
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /api-server ./api

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /api-server .
# Statik HTML embed edildiği için ek kopya gerekmez
EXPOSE 8080
CMD ["./api-server"]
