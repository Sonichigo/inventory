#!/bin/bash
# Build and push both bad and good versions of BBQBookkeeper

set -e

REGISTRY="ghcr.io/sonichigo"
IMAGE_NAME="bbqbookkeeper"

echo "🔨 Building BAD version (with performance anti-patterns)..."
docker build \
  --build-arg BUILD_VERSION=bad \
  -t "${REGISTRY}/${IMAGE_NAME}:bad" \
  .

echo "🔨 Building GOOD version (optimized)..."
docker build \
  --build-arg BUILD_VERSION=good \
  -t "${REGISTRY}/${IMAGE_NAME}:good" \
  -t "${REGISTRY}/${IMAGE_NAME}:latest" \
  .

echo "📦 Pushing images to registry..."
docker push "${REGISTRY}/${IMAGE_NAME}:bad"
docker push "${REGISTRY}/${IMAGE_NAME}:good"
docker push "${REGISTRY}/${IMAGE_NAME}:latest"

echo "✅ Done! Images pushed:"
echo "  - ${REGISTRY}/${IMAGE_NAME}:bad"
echo "  - ${REGISTRY}/${IMAGE_NAME}:good"
echo "  - ${REGISTRY}/${IMAGE_NAME}:latest"
