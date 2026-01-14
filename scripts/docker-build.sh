#!/bin/bash
# Obsidian Docker Build Script

set -e

# Configuration
IMAGE_NAME="obsidian-chain/obsidian"
VERSION="${VERSION:-1.0.0-alpha}"
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DATE=$(git log -1 --format=%cd --date=short 2>/dev/null || echo "unknown")

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}  Obsidian Docker Build Script${NC}"
echo -e "${GREEN}================================${NC}"
echo ""
echo "Version: ${VERSION}"
echo "Git Commit: ${GIT_COMMIT}"
echo "Git Date: ${GIT_DATE}"
echo ""

# Change to gobs directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/.."

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}Error: Docker daemon is not running${NC}"
    echo "Please start Docker Desktop or the Docker daemon"
    exit 1
fi

# Build the image
echo -e "${YELLOW}Building Docker image...${NC}"
docker build \
    -f obsidian/Dockerfile \
    --build-arg VERSION="${VERSION}" \
    --build-arg GIT_COMMIT="${GIT_COMMIT}" \
    --build-arg GIT_DATE="${GIT_DATE}" \
    -t "${IMAGE_NAME}:latest" \
    -t "${IMAGE_NAME}:${VERSION}" \
    .

echo ""
echo -e "${GREEN}Build successful!${NC}"
echo ""
echo "Images created:"
echo "  - ${IMAGE_NAME}:latest"
echo "  - ${IMAGE_NAME}:${VERSION}"
echo ""
echo "To run the node:"
echo "  docker run -d -p 8545:8545 -p 30303:30303 ${IMAGE_NAME}:latest"
echo ""
echo "Or use docker-compose:"
echo "  docker-compose up -d"
