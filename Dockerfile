FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/elegba ./cmd/elegba

FROM alpine:3.20
RUN addgroup -S elegba && adduser -S -G elegba elegba
COPY --from=builder /out/elegba /usr/local/bin/elegba
USER elegba
EXPOSE 8080 9090
ENTRYPOINT ["/usr/local/bin/elegba"]
