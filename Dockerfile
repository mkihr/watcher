# This Dockerfile builds a minimal Go application container.
# 
# 1. Uses the official Golang image as a builder to compile the Go binary.
# 2. Sets the working directory to /app and copies the main Go source file.
# 3. Configures environment variables to disable CGO and target Linux AMD64.
# 4. Initializes a Go module, resolves dependencies, and builds the binary.
# 5. Creates a minimal final image using 'scratch' and copies the compiled binary.
# 6. Sets the binary as the container entrypoint.
FROM golang:1.24.4 AS builder

WORKDIR /app
COPY watcher.go .
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go mod init watcher && go mod tidy && go build -o watcher

FROM scratch
COPY --from=builder /app/watcher /watcher
ENTRYPOINT ["/watcher"]
