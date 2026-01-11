#!/bin/bash

# Bitcoin Regtest 转账脚本
# 用法: ./send_btc.sh <发送地址> <接收地址> <金额> [找零地址]
# 示例:  ./send_btc.sh bcrt1q7hfafmwd45xjj2sqlul6lk7p5sewlmu5a2atnq bcrt1qkn6ca856lrptq3j5g0caq5dhugycz5208d4mrc 0.1
set -e

# 参数检查
if [ $# -lt 3 ]; then
    echo "用法: $0 <发送地址> <接收地址> <金额> [找零地址]"
    echo "示例: $0 bcrt1qsender... bcrt1qrecipient... 0.1"
    echo "示例: $0 bcrt1qsender... bcrt1qrecipient... 0.1 bcrt1qchange..."
    exit 1
fi

SENDER_ADDRESS=$1
RECIPIENT_ADDRESS=$2
AMOUNT=$3
CHANGE_ADDRESS=${4:-$SENDER_ADDRESS} # 默认找零给发送者

# Bitcoin CLI 命令前缀
BTC_CLI="docker exec bitcoin-regtest bitcoin-cli -regtest -rpcuser=test -rpcpassword=test"

echo "=========================================="
echo "Bitcoin Regtest 精确转账"
echo "=========================================="
echo "发送地址: $SENDER_ADDRESS"
echo "接收地址: $RECIPIENT_ADDRESS"
echo "金额: $AMOUNT BTC"
echo "找零地址: $CHANGE_ADDRESS"
echo "=========================================="

# 步骤0: 检查发送地址余额并锁定 UTXO
echo ""
echo "[0/5] 检查发送地址 UTXO..."
# 获取该地址的所有 UTXO
UTXOS=$($BTC_CLI listunspent 0 9999999 "[\"$SENDER_ADDRESS\"]")
UTXO_COUNT=$(echo $UTXOS | jq '. | length')

if [ "$UTXO_COUNT" -eq 0 ]; then
    echo "✗ 发送地址没有可用 UTXO"
    exit 1
fi

# 简单策略：使用第一个足够大的 UTXO 或者组合多个 UTXO
# 这里为了简化，我们让 walletcreatefundedpsbt 自动选择，但限制输入为该地址的 UTXO
# 注意：walletcreatefundedpsbt 的 inputs 参数可以指定具体的 UTXO

# 构建 inputs 数组
INPUTS=$(echo $UTXOS | jq '[.[] | {txid: .txid, vout: .vout}]')

echo "✓ 找到 $UTXO_COUNT 个可用 UTXO"

# 步骤1: 创建 PSBT
echo ""
echo "[1/5] 创建 PSBT..."
# 使用 inputs 参数限制只能使用发送地址的 UTXO
PSBT_RESULT=$($BTC_CLI walletcreatefundedpsbt "$INPUTS" "[{\"$RECIPIENT_ADDRESS\":$AMOUNT}]" 0 "{\"changeAddress\":\"$CHANGE_ADDRESS\",\"feeRate\":0.00001}")

PSBT=$(echo $PSBT_RESULT | jq -r '.psbt')
FEE=$(echo $PSBT_RESULT | jq -r '.fee')
CHANGEPOS=$(echo $PSBT_RESULT | jq -r '.changepos')

echo "✓ PSBT 创建成功"
echo "  手续费: $FEE BTC"
echo "  找零位置: $CHANGEPOS"

# 步骤2: 签名 PSBT
echo ""
echo "[2/5] 签名交易..."
SIGNED_RESULT=$($BTC_CLI walletprocesspsbt "$PSBT")
SIGNED_PSBT=$(echo $SIGNED_RESULT | jq -r '.psbt')
COMPLETE=$(echo $SIGNED_RESULT | jq -r '.complete')

if [ "$COMPLETE" != "true" ]; then
    echo "✗ 签名失败: 交易未完全签名"
    exit 1
fi

echo "✓ 签名成功"

# 步骤3: 完成交易
echo ""
echo "[3/5] 完成交易..."
FINALIZED_RESULT=$($BTC_CLI finalizepsbt "$SIGNED_PSBT")
RAW_TX=$(echo $FINALIZED_RESULT | jq -r '.hex')

if [ "$RAW_TX" == "null" ] || [ -z "$RAW_TX" ]; then
    echo "✗ 交易完成失败"
    exit 1
fi

echo "✓ 交易已完成"

# 步骤4: 广播交易
echo ""
echo "[4/5] 广播交易..."
TXID=$($BTC_CLI sendrawtransaction "$RAW_TX")

echo "✓ 交易已广播"
echo ""
echo "=========================================="
echo "交易成功！"
echo "=========================================="
echo "交易ID: $TXID"
echo "发送地址: $SENDER_ADDRESS"
echo "接收地址: $RECIPIENT_ADDRESS"
echo "金额: $AMOUNT BTC"
echo "手续费: $FEE BTC"
echo "=========================================="
echo ""
echo "提示: 运行以下命令挖矿确认交易"
echo "docker exec bitcoin-regtest bitcoin-cli -regtest -rpcuser=test -rpcpassword=test generatetoaddress 1 \"$SENDER_ADDRESS\""
