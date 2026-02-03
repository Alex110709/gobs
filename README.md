# Obsidian Blockchain (gobs)

Obsidian is a privacy-focused, Ethereum-compatible blockchain implementation based on `go-ethereum`. It introduces specialized P2P synchronization and consensus mechanisms designed for secure and efficient decentralized operations.

## Features

- **Ethereum Compatibility**: Fully compatible with Ethereum's execution layer and RPC standards.
- **Enhanced P2P Layer**: Implements batch synchronization using a dedicated `Downloader` integrated into the P2P `Handler`.
- **Stealth Addresses**: Built-in support for privacy-preserving stealth addresses.
- **Consensus**: Utilizes `obsidianash` consensus mechanism.
- **CI/CD Integrated**: Automated testing, linting, security scanning (Gosec & Trivy), and Docker builds.

## Repository Structure

- `obsidian/`: Core blockchain implementation.
  - `cmd/obsidian/`: Command-line interface for the node.
  - `p2p/`: Custom P2P networking and synchronization logic.
  - `eth/`: Ethereum protocol backend.
  - `stealth/`: Stealth address implementation.
- `go-ethereum/`: Submodule/dependency for Ethereum base logic.

## Getting Started

### Prerequisites

- Go 1.24 or higher
- Docker (optional, for containerized deployment)
- Make

### Building the Node

To build the `obsidian` binary:

```bash
cd obsidian
make build
```

### Running with Docker

Pull the latest image from GitHub Container Registry:

```bash
docker pull ghcr.io/hiteyy/obsidian-node:latest
```

Run a node:

```bash
docker run -d --name obsidian-node \
  -p 8545:8545 -p 8333:8333 \
  ghcr.io/hiteyy/obsidian-node:latest \
  run --datadir /data --http --http.addr 0.0.0.0
```

## Development

### Running Tests

```bash
cd obsidian
go test ./...
```

### Linting

The project uses `golangci-lint`:

```bash
cd obsidian
golangci-lint run
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.
