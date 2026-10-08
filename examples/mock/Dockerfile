FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mock ./main.go

FROM alpine:3.20
COPY --from=builder /out/mock /usr/local/bin/mock
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/mock"]
