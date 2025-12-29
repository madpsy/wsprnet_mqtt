#!/bin/bash
# Startup script for WSPR MQTT Docker Compose setup

set -e

echo "=========================================="
echo "WSPR MQTT Docker Compose Setup"
echo "=========================================="
echo ""

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo "Error: docker-compose is not installed"
    echo "Please install Docker Compose first"
    exit 1
fi

# Use 'docker compose' or 'docker-compose' depending on what's available
if docker compose version &> /dev/null; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

# Check for command line argument
MODE="${1:-hub}"

# Determine which compose file to use
if [ -f "docker-compose.yaml" ]; then
    # Installed via install-hub.sh - use docker-compose.yaml (which is the hub version)
    COMPOSE_FILE="docker-compose.yaml"
    if [ "$MODE" = "local" ]; then
        echo "⚠️  Warning: docker-compose.yaml exists (from install-hub.sh)"
        echo "   For local builds, use docker-compose.yml from the repository"
        COMPOSE_FILE="docker-compose.yml"
        BUILD_FLAG="--build"
    else
        echo "Starting services with DOCKER HUB IMAGES..."
        BUILD_FLAG=""
    fi
elif [ "$MODE" = "local" ]; then
    echo "Starting services with LOCAL BUILD..."
    COMPOSE_FILE="docker-compose.yml"
    BUILD_FLAG="--build"
elif [ "$MODE" = "hub" ]; then
    echo "Starting services with DOCKER HUB IMAGES..."
    COMPOSE_FILE="docker-compose-hub.yaml"
    BUILD_FLAG=""
else
    echo "Usage: $0 [hub|local]"
    echo "  hub   - Use pre-built images from Docker Hub (default)"
    echo "  local - Build images locally"
    exit 1
fi

echo ""

# Build and start all services
# Note: Docker Compose will create the sdr-network automatically
$DOCKER_COMPOSE -f $COMPOSE_FILE up -d $BUILD_FLAG

echo ""
echo "Waiting for services to initialize..."
sleep 3

echo ""
echo "=========================================="
echo "Services started successfully!"
echo "=========================================="
echo ""
echo "Web Interfaces:"
echo "  - KiwiSDR WSPR Decoder: http://localhost:8080"
echo "  - WSPR MQTT Aggregator: http://localhost:9009"
echo ""
echo "MQTT Broker:"
echo "  - Host: localhost"
echo "  - Port: 1883 (MQTT)"
echo "  - Port: 9001 (WebSocket)"
echo "  - Authentication: None (anonymous access enabled)"
echo ""
echo "Container Names:"
echo "  - mosquitto (MQTT broker)"
echo "  - kiwi-wspr (KiwiSDR decoder)"
echo "  - wsprnet-mqtt (WSPR aggregator)"
echo ""
echo "Next Steps:"
echo "  1. Configure kiwi-wspr:"
echo "     docker cp kiwi-wspr:/app/config.yaml.example ./config-kiwi.yaml"
echo "     # Edit config-kiwi.yaml, then:"
echo "     docker cp ./config-kiwi.yaml kiwi-wspr:/app/data/config.yaml"
echo "     $DOCKER_COMPOSE -f $COMPOSE_FILE restart kiwi-wspr"
echo ""
echo "  2. Configure wsprnet-mqtt:"
echo "     docker cp wsprnet-mqtt:/app/config.yaml.example ./config-wsprnet.yaml"
echo "     # Edit config-wsprnet.yaml, then:"
echo "     docker cp ./config-wsprnet.yaml wsprnet-mqtt:/app/data/config.yaml"
echo "     $DOCKER_COMPOSE -f $COMPOSE_FILE restart wsprnet-mqtt"
echo ""
echo "View logs:"
echo "  $DOCKER_COMPOSE -f $COMPOSE_FILE logs -f"
echo ""
echo "Stop services:"
echo "  $DOCKER_COMPOSE -f $COMPOSE_FILE down"
echo ""

# Extract and display the admin password
echo "=========================================="
echo "Web Access Information:"
echo "=========================================="
echo ""
echo "KiwiSDR WSPR Decoder:"
echo "  URL: http://localhost:8080"
echo "  Configure KiwiSDR instances and WSPR bands"
echo ""
echo "WSPR MQTT Aggregator:"
echo "  URL: http://localhost:9009"
echo "  Admin URL: http://localhost:9009/admin"
echo ""
ADMIN_PASSWORD=$($DOCKER_COMPOSE -f $COMPOSE_FILE logs wsprnet-mqtt 2>/dev/null | grep -A 1 "Generated random admin password:" | tail -n 1 | sed 's/^wsprnet-mqtt[[:space:]]*|[[:space:]]*//' | xargs)
if [ -n "$ADMIN_PASSWORD" ]; then
    echo "  Admin Password: $ADMIN_PASSWORD"
else
    echo "  Admin Password: (using ADMIN_PASSWORD env var)"
fi
echo ""
echo "=========================================="
