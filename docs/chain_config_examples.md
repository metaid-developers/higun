# 多链配置示例

## BTC 主网配置 (config_btc_mainnet.yaml)

```yaml
# 链标识
chain: "btc"

# 网络类型
network: "mainnet"

# 区块信息索引器
block_info_indexer: true

# 数据目录 - 建议按链分离
data_dir: "/data/higun/btc/mainnet"
backup_dir: "/data/higun/btc/backups"
block_files_dir: "/data/higun/btc/blockFiles"

# 性能配置
shard_count: 16
tx_concurrency: 64
workers: 8
batch_size: 20000
cpu_cores: 8
memory_gb: 32
high_perf: true

# API 端口
api_port: "3001"

# ZeroMQ 配置
zmq_address:
  - "tcp://127.0.0.1:28332"
mempool_clean_start_height: 0
max_tx_per_batch: 30000
zmq_reconnect_interval: 5

# Bitcoin RPC 配置
rpc:
  chain: "btc"  # 必须与顶层 chain 一致
  host: "127.0.0.1"
  port: "8332"
  user: "bitcoin"
  password: "your_btc_password"
```

## MVC 主网配置 (config_mvc_mainnet.yaml)

```yaml
# 链标识
chain: "mvc"

# 网络类型
network: "mainnet"

# 区块信息索引器
block_info_indexer: true

# 数据目录 - 建议按链分离
data_dir: "/data/higun/mvc/mainnet"
backup_dir: "/data/higun/mvc/backups"
block_files_dir: "/data/higun/mvc/blockFiles"

# 性能配置
shard_count: 16
tx_concurrency: 64
workers: 8
batch_size: 20000
cpu_cores: 8
memory_gb: 32
high_perf: true

# API 端口 - 注意与 BTC 不同端口
api_port: "3002"

# ZeroMQ 配置
zmq_address:
  - "tcp://127.0.0.1:28333"
mempool_clean_start_height: 0
max_tx_per_batch: 30000
zmq_reconnect_interval: 5

# MVC RPC 配置
rpc:
  chain: "mvc"  # 必须与顶层 chain 一致
  host: "127.0.0.1"
  port: "9882"
  user: "mvc"
  password: "your_mvc_password"
```

## BTC 测试网配置 (config_btc_testnet.yaml)

```yaml
chain: "btc"
network: "testnet"
block_info_indexer: true

data_dir: "/data/higun/btc/testnet"
backup_dir: "/data/higun/btc/testnet/backups"
block_files_dir: "/data/higun/btc/testnet/blockFiles"

shard_count: 8
tx_concurrency: 32
workers: 4
batch_size: 10000
cpu_cores: 4
memory_gb: 16
high_perf: true

api_port: "3011"

zmq_address:
  - "tcp://127.0.0.1:28332"
mempool_clean_start_height: 0
max_tx_per_batch: 20000
zmq_reconnect_interval: 5

rpc:
  chain: "btc"
  host: "127.0.0.1"
  port: "18332"
  user: "testbtc"
  password: "testbtc"
```

## BTC Regtest 配置 (config_btc_regtest.yaml)

```yaml
chain: "btc"
network: "regtest"
block_info_indexer: true

data_dir: "/data/higun/btc/regtest"
backup_dir: "/data/higun/btc/regtest/backups"
block_files_dir: "/data/higun/btc/regtest/blockFiles"

shard_count: 2
tx_concurrency: 16
workers: 2
batch_size: 5000
cpu_cores: 2
memory_gb: 4
high_perf: false

api_port: "3021"

zmq_address:
  - "tcp://127.0.0.1:28333"
mempool_clean_start_height: 0
max_tx_per_batch: 10000
zmq_reconnect_interval: 1

rpc:
  chain: "btc"
  host: "127.0.0.1"
  port: "18443"
  user: "test"
  password: "test"
```

## 启动命令示例

