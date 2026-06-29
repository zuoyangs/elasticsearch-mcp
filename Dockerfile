# ============================================================
# Stage 1: Builder
# ============================================================
FROM harbor.<your-domain>.net/infra/golang:1.26.2-alpine3.23 AS builder

# Go module proxy for faster download in China
ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn,direct

# Install build dependencies (git needed for go modules)
# Switch to Aliyun mirror for faster download in China
RUN sed -i 's/https:\/\/[^/]*\/alpine\//https:\/\/mirrors.aliyun.com\/alpine\//g' /etc/apk/repositories && \
    apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod/sum first for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary with CGO disabled for scratch compatibility
# Set GOOS=linux explicitly for cross-compile safety
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -extldflags '-static'" \
    -o elasticsearch-mcp \
    .

# ============================================================
# Stage 2: Runtime
# ============================================================
FROM harbor.<your-domain>.net/infra/alpine:3.23.4 AS runtime

# Install runtime dependencies
# Switch to Aliyun mirror for faster download in China
RUN sed -i 's/https:\/\/[^/]*\/alpine\//https:\/\/mirrors.aliyun.com\/alpine\//g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata

# Create non-root user for security
RUN adduser -D -u 1000 appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/elasticsearch-mcp /app/elasticsearch-mcp

# Copy default config (can be overridden at runtime via volume mount)
COPY --from=builder /build/etc /app/etc

# Create log directory with correct permissions
RUN mkdir -p /app/logs && chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Environment variables for configuration
ENV MCP_TRANSPORT=streamable-http \
    MCP_PORT=8080 \
    MCP_LOG_LEVEL=info \
    MCP_LOG_DIR=/app/logs

# Expose HTTP port
EXPOSE 8080

ENTRYPOINT ["/app/elasticsearch-mcp"]
