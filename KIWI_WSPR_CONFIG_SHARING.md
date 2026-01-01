# KiwiSDR Config Sharing with wsprnet_mqtt

This document explains how `wsprnet_mqtt` can read the configuration from `kiwi_wspr`.

## Overview

The Docker setup now includes a shared volume (`kiwi-wspr-config`) that allows `wsprnet_mqtt` to access the `kiwi_wspr` configuration file. This enables `wsprnet_mqtt` to:

- Read KiwiSDR instance information
- Access MQTT broker settings
- View enabled WSPR bands
- Understand the decoder configuration

## Docker Volume Setup

### docker-compose.yml

Both services now mount the shared `kiwi-wspr-config` volume:

- **kiwi-wspr**: Mounts at `/app/config` (read-only) and copies its config there on startup
- **wsprnet-mqtt**: Mounts at `/app/kiwi_wspr_config` (read-only) for reading

### Volume Configuration

```yaml
volumes:
  kiwi-wspr-config:
    driver: local
```

## Using the Config in wsprnet_mqtt

The `wsprnet_mqtt` application can now load the kiwi_wspr config using the provided Go code:

```go
import "path/to/wsprnet_mqtt"

// Load kiwi_wspr config
kiwiConfig, err := LoadKiwiWSPRConfig("/app/kiwi_wspr_config/config.yaml")
if err != nil {
    log.Printf("Warning: Could not load kiwi_wspr config: %v", err)
    // Continue with normal operation
}

// Access kiwi_wspr settings
if kiwiConfig != nil {
    // Get MQTT settings
    mqttHost := kiwiConfig.MQTT.Host
    mqttPort := kiwiConfig.MQTT.Port
    topicPrefix := kiwiConfig.MQTT.TopicPrefix
    
    // Get enabled bands
    enabledBands := kiwiConfig.GetEnabledBands()
    for _, band := range enabledBands {
        log.Printf("Band: %s at %.1f kHz", band.Name, band.Frequency)
    }
    
    // Get instance information
    for _, inst := range kiwiConfig.KiwiInstances {
        if inst.Enabled {
            log.Printf("Instance: %s at %s:%d", inst.Name, inst.Host, inst.Port)
        }
    }
}
```

## Available Config Structures

The following structures are available in `wsprnet_mqtt/kiwi_wspr_config.go`:

- `KiwiWSPRConfig` - Main configuration structure
- `KiwiMQTTConfig` - MQTT broker settings
- `KiwiInstance` - KiwiSDR instance details
- `KiwiWSPRBand` - WSPR band configuration
- `KiwiDecoderConfig` - Decoder settings
- `KiwiLoggingConfig` - Logging configuration

## Helper Functions

- `LoadKiwiWSPRConfig(filename string)` - Load config from file
- `GetEnabledBands()` - Get list of enabled WSPR bands
- `GetInstance(name string)` - Get specific KiwiSDR instance by name
- `GetMQTTTopicPrefix(instanceName string)` - Get MQTT topic prefix for an instance

## Config File Location

In Docker containers:
- **kiwi-wspr** writes to: `/app/config/config.yaml`
- **wsprnet-mqtt** reads from: `/app/kiwi_wspr_config/config.yaml`

## Notes

- The config is read-only for both containers to prevent accidental modifications
- The kiwi_wspr container copies its config to the shared volume on startup
- If the config file is not available, wsprnet_mqtt should handle the error gracefully
- The config is automatically updated when kiwi_wspr restarts

## Example Use Cases

1. **Auto-discovery of MQTT settings**: wsprnet_mqtt can read the MQTT broker settings from kiwi_wspr
2. **Band monitoring**: Display which bands are being monitored by kiwi_wspr
3. **Instance tracking**: Show which KiwiSDR instances are configured
4. **Coordination**: Ensure both services use compatible MQTT topic prefixes
