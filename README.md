# WSPR MQTT Docker Compose Setup

This Docker Compose configuration runs three services together:
1. **Mosquitto MQTT Broker** - Message broker for WSPR data
2. **KiwiSDR WSPR Decoder** - Decodes WSPR signals from KiwiSDR receivers
3. **WSPR MQTT Aggregator** - Aggregates WSPR spots and submits to WSPRNet

## One-Line Installation

Install everything with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/madpsy/wsprnet_mqtt/main/install-hub.sh | bash
```

This will:
- Install Docker if needed
- Download all configuration files
- Create the sdr-network
- Pull Docker images from Docker Hub
- Start all services
- Display access URLs and admin password

Custom installation directory:
```bash
INSTALL_DIR=/opt/wsprnet_mqtt curl -fsSL https://raw.githubusercontent.com/madpsy/wsprnet_mqtt/main/install-hub.sh | bash
```

## Manual Installation / Quick Start

### Option 1: Using Pre-built Docker Hub Images (Recommended)

```bash
# Quick start with automatic password display
./start.sh

# Or manually
docker-compose -f docker-compose-hub.yaml up -d
```

This uses pre-built images from Docker Hub:
- `madpsy/kiwi-wspr:latest`
- `madpsy/wsprnet-mqtt:latest`

The `start.sh` script will display the web URLs and generated admin password.

### Option 2: Build Locally

```bash
# Quick start
./start.sh local

# Or manually
docker-compose up -d
```

This will:
- Start the Mosquitto MQTT broker on port 1883 (no authentication required)
- Start the KiwiSDR WSPR decoder with web interface on port 8080
- Start the WSPR MQTT aggregator with web interface on port 9009

## Building and Publishing to Docker Hub

If you want to build and push your own images to Docker Hub:

```bash
# Login to Docker Hub (if not already logged in)
docker login

# Build and push images (default tag: latest)
./build-and-push.sh

# Or specify a custom tag
./build-and-push.sh v1.0.0
```

This will build both images and push them to:
- `madpsy/kiwi-wspr:latest` (or your specified tag)
- `madpsy/wsprnet-mqtt:latest` (or your specified tag)

### 2. Configure the Applications

**Note:** Both applications automatically copy example configurations to persistent volumes on first startup. You can then edit these configurations and restart the containers.

#### Configure KiwiSDR WSPR Decoder

The configuration file is automatically created at `/app/data/config.yaml` in the container (persistent volume) on first startup.

To edit the configuration:

```bash
# Option 1: Copy from container, edit, and copy back
docker cp kiwi-wspr:/app/data/config.yaml ./config-kiwi.yaml
nano ./config-kiwi.yaml
docker cp ./config-kiwi.yaml kiwi-wspr:/app/data/config.yaml
docker-compose restart kiwi-wspr

# Option 2: Edit directly in the container
docker exec -it kiwi-wspr nano /app/data/config.yaml
docker-compose restart kiwi-wspr
```

Key settings to configure:
- Set `mqtt.enabled: true`
- Set `mqtt.host: "mosquitto"` (the container name)
- Configure your KiwiSDR instances
- Enable the WSPR bands you want to decode

#### Configure WSPR MQTT Aggregator

The configuration file is automatically created at `/app/data/config.yaml` in the container (persistent volume) on first startup.

To edit the configuration:

```bash
# Option 1: Copy from container, edit, and copy back
docker cp wsprnet-mqtt:/app/data/config.yaml ./config-wsprnet.yaml
nano ./config-wsprnet.yaml
docker cp ./config-wsprnet.yaml wsprnet-mqtt:/app/data/config.yaml
docker-compose restart wsprnet-mqtt

