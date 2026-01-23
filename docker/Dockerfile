# VCDeploy Master Dockerfile
# Multi-stage build for minimal image size

FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o vcdeploy ./cmd/vcdeploy

# Final image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    openssh-client \
    git

# Create non-root user
RUN addgroup -S vcdeploy && adduser -S vcdeploy -G vcdeploy

# Create directories
RUN mkdir -p /etc/vcdeploy /var/lib/vcdeploy /var/log/vcdeploy && \
    chown -R vcdeploy:vcdeploy /etc/vcdeploy /var/lib/vcdeploy /var/log/vcdeploy

# Copy binary from builder
COPY --from=builder /build/vcdeploy /usr/local/bin/vcdeploy

# Copy default templates
COPY web/templates /var/lib/vcdeploy/templates

# Set permissions
RUN chmod +x /usr/local/bin/vcdeploy

# Switch to non-root user
USER vcdeploy

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q --spider http://localhost:8080/api/v1/health || exit 1

# Default command
ENTRYPOINT ["vcdeploy"]
CMD ["master", "start", "--config", "/etc/vcdeploy/master.yaml"]
