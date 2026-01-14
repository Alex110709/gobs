#!/bin/bash
# Obsidian Docker Push Script

set -e

# Configuration
IMAGE_NAME="obsidian-chain/obsidian"
VERSION="${VERSION:-1.0.0-alpha}"
REGISTRY="${REGISTRY:-docker.io}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}  Obsidian Docker Push Script${NC}"
echo -e "${GREEN}================================${NC}"
echo ""
echo "Image: ${IMAGE_NAME}"
echo "Version: ${VERSION}"
echo "Registry: ${REGISTRY}"
echo ""

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}Error: Docker daemon is not running${NC}"
    exit 1
fi

# Check if image exists
if ! docker image inspect "${IMAGE_NAME}:${VERSION}" > /dev/null 2>&1; then
    echo -e "${RED}Error: Image ${IMAGE_NAME}:${VERSION} not found${NC}"
    echo "Run docker-build.sh first"
    exit 1
fi

# Tag for registry
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}"

echo -e "${YELLOW}Tagging images for ${REGISTRY}...${NC}"
docker tag "${IMAGE_NAME}:${VERSION}" "${FULL_IMAGE}:${VERSION}"
docker tag "${IMAGE_NAME}:latest" "${FULL_IMAGE}:latest"

# Push to registry
echo -e "${YELLOW}Pushing to ${REGISTRY}...${NC}"
docker push "${FULL_IMAGE}:${VERSION}"
docker push "${FULL_IMAGE}:latest"

echo ""
echo -e "${GREEN}Push successful!${NC}"
echo ""
echo "Images pushed:"
echo "  - ${FULL_IMAGE}:${VERSION}"
echo "  - ${FULL_IMAGE}:latest"
echo ""
echo "To pull the image:"
echo "  docker pull ${FULL_IMAGE}:latest"
