# HIGUN - Hyper Indexer of General UTXO Network

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)](https://golang.org/)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](https://www.docker.com/)

HIGUN (Hyper Indexer of General UTXO Network) is a high-performance, universal UTXO network indexer designed to handle multiple blockchain networks simultaneously. As a Bitcoin sidechain, MVC has been committed to expanding Bitcoin's performance. However, there has been a lack of a high-performance UTXO indexer that integrates Bitcoin and other sidechains in the market.

This high-performance universal UTXO network indexer, jointly developed by the MVC technical team and Metalet team, is the first to index both Bitcoin and MVC UTXOs simultaneously in a single indexer, treating these two as an integrated network. It adopts Pebble database instead of traditional databases, and a medium-configuration server can actually process tens of millions of transactions daily in performance tests.

## 🌟 Key Features

- **Multi-Chain Support**: Unified indexing for Bitcoin, MVC, and Dogecoin blockchains
- **High Performance**: Capable of processing tens of millions of transactions daily with medium-configuration servers
- **Efficient Storage**: Uses Pebble database instead of traditional databases for superior performance
- **Comprehensive Indexing**: 
  - UTXO tracking (income/spend)
  - FT (Fungible Token) protocol support
  - NFT protocol support
  - Block information indexing
  - Transaction history
- **Real-time Monitoring**: ZeroMQ integration for live transaction tracking
- **Mempool Support**: Track unconfirmed transactions
- **Reorganization Handling**: Automatic blockchain reorganization detection and recovery
- **RESTful API**: Complete HTTP API for data queries
- **Docker Ready**: Easy deployment with Docker and Docker Compose

## 📋 Table of Contents

- [Architecture](#architecture)
- [Supported Chains](#supported-chains)
- [Current Features](#current-features)
- [Prerequisites](#prerequisites)
- [Configuration](#configuration)
- [Deployment](#deployment)
- [API Endpoints](#api-endpoints)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

## 🏗️ Architecture

HIGUN adopts a modular architecture with chain adapter pattern to support multiple blockchains:

### Core Components

- **ChainAdapter Interface**: Defines common blockchain operations
- **Specific Adapters**: BTC, MVC, and Dogecoin implementations
- **Factory Pattern**: Dynamically creates adapters based on configuration
- **Unified Data Format**: All chains return data in standardized format

### Storage Architecture

- **Sharded Storage**: Multiple Pebble database shards for parallel processing
- **Separated Stores**:
  - UTXO Store: Transaction outputs by hash
  - Address Store: Income transactions by address
  - Spend Store: Spent transactions by address
  - Meta Store: System metadata and state
  - FT/NFT Stores: Token-specific data

## 🔗 Supported Chains

| Chain | Network | Status | Features |
|-------|---------|--------|----------|
| **Bitcoin** | Mainnet/Testnet | ✅ Stable | UTXO indexing, mempool tracking |
| **MVC** | Mainnet/Testnet | ✅ Stable | UTXO, FT, NFT, block info |
| **Dogecoin** | Mainnet/Testnet | ✅ Stable | UTXO indexing with AuxPoW support |

## Current Features (v1.0)

- **Bitcoin UTXO Indexing**
- **MVC UTXO Indexing** 
- **MVC FT Indexing**
- **Dogecoin UTXO Indexing**
- **Block Information Indexing**
- **Mempool Tracking**
- **Reorganization Handling**

## Future Plans

Future versions will add support for more asset protocols and more Bitcoin sidechains, including:
- Lightning Network integration
- More sidechain support
- Advanced query capabilities
- WebSocket real-time updates

## 📦 Prerequisites

### System Requirements

- **OS**: Linux (Ubuntu 20.04+ recommended), macOS, or Windows with WSL2
- **Memory**: Minimum 8GB RAM (16GB+ recommended for production)
- **Storage**: 
  - Bitcoin: 500GB+ SSD
  - MVC: 200GB+ SSD
  - Dogecoin: 100GB+ SSD
- **CPU**: 4+ cores recommended

### Software Dependencies

- **Docker** 20.10+
- **Docker Compose** 1.29+ (optional)
- **Go** 1.21+ (for development)
- **Blockchain Node**: 
  - Bitcoin Core 24.0+
  - MVC Node 0.1.1+
  - Dogecoin Core 1.14+

## Deployment Instructions

### 1. Bitcoin UTXO Indexing

For Bitcoin UTXO indexing, use the standard Docker deployment:

### 2. MVC UTXO Indexing + Block Information Indexing

MVC UTXO indexing and block information indexing run together in the same service. Use the standard Docker deployment:

```bash
# Build the image
docker build -t higun_mvc .

# Run the container
  docker run -d   --name higun_mvc  --network=host  --restart=always -m 22g   -v /mnt/higun_mvc/utxo_indexer:/app/utxo_indexer   -v /mnt/higun_mvc/config.yaml:/app/config.yaml -v /mnt/higun_mvc/latest_block.txt:/app/latest_block.txt  -v /mnt/higun_mvc/data:/app/data   -v /mnt/higun_mvc/blockinfo_data:/app/blockinfo_data   higun_mvc
```

### 3. MVC FT Indexing

For MVC FT (Fungible Token) indexing, use the dedicated deployment script:

```bash
# Navigate to the deploy directory
cd deploy/

# Run the FT deployment script
./deploy.ft.sh
```

The FT deployment script will:
- Stop and remove existing containers
- Build the FT-specific Docker image
- Start the FT indexer service
- Display service status

**FT Indexer Details:**
- Container name: `ft-utxo-indexer`
- Port: `7789`
- Configuration: Uses `config_mvc_ft.yaml` by default
- Resource limits: 4 CPU cores, 8GB memory

## Configuration Files

### Standard Configuration (`config.yaml`)

Used for Bitcoin UTXO indexing, MVC UTXO indexing, and block information indexing (MVC UTXO and block information run together):

```yaml
# UTXO Indexer Configuration
network: "mainnet" # mainnet/testnet/regtest
data_dir: "/Volumes/MyData/mvc/test"
shard_count: 2
tx_concurrency: 64 # How many concurrent requests to fetch raw transactions from nodes
workers: 4
batch_size: 20000
cpu_cores: 1 # 4-core CPU
memory_gb: 4 # 64GB memory
high_perf: true # Prefer performance
api_port: "8080"
zmq_address: "tcp://localhost:28336"  # ZeroMQ connection address
mempool_clean_start_height: 567 # Mempool cleaning start height, avoid cleaning from the genesis block
max_tx_per_batch: 30000

# Bitcoin RPC Configuration
rpc:
  chain: "mvc"
  host: "localhost"
  port: "9882"
  user: ""
  password: ""
```

### FT Configuration (`config_mvc_ft.yaml`)

Used for MVC FT indexing:

```yaml
# MVC MetaContract FT UTXO Indexer Configuration
network: "mainnet"  # mainnet/testnet/regtest
data_dir: "data/mainnet"
shard_count: 2
cpu_cores: 4      # CPU cores to use
memory_gb: 8      # Memory allocation in GB
high_perf: true    # Performance optimization flag
api_port: "7789"
zmq_address: "tcp://localhost:28333"  # ZeroMQ connection address
mempool_clean_start_height: 567   # Mempool cleaning start height, avoid cleaning from the genesis block
max_tx_per_batch: 1000
raw_tx_in_block: true

# MVC RPC Configuration
rpc:
  chain: "mvc"
  host: "localhost"
  port: "9882"
  user: ""
  password: ""
```

## Configuration Parameters

### General Parameters

- **network**: Network type (`mainnet`/`testnet`/`regtest`)
- **data_dir**: Data storage directory
- **shard_count**: Number of database shards for performance optimization
- **cpu_cores**: Number of CPU cores to use
- **memory_gb**: Memory allocation in GB
- **high_perf**: Performance optimization flag
- **api_port**: API service port
- **zmq_address**: ZeroMQ connection address for real-time transaction monitoring
- **mempool_clean_start_height**: Starting block height for mempool cleaning
- **max_tx_per_batch**: Maximum transactions per batch for processing
- **raw_tx_in_block**: Enable raw transaction processing in blocks (FT specific)

### RPC Configuration

- **chain**: Blockchain type (`mvc` for MVC, `btc` for Bitcoin, `doge` for Dogecoin)
- **host**: RPC server host address
- **port**: RPC server port
- **user**: RPC authentication username
- **password**: RPC authentication password

## 📡 API Endpoints

### Base URL
```
http://localhost:{api_port}
```

### UTXO Endpoints

#### Get UTXOs by Address
```bash
GET /utxos?address={address}&limit={limit}

# Example
curl "http://localhost:8080/utxos?address=1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa&limit=10"
```

#### Check UTXO Spend Status
```bash
POST /check-utxo
Content-Type: application/json

{
  "outpoints": ["txid:vout"]
}
```

### FT Endpoints

#### Get FT Balance
```bash
GET /ft/balance?address={address}&codeHash={codeHash}&genesis={genesis}
```

#### Get FT Info
```bash
GET /db/ft/info?codeHash={codeHash}&genesis={genesis}
```

### System Endpoints

#### Health Check
```bash
GET /health
```

#### Reindex Blocks
```bash
POST /reindex

{
  "start_height": 100000,
  "end_height": 100100
}
```

For complete API documentation, see [CHECK_UTXO_API.md](docs/CHECK_UTXO_API.md)

## Service Management

### View Logs

```bash
# Standard indexer logs
docker logs -f mvc-utxo-indexer

# FT indexer logs
docker logs -f ft-utxo-indexer
# Or use docker-compose
docker-compose -f deploy/docker-compose.ft.yml logs -f
```

### Stop Services

```bash
# Stop standard indexer
docker stop mvc-utxo-indexer

# Stop FT indexer
docker stop ft-utxo-indexer
# Or use docker-compose
docker-compose -f deploy/docker-compose.ft.yml down
```

### Restart Services

```bash
# Restart standard indexer
docker restart mvc-utxo-indexer

# Restart FT indexer
docker restart ft-utxo-indexer
# Or use docker-compose
docker-compose -f deploy/docker-compose.ft.yml restart
```

## ⚡ Performance Optimization

### Database Tuning

1. **Shard Configuration**
   - Increase `shard_count` for high-load scenarios
   - Recommended: 2-4 shards for most setups

2. **Batch Processing**
   - Adjust `batch_size` based on available memory
   - Recommended: 10000-30000 transactions

3. **Concurrency Settings**
   - `tx_concurrency`: Number of parallel RPC requests
   - `workers`: Number of processing goroutines

### System Optimization

```bash
# Increase file descriptor limits
ulimit -n 65536

# Use SSD storage with appropriate mount options
mount -o noatime,nodiratime /dev/sdX /mnt/data
```

## 🛠️ Development

### Project Structure

```
higun/
├── api/                    # HTTP API handlers
├── blockchain/            # Blockchain clients and adapters
│   ├── adapter.go        # Chain adapter interface
│   ├── adapter_btc.go    # Bitcoin implementation
│   ├── adapter_mvc.go    # MVC implementation
│   ├── adapter_doge.go   # Dogecoin implementation
│   └── factory.go        # Adapter factory
├── indexer/              # Core indexing logic
│   ├── utxo.go          # UTXO indexing
│   ├── query.go         # Query operations
│   └── reorg.go         # Reorganization handling
├── storage/             # Database abstraction
├── mempool/            # Mempool management
├── config/            # Configuration
├── common/           # Shared utilities
├── deploy/          # Deployment scripts
├── docs/           # Documentation
└── main.go        # Entry point
```

### Building from Source

```bash
# Clone repository
git clone https://github.com/your-org/higun.git
cd higun

# Install dependencies
go mod download

# Build binary
make build

# Run tests
make test

# Build Docker image
make docker-build
```

## 🐛 Troubleshooting

### Common Issues

#### 1. Out of Memory
**Solution**: Reduce `batch_size` and `tx_concurrency`, increase Docker memory limit

#### 2. RPC Connection Failures
**Solution**: Verify RPC credentials, check node is running, ensure firewall allows connections

#### 3. Slow Indexing
**Solution**: Enable `high_perf: true`, increase `tx_concurrency`, use SSD storage

#### 4. Database Corruption
**Solution**: Stop indexer, backup data, remove corrupted database, restart

### Debug Mode

Enable verbose logging:
```bash
export LOG_LEVEL=debug
docker run -e LOG_LEVEL=debug higun:latest
```

## 📊 Performance Benchmarks

| Chain | Blocks/sec | Tx/sec | Storage/1M blocks |
|-------|-----------|--------|-------------------|
| Bitcoin | 50-100 | 5,000-10,000 | ~300GB |
| MVC | 100-200 | 20,000-50,000 | ~200GB |
| Dogecoin | 100-150 | 10,000-20,000 | ~100GB |

*Benchmarks on 4-core CPU, 16GB RAM, SSD storage*

## 📝 Documentation

Comprehensive documentation is available in the `docs/` directory:

- [Chain Adapter Design](docs/CHAIN_ADAPTER_DESIGN.md)
- [Multi-Chain Architecture](docs/README_MULTI_CHAIN.md)
- [Dogecoin Integration](docs/DOGE_ADAPTER_GUIDE.md)
- [Quick Start Guide](docs/QUICK_START.md)
- [API Documentation](docs/CHECK_UTXO_API.md)

## 🤝 Contributing

We welcome contributions! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Submit a pull request

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 🔐 Security

- Store RPC credentials securely
- Use firewall rules to limit API access
- Regular backups of Pebble database
- Keep blockchain nodes updated

## 📜 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- **MVC Team**: Core development and architecture
- **Metalet Team**: Product design and integration
- **Bitcoin Core**: Reference implementation
- **btcsuite**: Go Bitcoin libraries
- **Pebble DB**: High-performance storage engine

## 📞 Support and Community

- **Issues**: [GitHub Issues](https://github.com/your-org/higun/issues)
- **Documentation**: Check the `docs/` directory for detailed guides

## 🗺️ Roadmap

### Current (v1.x)
- ✅ Bitcoin UTXO indexing
- ✅ MVC UTXO indexing
- ✅ MVC FT/NFT support
- ✅ Dogecoin support
- ✅ Mempool tracking

### Upcoming (v2.x)
- 🔄 Lightning Network integration
- 🔄 Advanced query filters
- 🔄 WebSocket support
- 🔄 Cross-chain analytics

### Future
- 📅 More sidechain support
- 📅 Advanced analytics
- 📅 Inscription protocol support

---
