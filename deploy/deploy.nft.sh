#!/bin/bash

# Exit on error
set -e

echo "Starting deployment of NFT-UTXO indexer..."

# Stop and remove old containers (if exist)
docker-compose -f docker-compose.nft.yml down || true

# Build new image
echo "Building new image..."
docker-compose -f docker-compose.nft.yml build

# Start service
echo "Starting service..."
docker-compose -f docker-compose.nft.yml up -d

# Check service status
echo "Checking service status..."
docker-compose -f docker-compose.nft.yml ps

echo "Deployment completed!"
echo "You can view logs with the following command:"
echo "docker-compose -f docker-compose.nft.yml logs -f"

