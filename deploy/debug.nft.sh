#!/bin/bash

# Exit on error
set -e

echo "Starting local debug of NFT-UTXO indexer..."

# Run with local config file
echo "Starting service with local config file..."
go run ../apps/nft-main/main.go --config config_mvc_nft_local.yaml

echo "Service stopped"

