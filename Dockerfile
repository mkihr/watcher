FROM golang:1.24.4 AS builder

WORKDIR /app
COPY watcher.go .
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go mod init watcher && go mod tidy && go build -o watcher

FROM scratch
COPY --from=builder /app/watcher /watcher
ENTRYPOINT ["/watcher"]
