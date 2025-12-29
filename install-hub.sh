#!/bin/bash
# WSPR MQTT Docker Compose Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/madpsy/wsprnet_mqtt/main/install-hub.sh | bash
# Or with custom directory: INSTALL_DIR=/opt/wsprnet_mqtt curl -fsSL ... | bash

set -e

echo "=========================================="
echo "WSPR MQTT Docker Compose Installer"
echo "=========================================="
echo ""

# Install dependencies
echo "Installing system dependencies..."
sudo apt update
sudo apt install -y ntpsec
echo "✓ System dependencies installed"
echo ""

# Install Docker if not already installed
if command -v docker &> /dev/null; then
    echo "✓ Docker is already installed: $(docker --version)"
else
    echo "Installing Docker..."
    curl -sSL https://get.docker.com/ | sh
    echo "✓ Docker installed successfully"
fi

# Add current user to docker group if not already in it
if groups $USER | grep -q '\bdocker\b'; then
    echo "✓ User $USER is already in the docker group"
else
    echo "Adding user $USER to the docker group..."
    sudo usermod -aG docker $USER
    echo "⚠️  User added to docker group. You may need to log out and back in for this to take effect."
    echo "   Alternatively, you can run: newgrp docker"
fi

echo ""

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo "❌ Error: docker-compose is not installed"
    echo "Docker Compose should have been installed with Docker."
    echo "Please install it manually: https://docs.docker.com/compose/install/"
    exit 1
fi

# Use 'docker compose' or 'docker-compose' depending on what's available
if docker compose version &> /dev/null; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

echo "✓ Docker Compose found: $($DOCKER_COMPOSE version --short 2>/dev/null || echo 'installed')"
echo ""

# Determine installation directory (always use ~/wsprnet_mqtt for consistency)
INSTALL_DIR="$HOME/wsprnet_mqtt"

# Check if this is an update
if [ -f "$INSTALL_DIR/docker-compose.yaml" ] || [ -f "$INSTALL_DIR/docker-compose.yml" ]; then
    echo "Existing installation detected at $INSTALL_DIR"
    echo "Updating configuration files..."
    IS_UPDATE=true
else
    echo "Installing to: $INSTALL_DIR"
    IS_UPDATE=false
fi

echo ""

# Create installation directory
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

echo "Downloading configuration files..."
echo ""

# Base URL for raw files
BASE_URL="https://raw.githubusercontent.com/madpsy/wsprnet_mqtt/main"

# Download docker-compose-hub.yaml as docker-compose.yaml (local name)
echo "📥 Downloading docker-compose.yaml..."
curl -fsSL "$BASE_URL/docker-compose-hub.yaml" -o docker-compose.yaml || {
    echo "❌ Failed to download docker-compose.yaml"
    echo "Please check the URL or download manually"
    exit 1
}

# Download mosquitto.conf
echo "📥 Downloading mosquitto.conf..."
curl -fsSL "$BASE_URL/mosquitto.conf" -o mosquitto.conf

# Download start.sh
echo "📥 Downloading start.sh..."
curl -fsSL "$BASE_URL/start.sh" -o start.sh
chmod +x start.sh

# Download get-admin-password.sh
echo "📥 Downloading get-admin-password.sh..."
curl -fsSL "$BASE_URL/get-admin-password.sh" -o get-admin-password.sh
chmod +x get-admin-password.sh

# Download .env.example (don't overwrite .env if it exists)
if [ ! -f ".env" ]; then
    echo "📥 Downloading .env.example..."
    curl -fsSL "$BASE_URL/.env.example" -o .env.example
else
    echo "✓ .env already exists, skipping .env.example download"
fi

# Download README
echo "📥 Downloading README.md..."
curl -fsSL "$BASE_URL/README.md" -o README.md

echo ""
echo "✓ All files downloaded successfully"
echo ""

# Pull Docker images
if [ "$IS_UPDATE" = true ]; then
    echo "Pulling latest Docker images..."
else
    echo "Pulling Docker images from Docker Hub..."
    echo "This may take a few minutes on first install..."
fi
echo ""
$DOCKER_COMPOSE -f docker-compose.yaml pull

echo ""
echo "✓ Docker images pulled successfully"
echo ""

# Start or restart services
if [ "$IS_UPDATE" = true ]; then
    echo "Restarting services with updated configuration..."
    $DOCKER_COMPOSE -f docker-compose.yaml up -d
else
    echo "Starting services..."
    $DOCKER_COMPOSE -f docker-compose.yaml up -d
fi

echo ""
echo "Waiting for services to initialize..."
sleep 5

echo ""
echo "=========================================="
if [ "$IS_UPDATE" = true ]; then
    echo "Update Complete!"
else
    echo "Installation Complete!"
fi
echo "=========================================="
echo ""
echo "Services Status:"
$DOCKER_COMPOSE -f docker-compose.yaml ps
echo ""
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

# Get admin password
echo "Retrieving admin password..."
sleep 2  # Give container time to fully initialize
if [ -x "./get-admin-password.sh" ]; then
    ADMIN_PASSWORD=$(./get-admin-password.sh 2>/dev/null | grep "Password:" | sed 's/Password: //' | xargs)
    if [ -n "$ADMIN_PASSWORD" ]; then
        echo "  Admin Password: $ADMIN_PASSWORD"
        echo ""
        echo "  ⚠️  IMPORTANT: Save this password!"
    else
        echo "  Admin Password: Run ./get-admin-password.sh to retrieve"
    fi
else
    echo "  Admin Password: Run ./get-admin-password.sh to retrieve"
fi

echo ""
echo "MQTT Broker:"
echo "  Host: localhost (or 'mosquitto' from containers)"
echo "  Port: 1883 (MQTT), 9001 (WebSocket)"
echo "  Authentication: None (anonymous access enabled)"
echo ""
echo "=========================================="
echo "Useful Commands:"
echo "=========================================="
echo ""
echo "  View logs:        cd $INSTALL_DIR && $DOCKER_COMPOSE logs -f"
echo "  Stop services:    cd $INSTALL_DIR && $DOCKER_COMPOSE down"
echo "  Restart services: cd $INSTALL_DIR && $DOCKER_COMPOSE restart"
echo "  Get password:     cd $INSTALL_DIR && ./get-admin-password.sh"
echo "  Update install:   curl -fsSL https://raw.githubusercontent.com/madpsy/wsprnet_mqtt/main/install-hub.sh | bash"
echo ""
echo "Configuration:"
echo "  - Both applications auto-initialize configs on first startup"
echo "  - MQTT is pre-configured to use 'mosquitto' container"
echo "  - Edit configs via web interfaces or Docker volumes"
echo "  - See README.md for detailed instructions"
echo ""
echo "Network:"
echo "  - All services on 'sdr-network' (172.20.0.0/16)"
echo "  - Compatible with ka9q_ubersdr stack"
echo "  - Containers can reference each other by name"
echo ""
echo "=========================================="
echo ""
echo "Installation directory: $INSTALL_DIR"
echo ""
