FROM golang:1.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/main .

# Runtime
FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite
WORKDIR /root
COPY --from=builder /app/main /root/main
COPY .env .
COPY migrations ./migrations
COPY db ./db

RUN chmod +x /root/main
RUN ls -l /root/

EXPOSE 8080

CMD ["/root/main"]
