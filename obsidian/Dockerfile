# Obsidian Node Dockerfile
# Build from the gobs directory: docker build -f obsidian/Dockerfile -t obsidian .

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev linux-headers git make

# Set working directory
WORKDIR /build

# Copy go-ethereum (must be in build context)
COPY go-ethereum /build/go-ethereum

# Copy obsidian source
COPY obsidian /build/obsidian

# Set working directory to obsidian
WORKDIR /build/obsidian

# Build arguments for version info
ARG VERSION=1.0.0-alpha
ARG GIT_COMMIT=unknown
ARG GIT_DATE=unknown

# Build the binary
RUN CGO_ENABLED=1 go build \
    -ldflags "-X main.gitCommit=${GIT_COMMIT} -X main.gitDate=${GIT_DATE} -s -w" \
    -o /obsidian \
    ./cmd/obsidian

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# Create obsidian user
RUN addgroup -g 1000 obsidian && \
    adduser -D -u 1000 -G obsidian obsidian

# Create directories
RUN mkdir -p /data /etc/obsidian && chown -R obsidian:obsidian /data

# Copy binary from builder
COPY --from=builder /obsidian /usr/local/bin/obsidian

# Copy genesis file
COPY obsidian/genesis/obsidian.json /etc/obsidian/genesis.json

# Set permissions
RUN chmod +x /usr/local/bin/obsidian

# Switch to non-root user
USER obsidian

# Set working directory
WORKDIR /data

# Expose ports
# P2P
EXPOSE 8333/tcp
EXPOSE 8333/udp
# HTTP RPC
EXPOSE 8545/tcp
# WebSocket RPC  
EXPOSE 8546/tcp

# Data volume
VOLUME ["/data"]

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8545 || exit 1

# Default command
ENTRYPOINT ["obsidian"]
CMD ["run", "--datadir", "/data", "--http", "--http.addr", "0.0.0.0"]