### 启动 BTC 主网索引器
```bash
./utxo_indexer --config config_btc_mainnet.yaml
```

### 启动 MVC 主网索引器
```bash
./utxo_indexer --config config_mvc_mainnet.yaml
```

### 启动多个实例 (使用 systemd)

#### btc-indexer.service
```ini
[Unit]
Description=BTC UTXO Indexer
After=network.target

[Service]
Type=simple
User=bitcoin
WorkingDirectory=/opt/higun
ExecStart=/opt/higun/utxo_indexer --config /etc/higun/config_btc_mainnet.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

#### mvc-indexer.service
```ini
[Unit]
Description=MVC UTXO Indexer
After=network.target

[Service]
Type=simple
User=mvc
WorkingDirectory=/opt/higun
ExecStart=/opt/higun/utxo_indexer --config /etc/higun/config_mvc_mainnet.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

## 配置验证

### 必须检查的配置项
1. `chain` 字段必须与 `rpc.chain` 一致
2. `data_dir` 不同链不能重复
3. `api_port` 不同实例不能重复
4. `zmq_address` 要匹配对应链的 ZMQ 端口
5. `rpc.port` 要匹配对应链的 RPC 端口

### 配置验证脚本
```bash
#!/bin/bash
# validate_config.sh

CONFIG_FILE=$1

if [ -z "$CONFIG_FILE" ]; then
    echo "Usage: $0 <config_file>"
    exit 1
fi

# 提取配置值
CHAIN=$(grep "^chain:" $CONFIG_FILE | awk '{print $2}' | tr -d '"')
RPC_CHAIN=$(grep "chain:" $CONFIG_FILE | tail -1 | awk '{print $2}' | tr -d '"')
DATA_DIR=$(grep "^data_dir:" $CONFIG_FILE | awk '{print $2}' | tr -d '"')
API_PORT=$(grep "^api_port:" $CONFIG_FILE | awk '{print $2}' | tr -d '"')

echo "Validating $CONFIG_FILE..."
echo "Chain: $CHAIN"
echo "RPC Chain: $RPC_CHAIN"
echo "Data Dir: $DATA_DIR"
echo "API Port: $API_PORT"

# 验证 chain 一致性
if [ "$CHAIN" != "$RPC_CHAIN" ]; then
    echo "ERROR: chain ($CHAIN) and rpc.chain ($RPC_CHAIN) must match"
    exit 1
fi

# 验证数据目录
if [ ! -d "$DATA_DIR" ]; then
    echo "WARNING: Data directory $DATA_DIR does not exist, will be created"
fi

# 验证端口
if netstat -tuln | grep -q ":$API_PORT "; then
    echo "ERROR: API port $API_PORT is already in use"
    exit 1
fi

echo "Configuration validated successfully"
```

## Docker Compose 示例

```yaml
version: '3.8'

services:
  btc-indexer:
    image: utxo-indexer:latest
    container_name: btc-indexer
    volumes:
      - /data/higun/btc:/data/higun/btc
      - ./config_btc_mainnet.yaml:/app/config.yaml
    ports:
      - "3001:3001"
    environment:
      - CHAIN=btc
    restart: unless-stopped

  mvc-indexer:
    image: utxo-indexer:latest
    container_name: mvc-indexer
    volumes:
      - /data/higun/mvc:/data/higun/mvc
      - ./config_mvc_mainnet.yaml:/app/config.yaml
    ports:
      - "3002:3002"
    environment:
      - CHAIN=mvc
    restart: unless-stopped
```

## 注意事项

1. **数据隔离**: 不同链的数据必须存储在不同目录
2. **端口冲突**: 确保 API 端口、RPC 端口、ZMQ 端口不冲突
3. **资源分配**: 多实例运行时注意 CPU 和内存分配
4. **配置一致性**: chain 字段必须与 rpc.chain 一致
5. **网络配置**: 确保 network 字段与节点配置匹配
