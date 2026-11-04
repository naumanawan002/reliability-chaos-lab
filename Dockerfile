FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/order-service ./services/order-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/inventory-service ./services/inventory-service

FROM alpine:3.20
RUN apk --no-cache add ca-certificates curl

WORKDIR /root/
COPY --from=builder /bin/order-service /usr/local/bin/order-service
COPY --from=builder /bin/inventory-service /usr/local/bin/inventory-service

EXPOSE 8081 8082
