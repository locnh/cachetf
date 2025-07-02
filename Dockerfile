# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# Final stage
FROM alpine:3.22

# Install CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/server .

# Create cache directory and set permissions
RUN mkdir -p /cache && chown -R 1000:1000 /cache

# Set environment variables with defaults
ENV GIN_MODE=release
ENV PORT=8080
ENV CACHE_DIR=/cache
ENV LOG_LEVEL=info

# Expose the application port
EXPOSE 8080

# Run as non-root user for security
USER 1000

# Command to run the application
CMD ["/app/server"]
