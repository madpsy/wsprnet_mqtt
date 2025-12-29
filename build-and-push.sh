#!/bin/bash
# Build and push Docker images to Docker Hub

set -e

# Docker Hub username
DOCKER_USER="madpsy"

# Image names and tags
KIWI_IMAGE="${DOCKER_USER}/kiwi-wspr"
WSPRNET_IMAGE="${DOCKER_USER}/wsprnet-mqtt"
TAG="${1:-latest}"

echo "=========================================="
echo "Building and Pushing Docker Images"
echo "=========================================="
echo ""
echo "Docker Hub User: ${DOCKER_USER}"
echo "Tag: ${TAG}"
echo ""

# Check if logged in to Docker Hub
if ! docker info | grep -q "Username: ${DOCKER_USER}"; then
    echo "Please login to Docker Hub first:"
    docker login
    echo ""
fi

# Build KiwiSDR WSPR Decoder
echo "Building ${KIWI_IMAGE}:${TAG}..."
docker build -t ${KIWI_IMAGE}:${TAG} -f kiwi_wspr/docker/Dockerfile kiwi_wspr/

# Build WSPR MQTT Aggregator
echo "Building ${WSPRNET_IMAGE}:${TAG}..."
docker build -t ${WSPRNET_IMAGE}:${TAG} -f wsprnet_mqtt/docker/Dockerfile wsprnet_mqtt/

echo ""
echo "=========================================="
echo "Pushing images to Docker Hub..."
echo "=========================================="
echo ""

# Push KiwiSDR WSPR Decoder
echo "Pushing ${KIWI_IMAGE}:${TAG}..."
docker push ${KIWI_IMAGE}:${TAG}

# Push WSPR MQTT Aggregator
echo "Pushing ${WSPRNET_IMAGE}:${TAG}..."
docker push ${WSPRNET_IMAGE}:${TAG}

echo ""
echo "=========================================="
echo "Build and Push Complete!"
echo "=========================================="
echo ""
echo "Images pushed:"
echo "  - ${KIWI_IMAGE}:${TAG}"
echo "  - ${WSPRNET_IMAGE}:${TAG}"
echo ""

# Commit and push changes to git
echo "=========================================="
echo "Updating Git Repository"
echo "=========================================="
echo ""

# Check if we're in a git repository
if git rev-parse --git-dir > /dev/null 2>&1; then
    # Check if there are any changes
    if ! git diff-index --quiet HEAD --; then
        echo "Committing changes to git..."
        
        # Add all changes
        git add -A
        
        # Create commit with timestamp
        COMMIT_MSG="Update Docker images - $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
        git commit -m "$COMMIT_MSG"
        
        echo "✓ Changes committed: $COMMIT_MSG"
        
        # Push to remote
        echo "Pushing to remote repository..."
        if git push; then
            echo "✓ Changes pushed to remote repository"
        else
            echo "⚠️  Warning: Failed to push to remote. You may need to push manually."
        fi
    else
        echo "✓ No changes to commit"
    fi
else
    echo "⚠️  Not a git repository - skipping git operations"
fi

echo ""
echo "=========================================="
echo ""
echo "To use these images, run:"
echo "  docker-compose -f docker-compose-hub.yaml up -d"
echo ""
echo "Or install with one command:"
echo "  curl -fsSL https://raw.githubusercontent.com/madpsy/wsprnet_mqtt/main/install-hub.sh | bash"
echo ""
