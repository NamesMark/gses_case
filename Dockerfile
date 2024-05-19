FROM golang:1.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main . 
RUN ls -la /app

# Runtime
FROM debian:12.5-slim
RUN apt-get update && apt-get install -y ca-certificates sqlite3
WORKDIR /root/
COPY --from=builder /app/main .
RUN ls -la /root
COPY .env .
COPY db ./db

RUN chmod +x main

EXPOSE 8080

ENTRYPOINT ["/root/main"]
