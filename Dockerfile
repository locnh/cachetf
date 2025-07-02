# Final stage
FROM alpine:3.22

ARG TARGETOS
ARG TARGETARCH

# Install CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /app

# Copy the binary from builder
COPY ./build/server_${TARGETOS}_${TARGETARCH} /app/server

# Create cache directory and set permissions
RUN mkdir -p /cache && chown -R 1000:1000 /cache

# Set environment variables with defaults
ENV GIN_MODE=release
ENV PORT=8080
ENV METRICS_PORT=9100
ENV CACHE_DIR=/cache
ENV LOG_LEVEL=info

# Expose the application port
EXPOSE 8080
EXPOSE 9100

# Run as non-root user for security
USER 1000

# Command to run the application
CMD ["/app/server"]
