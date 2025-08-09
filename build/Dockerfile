FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod main.go ./

RUN go mod tidy && \
    go mod download && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server .

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /app/server /server

USER 65534:65534

EXPOSE 4050

ENTRYPOINT ["/server"]