# Option 2: Edit directly in the container
docker exec -it wsprnet-mqtt nano /app/data/config.yaml
docker-compose restart wsprnet-mqtt
```

Key settings to configure:
- Set your receiver callsign and locator
- Set `mqtt.broker: "tcp://mosquitto:1883"` (using the container name)
- Configure MQTT topic prefixes to match your kiwi_wspr configuration
- Set `dry_run: false` when ready to submit to WSPRNet

**Admin Password:** The ADMIN_PASSWORD environment variable in docker-compose.yml sets the admin interface password. If not set, a random password is generated on first startup. To retrieve the current password:
```bash
./get-admin-password.sh
```

Or view it from logs (only shows if just generated):
```bash
docker-compose logs wsprnet-mqtt | grep "Generated random admin password"
```

### 3. Access the Web Interfaces

- **KiwiSDR WSPR Decoder**: http://localhost:8080
- **WSPR MQTT Aggregator**: http://localhost:9009
- **Mosquitto MQTT Broker**: Port 1883 (MQTT), Port 9001 (WebSocket)

## Service Communication

All services are connected via the `sdr-network` Docker network (shared with ka9q_ubersdr). The applications can reference each other by their container names:

- **mosquitto** - MQTT broker
- **kiwi-wspr** - KiwiSDR WSPR decoder
- **wsprnet-mqtt** - WSPR MQTT aggregator
- **ka9q_ubersdr** - UberSDR web interface (if running)
- **ka9q-radio** - KA9Q radio backend (if running)

### Network Setup

The `sdr-network` must be created before starting these services:

```bash
docker network create sdr-network --subnet 172.20.0.0/16
```

This network is shared with the ka9q_ubersdr stack, allowing all containers to communicate with each other by name. The network is marked as `external: true` in the compose files, meaning it must exist before starting the services.

## MQTT Configuration

The Mosquitto broker is configured to:
- Allow anonymous connections (no username/password required)
- Listen on port 1883 for MQTT connections
- Listen on port 9001 for WebSocket connections
- Persist messages to `/mosquitto/data/`
- Log to `/mosquitto/log/mosquitto.log`

## Docker Commands

### View logs
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f mosquitto
docker-compose logs -f kiwi-wspr
docker-compose logs -f wsprnet-mqtt
```

### Stop services
```bash
docker-compose down
```

### Stop and remove volumes (WARNING: deletes all data)
```bash
docker-compose down -v
```

### Rebuild services
```bash
docker-compose build
docker-compose up -d
```

### Check service status
```bash
docker-compose ps
```

## Data Persistence

Data is persisted in Docker volumes:
- `mosquitto-data` - MQTT broker message persistence
- `mosquitto-logs` - MQTT broker logs
- `kiwi-wspr-data` - KiwiSDR decoder configuration and data
- `wsprnet-mqtt-data` - WSPR aggregator configuration and statistics

## Ports

The following ports are exposed to the host:

| Service | Port | Description |
|---------|------|-------------|
| Mosquitto | 1883 | MQTT broker |
| Mosquitto | 9001 | MQTT WebSocket |
| KiwiSDR WSPR | 8080 | Web interface |
| WSPR MQTT | 9009 | Web interface |

## Troubleshooting

### Check if Mosquitto is running
```bash
docker-compose ps mosquitto
```

### Test MQTT connection
```bash
# Subscribe to all topics
docker exec -it mosquitto mosquitto_sub -t '#' -v

# Publish a test message
docker exec -it mosquitto mosquitto_pub -t 'test/topic' -m 'Hello MQTT'
```

### View container logs
```bash
docker-compose logs -f [service-name]
```

### Restart a specific service
```bash
docker-compose restart [service-name]
```

## Configuration Files

- [`docker-compose.yml`](docker-compose.yml) - Docker Compose for local builds
- [`docker-compose-hub.yaml`](docker-compose-hub.yaml) - Docker Compose using Docker Hub images
- [`mosquitto.conf`](mosquitto.conf) - Mosquitto MQTT broker configuration
- [`build-and-push.sh`](build-and-push.sh) - Script to build and push images to Docker Hub
- `kiwi_wspr/docker/Dockerfile` - KiwiSDR WSPR decoder Dockerfile
- `wsprnet_mqtt/docker/Dockerfile` - WSPR MQTT aggregator Dockerfile

## Example MQTT Topic Structure

When configured, the KiwiSDR decoder publishes to topics like:
```
kiwi_wspr/metrics/digital_modes/WSPR/40m
kiwi_wspr/metrics/digital_modes/WSPR/20m
```

The WSPR aggregator subscribes to these topics and aggregates the data before submitting to WSPRNet.
