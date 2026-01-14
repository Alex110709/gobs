# Obsidian Makefile

.PHONY: all build test clean docker docker-push help

# Variables
VERSION ?= 1.0.0-alpha
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DATE ?= $(shell git log -1 --format=%cd --date=short 2>/dev/null || echo "unknown")
IMAGE_NAME = obsidian-chain/obsidian
LDFLAGS = -ldflags "-X main.gitCommit=$(GIT_COMMIT) -X main.gitDate=$(GIT_DATE) -s -w"

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get

# Main binary
BINARY_NAME = obsidian
BINARY_PATH = obsidian/$(BINARY_NAME)

all: build

## Build the obsidian binary
build:
	@echo "Building $(BINARY_NAME)..."
	cd obsidian && $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) ./cmd/obsidian
	@echo "Build complete: $(BINARY_PATH)"

## Run all tests
test:
	@echo "Running tests..."
	cd obsidian && $(GOTEST) -v ./...

## Clean build artifacts
clean:
	@echo "Cleaning..."
	cd obsidian && $(GOCLEAN)
	rm -f $(BINARY_PATH)

## Build Docker image
docker:
	@echo "Building Docker image $(IMAGE_NAME):$(VERSION)..."
	docker build -f obsidian/Dockerfile \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_DATE=$(GIT_DATE) \
		-t $(IMAGE_NAME):$(VERSION) \
		-t $(IMAGE_NAME):latest \
		.

## Push Docker image to registry
docker-push: docker
	@echo "Pushing Docker image..."
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest

## Run the node locally
run: build
	./$(BINARY_PATH) run --datadir ./data --http --verbosity 4

## Initialize genesis
init: build
	./$(BINARY_PATH) init obsidian/genesis/obsidian.json --datadir ./data

## Generate stealth keys
stealth-generate: build
	./$(BINARY_PATH) stealth generate

## Show version
version: build
	./$(BINARY_PATH) version

## Install dependencies
deps:
	cd obsidian && $(GOGET) -v ./...

## Format code
fmt:
	cd obsidian && $(GOCMD) fmt ./...

## Lint code
lint:
	cd obsidian && golangci-lint run

## Help
help:
	@echo "Obsidian Build System"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  build          Build the obsidian binary"
	@echo "  test           Run all tests"
	@echo "  clean          Clean build artifacts"
	@echo "  docker         Build Docker image"
	@echo "  docker-push    Push Docker image to registry"
	@echo "  run            Run the node locally"
	@echo "  init           Initialize with genesis"
	@echo "  stealth-generate  Generate stealth key pair"
	@echo "  version        Show version info"
	@echo "  deps           Install dependencies"
	@echo "  fmt            Format code"
	@echo "  lint           Lint code"
	@echo "  help           Show this help"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION        Version string (default: $(VERSION))"
	@echo "  IMAGE_NAME     Docker image name (default: $(IMAGE_NAME))"
