#!/bin/bash
# Script to retrieve the admin password for wsprnet-mqtt

set -e

# Check if container is running
if ! docker ps --format '{{.Names}}' | grep -q '^wsprnet-mqtt$'; then
    echo "Error: wsprnet-mqtt container is not running"
    echo "Start it with: ./start.sh"
    echo "Or: docker-compose up -d (or docker compose up -d)"
    exit 1
fi

# Extract password from config file using yq inside the container
PASSWORD=$(docker exec wsprnet-mqtt yq eval '.admin_password' /app/data/config.yaml 2>/dev/null)

if [ -z "$PASSWORD" ] || [ "$PASSWORD" = "null" ]; then
    echo "Error: Could not retrieve admin password"
    echo "The password may not be set in the config file"
    exit 1
fi

echo "=========================================="
echo "WSPR MQTT Aggregator Admin Password"
echo "=========================================="
echo ""
echo "Password: $PASSWORD"
echo ""
echo "Admin URL: http://localhost:9009/admin"
echo ""
echo "=========================================="
