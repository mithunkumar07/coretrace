# Multi-stage build for CoreTrace Agent
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments
ARG VERSION=dev
ARG BUILD_TIME
ARG COMMIT_SHA

# Build the application
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commitSHA=${COMMIT_SHA}" \
    -a -installsuffix cgo \
    -o coretrace-agent .

# Runtime stage - minimal image for production
FROM alpine:3.18 AS release

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

# Create app user
RUN addgroup -g 1001 coretrace && \
    adduser -D -s /bin/sh -u 1001 -G coretrace coretrace

# Create required directories
RUN mkdir -p /etc/coretrace /var/log/coretrace /tmp/coretrace && \
    chown -R coretrace:coretrace /etc/coretrace /var/log/coretrace /tmp/coretrace

# Copy built binary
COPY --from=builder /build/coretrace-agent /usr/local/bin/coretrace-agent

# Copy config file
COPY config.yaml /etc/coretrace/config.yaml

# Set ownership and permissions
RUN chmod +x /usr/local/bin/coretrace-agent && \
    chown -R coretrace:coretrace /etc/coretrace

# Switch to non-root user
USER coretrace

# Set working directory
WORKDIR /app

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep coretrace-agent > /dev/null || exit 1

# Labels
LABEL maintainer="CoreTrace Team <team@coretrace.io>" \
      description="CoreTrace Agent - SSH Session Monitoring" \
      org.opencontainers.image.title="CoreTrace Agent" \
      org.opencontainers.image.description="SSH session monitoring agent for security observability" \
      org.opencontainers.image.vendor="CoreTrace" \
      org.opencontainers.image.licenses="MIT"

# Default command
ENTRYPOINT ["/usr/local/bin/coretrace-agent"]