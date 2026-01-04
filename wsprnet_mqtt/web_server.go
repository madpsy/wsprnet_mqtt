package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// WebServer provides HTTP endpoints for statistics
type WebServer struct {
	stats        *StatisticsTracker
	aggregator   *SpotAggregator
	wsprnet      *WSPRNet
	config       *Config
	port         int
	adminHandler *AdminHandler
	configFile   string
	mqttClient   *MQTTClient
	spotWriter   *SpotWriter
}

// NewWebServer creates a new web server
func NewWebServer(stats *StatisticsTracker, aggregator *SpotAggregator, wsprnet *WSPRNet, config *Config, port int, configFile string, mqttClient *MQTTClient, spotWriter *SpotWriter) *WebServer {
	return &WebServer{
		stats:        stats,
		aggregator:   aggregator,
		wsprnet:      wsprnet,
		config:       config,
		port:         port,
		adminHandler: NewAdminHandler(config, configFile),
		configFile:   configFile,
		mqttClient:   mqttClient,
		spotWriter:   spotWriter,
	}
}

// Start starts the web server
func (ws *WebServer) Start() error {
	// API endpoints
	http.HandleFunc("/api/stats", ws.handleStats)
	http.HandleFunc("/api/instances", ws.handleInstances)
	http.HandleFunc("/api/windows", ws.handleWindows)
	http.HandleFunc("/api/aggregator", ws.handleAggregator)
	http.HandleFunc("/api/countries", ws.handleCountries)
	http.HandleFunc("/api/spots", ws.handleSpots)
	http.HandleFunc("/api/wsprnet", ws.handleWSPRNet)
	http.HandleFunc("/api/snr-history", ws.handleSNRHistory)
	http.HandleFunc("/api/receiver", ws.handleReceiver)
	http.HandleFunc("/api/instance-performance", ws.handleInstancePerformance)
	http.HandleFunc("/api/instance-performance-raw", ws.handleInstancePerformanceRaw)
	http.HandleFunc("/api/mqtt/status", ws.handleMQTTStatus)

	// Spot history endpoints
	http.HandleFunc("/api/spots/raw", ws.handleRawSpots)
	http.HandleFunc("/api/spots/deduped", ws.handleDedupedSpots)
	http.HandleFunc("/api/spots/instances", ws.handleSpotInstances)
	http.HandleFunc("/api/spots/gaps", ws.handleSpotGaps)

	// Admin endpoints
	http.HandleFunc("/admin/login", ws.adminHandler.HandleAdminLogin)
	http.HandleFunc("/admin/logout", ws.adminHandler.HandleAdminLogout)
	http.HandleFunc("/admin/dashboard", ws.adminHandler.AuthMiddleware(ws.adminHandler.HandleAdminDashboard))
	http.HandleFunc("/admin/api/config", ws.adminHandler.AuthMiddleware(ws.handleAdminAPI))
	http.HandleFunc("/admin/api/config/export", ws.adminHandler.AuthMiddleware(ws.adminHandler.HandleExportConfig))
	http.HandleFunc("/admin/api/config/import", ws.adminHandler.AuthMiddleware(ws.adminHandler.HandleImportConfig))
	http.HandleFunc("/admin/api/mqtt/test", ws.adminHandler.AuthMiddleware(ws.handleMQTTTest))
	http.HandleFunc("/admin/api/kiwi/sync", ws.adminHandler.AuthMiddleware(ws.adminHandler.HandleSyncKiwis))
	http.HandleFunc("/admin/api/stats/clear", ws.adminHandler.AuthMiddleware(ws.handleClearStats))
	http.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
	})

	// Dashboard
	http.HandleFunc("/", ws.handleDashboard)

	addr := fmt.Sprintf(":%d", ws.port)
	log.Printf("Web server starting on http://localhost%s", addr)
	if ws.adminHandler.IsAdminEnabled() {
		log.Printf("Admin interface enabled at http://localhost%s/admin", addr)
	} else {
		log.Printf("Admin interface disabled (set admin_password in config to enable)")
	}

	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Printf("Web server error: %v", err)
		}
	}()

	return nil
}

// handleStats returns overall statistics
func (ws *WebServer) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats := ws.stats.GetOverallStats()
	_ = json.NewEncoder(w).Encode(stats)
}

// handleInstances returns per-instance statistics
func (ws *WebServer) handleInstances(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	instances := ws.stats.GetInstanceStats()
	_ = json.NewEncoder(w).Encode(instances)
}

// handleWindows returns recent window statistics
func (ws *WebServer) handleWindows(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get last 720 windows (24 hours of history)
	windows := ws.stats.GetRecentWindows(720)
	_ = json.NewEncoder(w).Encode(windows)
}

// handleAggregator returns current aggregator state
func (ws *WebServer) handleAggregator(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	aggStats := ws.aggregator.GetStats()
	_ = json.NewEncoder(w).Encode(aggStats)
}

// handleCountries returns country statistics
func (ws *WebServer) handleCountries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	countries := ws.stats.GetCountryStats()
	_ = json.NewEncoder(w).Encode(countries)
}

// handleSpots returns current spots for mapping
func (ws *WebServer) handleSpots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	spots := ws.stats.GetCurrentSpots()
	_ = json.NewEncoder(w).Encode(spots)
}

// handleWSPRNet returns WSPRNet statistics
func (ws *WebServer) handleWSPRNet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	wsprnetStats := ws.wsprnet.GetStats()
	_ = json.NewEncoder(w).Encode(wsprnetStats)
}

// handleSNRHistory returns SNR history for all bands and instances
func (ws *WebServer) handleSNRHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	snrHistory := ws.stats.GetSNRHistory()
	_ = json.NewEncoder(w).Encode(snrHistory)
}

// handleReceiver returns receiver information from config
func (ws *WebServer) handleReceiver(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	receiverInfo := map[string]interface{}{
		"callsign": ws.config.Receiver.Callsign,
		"locator":  ws.config.Receiver.Locator,
	}
	_ = json.NewEncoder(w).Encode(receiverInfo)
}

// handleInstancePerformance returns instance performance data over time
func (ws *WebServer) handleInstancePerformance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	performance := ws.stats.GetInstancePerformance()
	_ = json.NewEncoder(w).Encode(performance)
}

// handleInstancePerformanceRaw returns raw instance performance data over time (pre-deduplication)
func (ws *WebServer) handleInstancePerformanceRaw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	performance := ws.stats.GetInstancePerformanceRaw()
	_ = json.NewEncoder(w).Encode(performance)
}

// handleMQTTStatus returns the current MQTT connection status
func (ws *WebServer) handleMQTTStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if ws.mqttClient == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"connected": false,
			"error":     "MQTT client not initialized",
		})
		return
	}

	status := ws.mqttClient.GetStatus()
	_ = json.NewEncoder(w).Encode(status)
}

// handleAdminAPI handles admin API requests (GET and POST for config)
func (ws *WebServer) handleAdminAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		ws.adminHandler.HandleGetConfig(w, r)
	} else if r.Method == "POST" {
		ws.adminHandler.HandleUpdateConfig(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleMQTTTest tests the MQTT connection with provided parameters
func (ws *WebServer) handleMQTTTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse MQTT config from request body
	var testConfig struct {
		Broker   string `json:"broker"`
		Username string `json:"username"`
		Password string `json:"password"`
		QoS      int    `json:"qos"`
	}

	if err := json.NewDecoder(r.Body).Decode(&testConfig); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Test MQTT connection
	result := map[string]interface{}{
		"success": false,
		"message": "",
	}

	// Try to create a test MQTT connection
	log.Printf("Testing MQTT connection to %s", testConfig.Broker)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(testConfig.Broker)
	opts.SetClientID(fmt.Sprintf("wsprnet_mqtt_test_%d", time.Now().Unix()))

	if testConfig.Username != "" {
		opts.SetUsername(testConfig.Username)
	}
	if testConfig.Password != "" {
		opts.SetPassword(testConfig.Password)
	}

	// Disable auto-reconnect for testing
	opts.SetAutoReconnect(false)
	opts.SetConnectRetry(false)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)

	// Channel to track connection result
	connectedChan := make(chan bool, 1)

	// Set connection handlers
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Println("MQTT test: Connected to broker")
		connectedChan <- true
	})

	client := mqtt.NewClient(opts)

	// Attempt connection
	log.Printf("MQTT test: Connecting to broker: %s", testConfig.Broker)
	token := client.Connect()

	// Monitor connection in background
	go func() {
		if token.WaitTimeout(5 * time.Second) {
			if token.Error() != nil {
				log.Printf("MQTT test: Connection failed: %v", token.Error())
				connectedChan <- false
			}
			// If no error, the OnConnect handler will send true
		} else {
			log.Printf("MQTT test: Connection timeout")
			connectedChan <- false
		}
	}()

	// Wait for connection result with timeout
	select {
	case connected := <-connectedChan:
		if connected {
			result["success"] = true
			result["message"] = fmt.Sprintf("✓ Successfully connected to MQTT broker at %s", testConfig.Broker)
			log.Printf("MQTT test successful: %s", testConfig.Broker)
			// Disconnect after successful test
			if client.IsConnected() {
				client.Disconnect(250)
			}
		} else {
			result["success"] = false
			result["message"] = fmt.Sprintf("❌ Failed to connect to MQTT broker at %s", testConfig.Broker)
			log.Printf("MQTT test failed: connection failed to %s", testConfig.Broker)
		}
	case <-time.After(6 * time.Second):
		result["success"] = false
		result["message"] = fmt.Sprintf("❌ Connection timeout - could not connect to MQTT broker at %s", testConfig.Broker)
		log.Printf("MQTT test failed: connection timeout to %s", testConfig.Broker)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleClearStats clears all statistics from memory and disk
func (ws *WebServer) handleClearStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("Admin: Clearing all statistics and spot logs")

	// Clear statistics from memory
	ws.stats.ClearAllStatistics()

	// Clear the persistence file by writing an empty/initial state
	if ws.config.PersistenceFile != "" {
		emptyStats := &PersistenceData{
			SavedAt:      time.Now(),
			Windows:      make([]*WindowStats, 0),
			Instances:    make(map[string]*InstanceStats),
			CountryStats: make(map[string]*CountryStatsExport),
			MapSpots:     make(map[string]*SpotLocation),
			SNRHistory:   make(map[string]map[string][]SNRHistoryPoint),
			TotalStats: OverallStats{
				TotalSubmitted:  0,
				TotalDuplicates: 0,
				TotalUnique:     0,
			},
			WSPRNetStats: WSPRNetStats{
				Successful: 0,
				Failed:     0,
				Retries:    0,
			},
		}

		data, err := json.MarshalIndent(emptyStats, "", "  ")
		if err != nil {
			log.Printf("Error marshaling empty stats: %v", err)
			http.Error(w, fmt.Sprintf("Failed to clear statistics file: %v", err), http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile(ws.config.PersistenceFile, data, 0644); err != nil {
			log.Printf("Error writing empty stats file: %v", err)
			http.Error(w, fmt.Sprintf("Failed to clear statistics file: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("Statistics file cleared: %s", ws.config.PersistenceFile)
	}

	// Also reset WSPRNet stats
	ws.wsprnet.ResetStats()

	// Clear all spot logs
	if ws.spotWriter != nil {
		if err := ws.spotWriter.ClearAllSpots(); err != nil {
			log.Printf("Error clearing spot logs: %v", err)
			http.Error(w, fmt.Sprintf("Failed to clear spot logs: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "All statistics and spot logs have been cleared successfully",
	})

	log.Println("Admin: All statistics and spot logs cleared successfully")
}

// handleDashboard serves the HTML dashboard
func (ws *WebServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WSPR MQTT Aggregator Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns@3.0.0/dist/chartjs-adapter-date-fns.bundle.min.js"></script>
    <link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" />
    <script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
    <link rel="stylesheet" href="https://unpkg.com/leaflet.markercluster@1.5.3/dist/MarkerCluster.css" />
    <link rel="stylesheet" href="https://unpkg.com/leaflet.markercluster@1.5.3/dist/MarkerCluster.Default.css" />
    <script src="https://unpkg.com/leaflet.markercluster@1.5.3/dist/leaflet.markercluster.js"></script>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: #0f172a;
            color: #e2e8f0;
            padding: 20px;
        }
        .tabs {
            display: flex;
            gap: 5px;
            margin-bottom: 20px;
            border-bottom: 2px solid #334155;
            overflow-x: auto;
            flex-wrap: wrap;
        }
        .tab {
            padding: 12px 24px;
            background: #1e293b;
            border: 2px solid #334155;
            border-bottom: none;
            border-radius: 8px 8px 0 0;
            cursor: pointer;
            color: #94a3b8;
            font-weight: 600;
            transition: all 0.2s ease;
            white-space: nowrap;
            user-select: none;
        }
        .tab:hover {
            background: #2d3748;
            color: #e2e8f0;
        }
        .tab.active {
            background: #334155;
            color: #60a5fa;
            border-color: #60a5fa;
        }
        .tab-content {
            display: none;
        }
        .tab-content.active {
            display: block;
        }
        .band-nav {
            background: #1e293b;
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 20px;
            border: 1px solid #334155;
            position: sticky;
            top: 0;
            z-index: 100;
        }
        .band-nav-title {
            font-size: 0.9em;
            color: #94a3b8;
            margin-bottom: 10px;
            font-weight: 600;
        }
        .band-nav-buttons {
            display: flex;
            flex-wrap: wrap;
            gap: 8px;
        }
        .band-nav-btn {
            padding: 6px 12px;
            background: #334155;
            color: #e2e8f0;
            border: 1px solid #475569;
            border-radius: 6px;
            cursor: pointer;
            font-size: 0.85em;
            font-weight: 600;
            transition: all 0.2s ease;
            text-decoration: none;
        }
        .band-nav-btn:hover {
            background: #475569;
            border-color: #64748b;
            transform: translateY(-1px);
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 30px;
            border-radius: 12px;
            margin-bottom: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.3);
        }
        h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
        }
        .subtitle {
            opacity: 0.9;
            font-size: 1.1em;
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .stat-card {
            background: #1e293b;
            padding: 25px;
            border-radius: 12px;
            border: 1px solid #334155;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }
        .stat-label {
            color: #94a3b8;
            font-size: 0.9em;
            margin-bottom: 8px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .stat-value {
            font-size: 2.5em;
            font-weight: bold;
            color: #60a5fa;
        }
        .chart-container {
            background: #1e293b;
            padding: 25px;
            border-radius: 12px;
            margin-bottom: 30px;
            border: 1px solid #334155;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }
        .chart-title {
            font-size: 1.5em;
            margin-bottom: 20px;
            color: #f1f5f9;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            background: #1e293b;
            border-radius: 12px;
            overflow: hidden;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }
        th {
            background: #334155;
            padding: 15px;
            text-align: left;
            font-weight: 600;
            color: #f1f5f9;
            text-transform: uppercase;
            font-size: 0.85em;
            letter-spacing: 0.5px;
        }
        td {
            padding: 15px;
            border-top: 1px solid #334155;
        }
        tr:hover {
            background: #2d3748;
        }
        .badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 0.85em;
            font-weight: 600;
        }
        .badge-primary {
            background: #3b82f6;
            color: white;
        }
        .badge-success {
            background: #10b981;
            color: white;
        }
        .badge-warning {
            background: #f59e0b;
            color: white;
        }
        .last-update {
            text-align: center;
            color: #94a3b8;
            margin-top: 20px;
            font-size: 0.9em;
        }
        .grid-2col {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
            margin-bottom: 30px;
        }
        @media (max-width: 768px) {
            .grid-2col {
                grid-template-columns: 1fr;
            }
        }
        .instance-name {
            font-weight: 600;
            color: #60a5fa;
        }
        .progress-bar {
            width: 100%;
            height: 8px;
            background: #334155;
            border-radius: 4px;
            overflow: hidden;
            margin-top: 8px;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #3b82f6, #8b5cf6);
            transition: width 0.3s ease;
        }
        #map {
            height: 600px;
            width: 100%;
            border-radius: 8px;
        }
        .filter-container {
            background: #1e293b;
            padding: 20px;
            border-radius: 12px;
            margin-bottom: 20px;
            border: 1px solid #334155;
        }
        .filter-title {
            font-size: 1.2em;
            margin-bottom: 15px;
            color: #f1f5f9;
            font-weight: 600;
        }
        .filter-buttons {
            display: flex;
            flex-wrap: wrap;
            gap: 10px;
            align-items: center;
        }
        .filter-btn {
            padding: 8px 16px;
            border: 2px solid;
            border-radius: 8px;
            cursor: pointer;
            font-weight: 600;
            font-size: 0.9em;
            transition: all 0.2s ease;
            user-select: none;
        }
        .filter-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(0,0,0,0.3);
        }
        .filter-btn.active {
            opacity: 1;
        }
        .filter-btn.inactive {
            opacity: 0.3;
            filter: grayscale(70%);
        }
        .filter-control {
            margin-left: auto;
            display: flex;
            gap: 10px;
        }
        .control-btn {
            padding: 8px 16px;
            background: #334155;
            color: #e2e8f0;
            border: 2px solid #475569;
            border-radius: 8px;
            cursor: pointer;
            font-weight: 600;
            font-size: 0.9em;
            transition: all 0.2s ease;
        }
        .control-btn:hover {
            background: #475569;
            border-color: #64748b;
        }
        .legend {
            background: rgba(30, 41, 59, 0.95);
            padding: 12px;
            border-radius: 8px;
            border: 2px solid #334155;
            box-shadow: 0 4px 6px rgba(0,0,0,0.3);
            line-height: 20px;
            color: #e2e8f0;
            font-size: 13px;
        }
        .legend h4 {
            margin: 0 0 8px 0;
            font-size: 14px;
            font-weight: 600;
            color: #f1f5f9;
        }
        .legend-item {
            display: flex;
            align-items: center;
            margin: 4px 0;
        }
        .legend-color {
            width: 16px;
            height: 16px;
            border-radius: 50%;
            margin-right: 8px;
            border: 2px solid white;
            box-shadow: 0 0 3px rgba(0,0,0,0.5);
        }
        .marker-cluster-small {
            background-color: rgba(59, 130, 246, 0.6);
        }
        .marker-cluster-small div {
            background-color: rgba(59, 130, 246, 0.8);
        }
        .marker-cluster-medium {
            background-color: rgba(245, 158, 11, 0.6);
        }
        .marker-cluster-medium div {
            background-color: rgba(245, 158, 11, 0.8);
        }
        .marker-cluster-large {
            background-color: rgba(239, 68, 68, 0.6);
        }
        .marker-cluster-large div {
            background-color: rgba(239, 68, 68, 0.8);
        }
        .sortable {
            cursor: pointer;
            user-select: none;
            position: relative;
            padding-right: 20px !important;
        }
        .sortable:hover {
            background: #475569;
        }
        .sortable::after {
            content: '⇅';
            position: absolute;
            right: 8px;
            opacity: 0.3;
        }
        .sortable.asc::after {
            content: '↑';
            opacity: 1;
        }
        .sortable.desc::after {
            content: '↓';
            opacity: 1;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🛰️ WSPR MQTT Aggregator</h1>
        <div class="subtitle">Real-time monitoring and statistics</div>
    </div>

    <div class="tabs">
        <div class="tab active" onclick="switchTab('overview')">📊 Overview</div>
        <div class="tab" onclick="switchTab('instances')">🖥️ Instances</div>
        <div class="tab" onclick="switchTab('perband')">📡 Per Band</div>
        <div class="tab" onclick="switchTab('relationships')">🔗 Relationships</div>
        <div class="tab" onclick="switchTab('value')">💎 Value</div>
        <div class="tab" onclick="switchTab('snr')">📈 SNR</div>
        <div class="tab" onclick="switchTab('countries')">🌍 Countries</div>
        <div class="tab" onclick="switchTab('spots')">📍 Spots</div>
        <div class="tab" onclick="switchTab('gaps')">🔍 Gaps</div>
    </div>

    <!-- Overview Tab -->
    <div id="overview" class="tab-content active">
    <div class="stats-grid">
        <div class="stat-card">
            <div class="stat-label">Spots Sent (24h)</div>
            <div class="stat-value" id="successfulSent" style="color: #10b981;">-</div>
        </div>
        <div class="stat-card">
            <div class="stat-label">Duplicates Removed (24h)</div>
            <div class="stat-value" id="totalDuplicates">-</div>
        </div>
        <div class="stat-card">
            <div class="stat-label">Failed Submissions (24h)</div>
            <div class="stat-value" id="failedSent" style="color: #ef4444;">-</div>
        </div>
        <div class="stat-card">
            <div class="stat-label">Pending Spots</div>
            <div class="stat-value" id="pendingSpots">-</div>
        </div>
    </div>

    <div class="grid-2col">
        <div class="chart-container">
            <div class="chart-title" style="display: flex; justify-content: space-between; align-items: center;">
                <span>Spots Over Time</span>
                <label style="font-size: 0.9em; font-weight: normal; cursor: pointer; user-select: none;">
                    <input type="checkbox" id="spotsSmoothingToggle" checked style="margin-right: 8px; cursor: pointer;">
                    Apply Smoothing (Moving Average)
                </label>
            </div>
            <canvas id="spotsChart"></canvas>
        </div>
        <div class="chart-container">
            <div class="chart-title">Band Distribution</div>
            <canvas id="bandChart"></canvas>
        </div>
    </div>

    <div class="chart-container">
        <div class="chart-title">Live WSPR Spots Map</div>
        <div class="filter-container">
            <div class="filter-title">Band Filters</div>
            <div class="filter-buttons">
                <div id="bandFilters"></div>
                <div class="filter-control">
                    <button class="control-btn" onclick="selectAllBands()">All</button>
                    <button class="control-btn" onclick="deselectAllBands()">None</button>
                </div>
            </div>
        </div>
        <div id="map"></div>
    </div>
    </div>
    <!-- End Overview Tab -->

    <!-- Instances Tab -->
    <div id="instances" class="tab-content">
    <div class="chart-container">
        <div class="chart-title">Instance Performance Comparison</div>
        <canvas id="instanceComparisonChart" style="max-height: 300px;"></canvas>
    </div>

    <div class="chart-container">
        <div class="chart-title">Instance Performance Details</div>
        <table id="instanceTable">
            <thead>
                <tr>
                    <th>Instance</th>
                    <th>Total Spots</th>
                    <th>Unique</th>
                    <th>Best SNR Wins</th>
                    <th>Tied SNR</th>
                    <th>Win Rate</th>
                    <th>Last Report</th>
                </tr>
            </thead>
            <tbody id="instanceTableBody">
            </tbody>
        </table>
    </div>

    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin-bottom: 30px;">
        <div class="chart-container" style="margin-bottom: 0;">
            <div class="chart-title" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px;">
                <span style="font-size: 1em;">Before Deduplication</span>
                <label style="font-size: 0.85em; font-weight: normal; cursor: pointer; user-select: none;">
                    <input type="checkbox" id="instanceRawSmoothingToggle" checked style="margin-right: 8px; cursor: pointer;">
                    Smoothing
                </label>
            </div>
            <canvas id="instancePerformanceRawChart" style="max-height: 300px;"></canvas>
        </div>

        <div class="chart-container" style="margin-bottom: 0;">
            <div class="chart-title" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px;">
                <span style="font-size: 1em;">After Deduplication (Highest SNR)</span>
                <label style="font-size: 0.85em; font-weight: normal; cursor: pointer; user-select: none;">
                    <input type="checkbox" id="instanceSmoothingToggle" checked style="margin-right: 8px; cursor: pointer;">
                    Smoothing
                </label>
            </div>
            <canvas id="instancePerformanceChart" style="max-height: 300px;"></canvas>
        </div>
    </div>
    </div>
    <!-- End Instances Tab -->

    <!-- Per Band Tab -->
    <div id="perband" class="tab-content">
    <div class="band-nav">
        <div class="band-nav-title">Jump to Band:</div>
        <div class="band-nav-buttons" id="perbandBandNav"></div>
    </div>
    <div class="chart-container">
        <div class="chart-title" style="display: flex; justify-content: space-between; align-items: center;">
            <span>Per-Band Instance Performance</span>
            <label style="font-size: 0.9em; font-weight: normal; cursor: pointer; user-select: none;">
                <input type="checkbox" id="bandSmoothingToggle" checked style="margin-right: 8px; cursor: pointer;">
                Apply Smoothing (Moving Average)
            </label>
        </div>
        <div id="bandInstanceTables"></div>
    </div>
    </div>
    <!-- End Per Band Tab -->

    <!-- Relationships Tab -->
    <div id="relationships" class="tab-content">
    <div class="band-nav">
        <div class="band-nav-title">Jump to Band:</div>
        <div class="band-nav-buttons" id="relationshipsBandNav"></div>
    </div>
    <div class="chart-container">
        <div class="chart-title">Instance Relationships by Band</div>
        <div id="relationshipsContainer"></div>
    </div>
    </div>
    <!-- End Relationships Tab -->

    <!-- Value Tab -->
    <div id="value" class="tab-content">
    <div class="band-nav">
        <div class="band-nav-title">Jump to Band:</div>
        <div class="band-nav-buttons" id="valueBandNav"></div>
    </div>
    <div class="chart-container">
        <div class="chart-title">📊 Multi-Instance Value Analysis</div>
        <div id="multiInstanceAnalysis"></div>
    </div>
    </div>
    <!-- End Value Tab -->

    <!-- SNR Tab -->
    <div id="snr" class="tab-content">
    <div class="band-nav">
        <div class="band-nav-title">Jump to Band:</div>
        <div class="band-nav-buttons" id="snrBandNav"></div>
    </div>
    <div class="chart-container">
        <div class="chart-title" style="display: flex; justify-content: space-between; align-items: center;">
            <span>SNR History by Band</span>
            <label style="font-size: 0.9em; font-weight: normal; cursor: pointer; user-select: none;">
                <input type="checkbox" id="snrSmoothingToggle" checked style="margin-right: 8px; cursor: pointer;">
                Apply Smoothing (Moving Average)
            </label>
        </div>
        <div id="snrHistoryCharts"></div>
    </div>
    </div>
    <!-- End SNR Tab -->

    <!-- Countries Tab -->
    <div id="countries" class="tab-content">
    <div class="band-nav">
        <div class="band-nav-title">Jump to Band:</div>
        <div class="band-nav-buttons" id="countriesBandNav"></div>
    </div>
    
    <div class="chart-container">
        <div class="chart-title">📊 Country Statistics Summary</div>
        <div id="countrySummary"></div>
    </div>
    
    <div class="chart-container">
        <div class="chart-title">Country Statistics by Band</div>
        <div id="countryTables"></div>
    </div>
    </div>
    <!-- End Countries Tab -->

    <!-- Spots Tab -->
    <div id="spots" class="tab-content">
    <div class="chart-container">
        <div class="chart-title">📍 Spot History (24 Hours)</div>
        
        <!-- Filters -->
        <div class="filter-container">
            <div class="filter-title">Filters</div>
            <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 15px; margin-bottom: 15px;">
                <div>
                    <label style="display: block; color: #94a3b8; font-size: 0.9em; margin-bottom: 5px;">Source</label>
                    <select id="spotSourceFilter" style="width: 100%; padding: 8px; background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                        <option value="deduped">Deduped Spots (Sent to WSPRNet)</option>
                    </select>
                </div>
                <div>
                    <label style="display: block; color: #94a3b8; font-size: 0.9em; margin-bottom: 5px;">Time Range</label>
                    <select id="spotTimeFilter" style="width: 100%; padding: 8px; background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                        <option value="1">Last 1 Hour</option>
                        <option value="6">Last 6 Hours</option>
                        <option value="12">Last 12 Hours</option>
                        <option value="24" selected>Last 24 Hours</option>
                    </select>
                </div>
                <div>
                    <label style="display: block; color: #94a3b8; font-size: 0.9em; margin-bottom: 5px;">Submission Status</label>
                    <select id="spotSubmittedFilter" style="width: 100%; padding: 8px; background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                        <option value="">All</option>
                        <option value="true">Successfully Sent</option>
                        <option value="false">Failed to Send</option>
                    </select>
                </div>
                <div>
                    <label style="display: block; color: #94a3b8; font-size: 0.9em; margin-bottom: 5px;">Search Callsign</label>
                    <input type="text" id="spotCallsignSearch" placeholder="Filter by callsign..." style="width: 100%; padding: 8px; background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                </div>
                <div>
                    <label style="display: block; color: #94a3b8; font-size: 0.9em; margin-bottom: 5px;">Country</label>
                    <select id="spotCountryFilter" style="width: 100%; padding: 8px; background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                        <option value="">All Countries</option>
                    </select>
                </div>
            </div>
            
            <!-- Band Filter Buttons -->
            <div style="margin-bottom: 15px;">
                <div style="color: #94a3b8; font-size: 0.9em; margin-bottom: 8px; font-weight: 600;">Band Filter:</div>
                <div id="spotBandButtons" style="display: flex; flex-wrap: wrap; gap: 8px;"></div>
            </div>
            
            <div style="display: flex; gap: 10px;">
                <button class="control-btn" onclick="loadSpots()">🔄 Refresh</button>
                <button class="control-btn" onclick="exportSpots()">💾 Export CSV</button>
            </div>
        </div>

        <!-- Stats Summary -->
        <div id="spotsSummary" style="display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 15px; margin-bottom: 20px;">
        </div>

        <!-- Pagination Controls -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px; padding: 10px; background: #1e293b; border-radius: 8px; border: 1px solid #334155;">
            <div style="display: flex; gap: 10px; align-items: center;">
                <span style="color: #94a3b8; font-size: 0.9em;">Results per page:</span>
                <select id="spotsPerPage" style="padding: 6px 10px; background: #0f172a; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                    <option value="50">50</option>
                    <option value="100" selected>100</option>
                    <option value="250">250</option>
                    <option value="500">500</option>
                    <option value="1000">1000</option>
                    <option value="all">All</option>
                </select>
            </div>
            <div id="spotsPagination" style="display: flex; gap: 5px; align-items: center;">
            </div>
        </div>

        <!-- Spots Table -->
        <div style="overflow-x: auto;">
            <table id="spotsTable">
                <thead>
                    <tr>
                        <th class="sortable" data-column="timestamp" data-type="date">Timestamp</th>
                        <th class="sortable" data-column="callsign" data-type="string">Callsign</th>
                        <th class="sortable" data-column="locator" data-type="string">Locator</th>
                        <th class="sortable" data-column="snr" data-type="number">SNR</th>
                        <th class="sortable" data-column="frequency" data-type="number">Frequency</th>
                        <th class="sortable" data-column="band" data-type="string">Band</th>
                        <th class="sortable" data-column="dbm" data-type="number">Power</th>
                        <th class="sortable" data-column="country" data-type="string">Country</th>
                        <th class="sortable" data-column="instance" data-type="string">Instance</th>
                        <th class="sortable" data-column="submitted" data-type="boolean">Status</th>
                    </tr>
                </thead>
                <tbody id="spotsTableBody">
                    <tr><td colspan="10" style="text-align: center; padding: 40px; color: #94a3b8;">Loading spots...</td></tr>
                </tbody>
            </table>
        </div>
        
        <div id="spotsCount" style="text-align: center; color: #94a3b8; margin-top: 15px; font-size: 0.9em;">
            Showing 0 spots
        </div>
    </div>
    </div>
    <!-- End Spots Tab -->

    <!-- Gaps Tab -->
    <div id="gaps" class="tab-content">
    <div class="chart-container">
        <div class="chart-title">🔍 WSPR Cycle Gap Analysis</div>
        
        <div style="background: rgba(59, 130, 246, 0.1); padding: 15px; border-radius: 8px; border: 1px solid rgba(59, 130, 246, 0.3); margin-bottom: 20px;">
            <p style="color: #cbd5e1; margin-bottom: 10px;">
                WSPR transmissions occur every 2 minutes at even minutes (00, 02, 04, etc.). This analysis identifies missing cycles where no spots were received.
            </p>
            <p style="color: #cbd5e1; font-size: 0.9em;">
                <strong>Coverage Rate</strong> = (Cycles with spots / Total expected cycles) × 100%
            </p>
        </div>

        <!-- Time Range Filter -->
        <div class="filter-container">
            <div class="filter-title">Filters</div>
            <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 15px; margin-bottom: 15px;">
                <div>
                    <label style="display: block; color: #94a3b8; font-size: 0.9em; margin-bottom: 5px;">Time Range</label>
                    <select id="gapsTimeFilter" style="width: 100%; padding: 8px; background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                        <option value="1">Last 1 Hour</option>
                        <option value="6">Last 6 Hours</option>
                        <option value="12">Last 12 Hours</option>
                        <option value="24" selected>Last 24 Hours</option>
                    </select>
                </div>
                <div>
                    <label style="display: block; color: #94a3b8; font-size: 0.9em; margin-bottom: 5px;">Instance</label>
                    <select id="gapsInstanceFilter" style="width: 100%; padding: 8px; background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                        <option value="all">All Instances</option>
                        <option value="deduped" selected>📤 Deduped (Sent to WSPRNet)</option>
                    </select>
                </div>
                <div>
                    <label style="display: block; color: #94a3b8; font-size: 0.9em; margin-bottom: 5px;">Gap Type</label>
                    <select id="gapsTypeFilter" style="width: 100%; padding: 8px; background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 6px;">
                        <option value="all">All Gaps</option>
                        <option value="common">Common Gaps (Affecting Multiple Bands/Instances)</option>
                    </select>
                </div>
            </div>
            <div style="display: flex; gap: 10px;">
                <button class="control-btn" onclick="loadGaps()">🔄 Refresh</button>
            </div>
        </div>

        <!-- Band Navigation -->
        <div class="band-nav">
            <div class="band-nav-title">Jump to Band:</div>
            <div class="band-nav-buttons" id="gapsBandNav"></div>
        </div>

        <!-- Gap Summary -->
        <div id="gapsSummary" style="margin-bottom: 20px;"></div>

        <!-- Gap Details by Instance/Band -->
        <div id="gapsDetails"></div>
    </div>
    </div>
    <!-- End Gaps Tab -->

    <div class="last-update">
        Last updated: <span id="lastUpdate">-</span> | Auto-refresh every 120 seconds | <a href="/admin" style="color: #60a5fa; text-decoration: none;">⚙️ Admin</a>
    </div>

    <script>
        // Tab switching function
        function switchTab(tabName) {
            // Hide all tab contents
            const tabContents = document.querySelectorAll('.tab-content');
            tabContents.forEach(content => {
                content.classList.remove('active');
            });
            
            // Remove active class from all tabs
            const tabs = document.querySelectorAll('.tab');
            tabs.forEach(tab => {
                tab.classList.remove('active');
            });
            
            // Show selected tab content
            const selectedContent = document.getElementById(tabName);
            if (selectedContent) {
                selectedContent.classList.add('active');
            }
            
            // Add active class to clicked tab
            const clickedTab = event.target;
            if (clickedTab) {
                clickedTab.classList.add('active');
            }
            
            // Store active tab in localStorage
            localStorage.setItem('activeTab', tabName);
        }
        
        // Restore last active tab on page load
        window.addEventListener('DOMContentLoaded', function() {
            const savedTab = localStorage.getItem('activeTab');
            if (savedTab) {
                // Find and click the saved tab
                const tabs = document.querySelectorAll('.tab');
                tabs.forEach(tab => {
                    if (tab.textContent.toLowerCase().includes(savedTab.toLowerCase()) ||
                        tab.getAttribute('onclick').includes(savedTab)) {
                        // Simulate click to activate the tab
                        const tabContents = document.querySelectorAll('.tab-content');
                        tabContents.forEach(content => content.classList.remove('active'));
                        tabs.forEach(t => t.classList.remove('active'));
                        
                        document.getElementById(savedTab).classList.add('active');
                        tab.classList.add('active');
                    }
                });
            }
        });

        let spotsChart, bandChart, instancePerformanceChart, instancePerformanceRawChart, instanceComparisonChart, map, markerClusterGroup, receiverMarker;
        let allSpots = []; // Store all spots for filtering
        let activeBands = new Set(); // Track which bands are active
        let snrSmoothingEnabled = true; // Track SNR smoothing state (default enabled)
        let bandSmoothingEnabled = true; // Track band performance smoothing state (default enabled)
        let instanceSmoothingEnabled = true; // Track instance performance smoothing state (default enabled)
        let instanceRawSmoothingEnabled = true; // Track raw instance performance smoothing state (default enabled)
        let spotsSmoothingEnabled = true; // Track spots over time smoothing state (default enabled)
        let rawSNRData = {}; // Store raw SNR data for re-rendering
        let rawBandData = {}; // Store raw band instance data for re-rendering
        let rawInstanceData = {}; // Store raw instance performance data for re-rendering
        let rawInstanceRawData = {}; // Store raw instance performance data (pre-dedup) for re-rendering
        let rawWindowsData = []; // Store raw windows data for re-rendering

        // Band colors for map markers (2200m through 10m)
        const bandColors = {
            '2200m': '#7c2d12',
            '630m': '#991b1b',
            '160m': '#dc2626',
            '80m': '#ea580c',
            '60m': '#f59e0b',
            '40m': '#eab308',
            '30m': '#84cc16',
            '20m': '#22c55e',
            '17m': '#10b981',
            '15m': '#14b8a6',
            '12m': '#06b6d4',
            '10m': '#0ea5e9'
        };

        // Initialize band filters
        function initBandFilters() {
            const container = document.getElementById('bandFilters');
            const bands = [
                '2200m', '630m', '160m', '80m', '60m', '40m',
                '30m', '20m', '17m', '15m', '12m', '10m'
            ];
            
            bands.forEach(band => {
                const btn = document.createElement('button');
                btn.className = 'filter-btn active';
                btn.style.borderColor = bandColors[band];
                btn.style.color = bandColors[band];
                btn.textContent = band;
                btn.onclick = () => toggleBand(band);
                btn.dataset.band = band;
                container.appendChild(btn);
                activeBands.add(band);
            });
        }

        // Toggle band filter
        function toggleBand(band) {
            const btn = document.querySelector('[data-band="' + band + '"]');
            if (activeBands.has(band)) {
                activeBands.delete(band);
                btn.classList.remove('active');
                btn.classList.add('inactive');
            } else {
                activeBands.add(band);
                btn.classList.remove('inactive');
                btn.classList.add('active');
            }
            updateMapWithFilters();
        }

        // Select all bands
        function selectAllBands() {
            const bands = [
                '2200m', '630m', '160m', '80m', '60m', '40m',
                '30m', '20m', '17m', '15m', '12m', '10m'
            ];
            bands.forEach(band => {
                activeBands.add(band);
                const btn = document.querySelector('[data-band="' + band + '"]');
                if (btn) {
                    btn.classList.remove('inactive');
                    btn.classList.add('active');
                }
            });
            updateMapWithFilters();
        }

        // Deselect all bands
        function deselectAllBands() {
            const bands = [
                '2200m', '630m', '160m', '80m', '60m', '40m',
                '30m', '20m', '17m', '15m', '12m', '10m'
            ];
            bands.forEach(band => {
                activeBands.delete(band);
                const btn = document.querySelector('[data-band="' + band + '"]');
                if (btn) {
                    btn.classList.remove('active');
                    btn.classList.add('inactive');
                }
            });
            updateMapWithFilters();
        }

        // Update map with current filters
        function updateMapWithFilters() {
            if (!map || !markerClusterGroup) return;
            
            // Clear existing markers
            markerClusterGroup.clearLayers();
            
            if (!allSpots || allSpots.length === 0) return;
            
            // Filter spots based on active bands
            const filteredSpots = allSpots.map(spot => {
                // Filter bands for this spot
                const filteredBands = spot.bands.filter(band => activeBands.has(band));
                if (filteredBands.length === 0) return null;
                
                // Filter SNR values to match filtered bands
                const filteredSNR = spot.bands
                    .map((band, idx) => activeBands.has(band) ? spot.snr[idx] : null)
                    .filter(snr => snr !== null);
                
                return {
                    ...spot,
                    bands: filteredBands,
                    snr: filteredSNR
                };
            }).filter(spot => spot !== null);
            
            // Render filtered spots
            filteredSpots.forEach(spot => {
                const coords = maidenheadToLatLon(spot.locator);
                if (!coords) return;
                
                const icon = createMultiBandIcon(spot.bands);
                const marker = L.marker(coords, { icon: icon });
                
                const bandList = spot.bands.map(b => ` + "`" + `<span style="color: ${bandColors[b]}">${b}</span>` + "`" + `).join(', ');
                const snrList = spot.bands.map((b, i) => ` + "`" + `${b}: ${spot.snr[i]} dB` + "`" + `).join('<br>');
                
                marker.bindPopup(` + "`" + `
                    <strong>${spot.callsign}</strong><br>
                    ${spot.country}<br>
                    Locator: ${spot.locator}<br>
                    Bands: ${bandList}<br>
                    SNR:<br>${snrList}
                ` + "`" + `);
                
                markerClusterGroup.addLayer(marker);
            });
        }

        // Initialize map
        function initMap() {
            map = L.map('map').setView([20, 0], 2);
            L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
                attribution: '© OpenStreetMap contributors',
                maxZoom: 18
            }).addTo(map);
            
            // Initialize marker cluster group
            markerClusterGroup = L.markerClusterGroup({
                maxClusterRadius: 30,
                spiderfyOnMaxZoom: true,
                showCoverageOnHover: false,
                zoomToBoundsOnClick: true,
                disableClusteringAtZoom: 6
            });
            map.addLayer(markerClusterGroup);

            // Add legend
            const legend = L.control({position: 'bottomright'});
            legend.onAdd = function(map) {
                const div = L.DomUtil.create('div', 'legend');
                div.innerHTML = '<h4>WSPR Bands</h4>';
                
                const bands = [
                    '2200m', '630m', '160m', '80m', '60m', '40m',
                    '30m', '20m', '17m', '15m', '12m', '10m'
                ];
                
                bands.forEach(band => {
                    div.innerHTML += ` + "`" + `
                        <div class="legend-item">
                            <div class="legend-color" style="background: ${bandColors[band]}"></div>
                            <span>${band}</span>
                        </div>
                    ` + "`" + `;
                });
                
                return div;
            };
            legend.addTo(map);
        }

        // Helper function to sort bands in proper order
        function sortBands(bands) {
            const bandOrder = ['2200m', '630m', '160m', '80m', '60m', '40m', '30m', '20m', '17m', '15m', '12m', '10m'];
            return bands.sort((a, b) => {
                const aIndex = bandOrder.indexOf(a);
                const bIndex = bandOrder.indexOf(b);
                // If band not in order list, put it at the end
                if (aIndex === -1 && bIndex === -1) return a.localeCompare(b);
                if (aIndex === -1) return 1;
                if (bIndex === -1) return -1;
                return aIndex - bIndex;
            });
        }

        // Convert Maidenhead locator to lat/lon
        function maidenheadToLatLon(locator) {
            if (!locator || locator.length < 4) return null;

            locator = locator.toUpperCase();

            // Field (first 2 chars): 20° longitude, 10° latitude
            const lon1 = (locator.charCodeAt(0) - 65) * 20 - 180;
            const lat1 = (locator.charCodeAt(1) - 65) * 10 - 90;

            // Square (next 2 chars): 2° longitude, 1° latitude
            const lon2 = parseInt(locator[2]) * 2;
            const lat2 = parseInt(locator[3]) * 1;

            let lon = lon1 + lon2;
            let lat = lat1 + lat2;

            // Subsquare (optional 2 chars): 5' (2/24°) longitude, 2.5' (1/24°) latitude
            if (locator.length >= 6) {
                const lon3 = (locator.charCodeAt(4) - 65) * (2/24);
                const lat3 = (locator.charCodeAt(5) - 65) * (1/24);
                lon += lon3;
                lat += lat3;
                // Center of subsquare
                lon += (1/24);
                lat += (1/48);
            } else {
                // Center of square (4-char locator)
                lon += 1;
                lat += 0.5;
            }

            // Add small random offset to spread out multiple stations
            lon += (Math.random() - 0.5) * 0.02;
            lat += (Math.random() - 0.5) * 0.01;

            return [lat, lon];
        }

        // Create multi-colored marker icon
        function createMultiBandIcon(bands) {
            const colors = bands.map(b => bandColors[b] || '#666');
            
            if (colors.length === 1) {
                return L.divIcon({
                    className: 'custom-marker',
                    html: ` + "`" + `<div style="background: ${colors[0]}; width: 12px; height: 12px; border-radius: 50%; border: 2px solid white; box-shadow: 0 0 4px rgba(0,0,0,0.5);"></div>` + "`" + `,
                    iconSize: [16, 16],
                    iconAnchor: [8, 8]
                });
            }
            
            // Multi-band: create gradient or split effect
            let background;
            if (colors.length === 2) {
                // Split in half
                background = ` + "`" + `linear-gradient(90deg, ${colors[0]} 50%, ${colors[1]} 50%)` + "`" + `;
            } else if (colors.length === 3) {
                // Three sections
                background = ` + "`" + `linear-gradient(90deg, ${colors[0]} 33.33%, ${colors[1]} 33.33%, ${colors[1]} 66.66%, ${colors[2]} 66.66%)` + "`" + `;
            } else {
                // More than 3: use conic gradient for pie chart
                const stops = colors.map((color, i) => {
                    const start = (i / colors.length) * 360;
                    const end = ((i + 1) / colors.length) * 360;
                    return ` + "`" + `${color} ${start}deg ${end}deg` + "`" + `;
                }).join(', ');
                background = ` + "`" + `conic-gradient(from 0deg, ${stops})` + "`" + `;
            }
            
            return L.divIcon({
                className: 'custom-marker',
                html: ` + "`" + `<div style="background: ${background}; width: 14px; height: 14px; border-radius: 50%; border: 2px solid white; box-shadow: 0 0 4px rgba(0,0,0,0.5);"></div>` + "`" + `,
                iconSize: [18, 18],
                iconAnchor: [9, 9]
            });
        }

        // Update map with spots (stores spots and applies filters)
        function updateMap(spots) {
            if (!map || !markerClusterGroup) return;
            
            // Store all spots for filtering
            allSpots = spots || [];
            
            // Apply current filters
            updateMapWithFilters();
        }

        // Update receiver marker on map
        function updateReceiverMarker(receiverInfo) {
            if (!map || !receiverInfo || !receiverInfo.locator) return;

            const coords = maidenheadToLatLon(receiverInfo.locator);
            if (!coords) return;

            // Remove existing receiver marker if present
            if (receiverMarker) {
                map.removeLayer(receiverMarker);
            }

            // Create custom icon for receiver
            const receiverIcon = L.divIcon({
                className: 'receiver-marker',
                html: ` + "`" + `<div style="background: radial-gradient(circle, #ef4444 0%, #dc2626 100%); width: 16px; height: 16px; border-radius: 50%; border: 3px solid white; box-shadow: 0 0 10px rgba(239, 68, 68, 0.8);"></div>` + "`" + `,
                iconSize: [22, 22],
                iconAnchor: [11, 11]
            });

            receiverMarker = L.marker(coords, {
                icon: receiverIcon,
                zIndexOffset: 1000
            });

            receiverMarker.bindPopup(` + "`" + `
                <strong>🏠 Receiver Station</strong><br>
                Callsign: ${receiverInfo.callsign}<br>
                Locator: ${receiverInfo.locator}
            ` + "`" + `);

            receiverMarker.addTo(map);
        }

        async function fetchData() {
            try {
                const [stats, instances, windows, aggregator, countries, spots, wsprnet, snrHistory, receiver, instancePerformance, instancePerformanceRaw] = await Promise.all([
                    fetch('/api/stats').then(r => r.json()),
                    fetch('/api/instances').then(r => r.json()),
                    fetch('/api/windows').then(r => r.json()),
                    fetch('/api/aggregator').then(r => r.json()),
                    fetch('/api/countries').then(r => r.json()),
                    fetch('/api/spots').then(r => r.json()),
                    fetch('/api/wsprnet').then(r => r.json()),
                    fetch('/api/snr-history').then(r => r.json()),
                    fetch('/api/receiver').then(r => r.json()),
                    fetch('/api/instance-performance').then(r => r.json()),
                    fetch('/api/instance-performance-raw').then(r => r.json())
                ]);

                updateCharts(windows);
                updateStats(stats, aggregator, wsprnet);
                updateInstanceComparisonChart(instances);
                updateInstanceTable(instances);
                updateInstancePerformanceRawChart(instancePerformanceRaw);
                updateInstancePerformanceChart(instancePerformance);
                updateBandInstanceTable(instances, snrHistory);
                updateRelationships(instances);
                updateMultiInstanceAnalysis(instances);
                updateSNRHistoryCharts(snrHistory);
                updateCountryTables(countries);
                updateMap(spots);
                updateReceiverMarker(receiver);
                
                document.getElementById('lastUpdate').textContent = new Date().toLocaleTimeString();
            } catch (error) {
                console.error('Error fetching data:', error);
            }
        }

        function updateStats(stats, aggregator, wsprnet) {
            // Calculate 24-hour rolling window stats from rawWindowsData
            let rolling24hSent = 0;
            let rolling24hDuplicates = 0;
            let rolling24hFailed = 0;

            if (rawWindowsData && rawWindowsData.length > 0) {
                rawWindowsData.forEach(window => {
                    rolling24hSent += window.TotalSpots || 0;
                    rolling24hDuplicates += window.DuplicateCount || 0;
                    rolling24hFailed += window.FailedCount || 0;
                });
            }

            document.getElementById('successfulSent').textContent = rolling24hSent;
            document.getElementById('failedSent').textContent = rolling24hFailed;
            document.getElementById('totalDuplicates').textContent = rolling24hDuplicates;
            document.getElementById('pendingSpots').textContent = aggregator.pending_spots || 0;
        }

        function updateCharts(windows) {
            if (!windows || windows.length === 0) return;

            // Store raw data for re-rendering when smoothing is toggled
            rawWindowsData = windows;

            // Spots over time chart
            const labels = windows.map(w => {
                const date = new Date(w.WindowTime);
                return date.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
            });
            let spotData = windows.map(w => w.TotalSpots);
            let dupData = windows.map(w => w.DuplicateCount);

            // Apply smoothing if enabled
            if (spotsSmoothingEnabled) {
                spotData = applySmoothingToArray(spotData, 5);
                dupData = applySmoothingToArray(dupData, 5);
            }

            if (spotsChart) {
                spotsChart.data.labels = labels;
                spotsChart.data.datasets[0].data = spotData;
                spotsChart.data.datasets[1].data = dupData;
                spotsChart.update();
            } else {
                const ctx = document.getElementById('spotsChart').getContext('2d');
                spotsChart = new Chart(ctx, {
                    type: 'line',
                    data: {
                        labels: labels,
                        datasets: [{
                            label: 'Total Spots',
                            data: spotData,
                            borderColor: '#3b82f6',
                            backgroundColor: 'rgba(59, 130, 246, 0.1)',
                            borderWidth: 1.5,
                            tension: 0.4,
                            pointRadius: 0,
                            pointHoverRadius: 3
                        }, {
                            label: 'Duplicates',
                            data: dupData,
                            borderColor: '#f59e0b',
                            backgroundColor: 'rgba(245, 158, 11, 0.1)',
                            borderWidth: 1.5,
                            tension: 0.4,
                            pointRadius: 0,
                            pointHoverRadius: 3
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: true,
                        plugins: {
                            legend: {
                                labels: { color: '#e2e8f0' }
                            }
                        },
                        scales: {
                            y: {
                                beginAtZero: true,
                                ticks: { color: '#94a3b8' },
                                grid: { color: '#334155' }
                            },
                            x: {
                                ticks: { color: '#94a3b8' },
                                grid: { color: '#334155' }
                            }
                        }
                    }
                });
            }

            // Band distribution (sum of all windows in last 24 hours)
            if (windows && windows.length > 0) {
                // Aggregate band counts across all windows
                const bandTotals = {};
                windows.forEach(window => {
                    if (window.BandBreakdown) {
                        Object.entries(window.BandBreakdown).forEach(([band, count]) => {
                            bandTotals[band] = (bandTotals[band] || 0) + count;
                        });
                    }
                });

                // Sort bands properly
                const sortedBands = sortBands(Object.keys(bandTotals));
                const counts = sortedBands.map(band => bandTotals[band]);

                if (bandChart) {
                    bandChart.data.labels = sortedBands;
                    bandChart.data.datasets[0].data = counts;
                    bandChart.update();
                } else {
                    const ctx = document.getElementById('bandChart').getContext('2d');
                    bandChart = new Chart(ctx, {
                        type: 'bar',
                        data: {
                            labels: sortedBands,
                            datasets: [{
                                label: 'Spots per Band (24h)',
                                data: counts,
                                backgroundColor: [
                                    '#3b82f6', '#8b5cf6', '#ec4899', '#f59e0b',
                                    '#10b981', '#06b6d4', '#6366f1', '#a855f7',
                                    '#f43f5e', '#14b8a6', '#a855f7', '#22c55e'
                                ]
                            }]
                        },
                        options: {
                            responsive: true,
                            maintainAspectRatio: true,
                            plugins: {
                                legend: { display: false },
                                title: {
                                    display: true,
                                    text: 'Last 24 Hours',
                                    color: '#94a3b8',
                                    font: { size: 12 }
                                }
                            },
                            scales: {
                                y: {
                                    beginAtZero: true,
                                    ticks: { color: '#94a3b8' },
                                    grid: { color: '#334155' }
                                },
                                x: {
                                    ticks: { color: '#94a3b8' },
                                    grid: { color: '#334155' }
                                }
                            }
                        }
                    });
                }
            }
        }

        function updateInstanceComparisonChart(instances) {
            if (!instances || Object.keys(instances).length === 0) return;

            // Sort instances alphabetically by name
            const sortedInstances = Object.values(instances).sort((a, b) =>
                a.Name.localeCompare(b.Name)
            );

            const labels = sortedInstances.map(inst => inst.Name);
            const bestSNRData = sortedInstances.map(inst => inst.BestSNRWins || 0);
            const tiedSNRData = sortedInstances.map(inst => inst.TiedSNR || 0);
            const uniqueData = sortedInstances.map(inst => inst.UniqueSpots || 0);

            if (instanceComparisonChart) {
                instanceComparisonChart.data.labels = labels;
                instanceComparisonChart.data.datasets[0].data = bestSNRData;
                instanceComparisonChart.data.datasets[1].data = tiedSNRData;
                instanceComparisonChart.data.datasets[2].data = uniqueData;
                instanceComparisonChart.update();
            } else {
                const ctx = document.getElementById('instanceComparisonChart').getContext('2d');
                instanceComparisonChart = new Chart(ctx, {
                    type: 'bar',
                    data: {
                        labels: labels,
                        datasets: [{
                            label: 'Best SNR Wins',
                            data: bestSNRData,
                            backgroundColor: '#3b82f6',
                            borderColor: '#2563eb',
                            borderWidth: 1
                        }, {
                            label: 'Tied SNR',
                            data: tiedSNRData,
                            backgroundColor: '#f59e0b',
                            borderColor: '#d97706',
                            borderWidth: 1
                        }, {
                            label: 'Unique Spots',
                            data: uniqueData,
                            backgroundColor: '#10b981',
                            borderColor: '#059669',
                            borderWidth: 1
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: true,
                        plugins: {
                            legend: {
                                labels: { color: '#e2e8f0' },
                                position: 'top'
                            },
                            title: {
                                display: true,
                                text: 'Instance Performance Breakdown (24 hours)',
                                color: '#f1f5f9',
                                font: { size: 16 }
                            },
                            tooltip: {
                                callbacks: {
                                    label: function(context) {
                                        return context.dataset.label + ': ' + context.parsed.y + ' spots';
                                    }
                                }
                            }
                        },
                        scales: {
                            x: {
                                stacked: false,
                                ticks: { color: '#94a3b8' },
                                grid: { color: '#334155' }
                            },
                            y: {
                                stacked: false,
                                beginAtZero: true,
                                ticks: {
                                    color: '#94a3b8',
                                    callback: function(value) {
                                        return value + ' spots';
                                    }
                                },
                                grid: { color: '#334155' },
                                title: {
                                    display: true,
                                    text: 'Spot Count',
                                    color: '#94a3b8'
                                }
                            }
                        }
                    }
                });
            }
        }

        function updateInstanceTable(instances) {
            const tbody = document.getElementById('instanceTableBody');
            tbody.innerHTML = '';

            // Sort instances alphabetically by name
            const sortedInstances = Object.values(instances).sort((a, b) =>
                a.Name.localeCompare(b.Name)
            );

            sortedInstances.forEach(inst => {
                const winRate = inst.TotalSpots > 0
                    ? ((inst.BestSNRWins / inst.TotalSpots) * 100).toFixed(1)
                    : '0.0';
                
                const lastReport = inst.LastReportTime
                    ? new Date(inst.LastReportTime).toLocaleTimeString()
                    : 'Never';

                const row = ` + "`" + `
                    <tr>
                        <td><span class="instance-name">${inst.Name}</span></td>
                        <td>${inst.TotalSpots}</td>
                        <td><span class="badge badge-success">${inst.UniqueSpots}</span></td>
                        <td><span class="badge badge-primary">${inst.BestSNRWins}</span></td>
                        <td><span class="badge badge-warning">${inst.TiedSNR || 0}</span></td>
                        <td>
                            ${winRate}%
                            <div class="progress-bar">
                                <div class="progress-fill" style="width: ${winRate}%"></div>
                            </div>
                        </td>
                        <td>${lastReport}</td>
                    </tr>
                ` + "`" + `;
                tbody.innerHTML += row;
            });
        }

        function updateInstancePerformanceRawChart(performanceData) {
            if (!performanceData || Object.keys(performanceData).length === 0) return;

            // Store raw data for re-rendering when smoothing is toggled
            rawInstanceRawData = performanceData;

            // Sort instances alphabetically
            const instanceNames = Object.keys(performanceData).sort();

            // Generate colors for each instance
            const colors = [
                '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
                '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16',
                '#f97316', '#14b8a6', '#a855f7', '#22c55e'
            ];

            const datasets = instanceNames.map((instance, idx) => {
                const points = performanceData[instance];
                const color = colors[idx % colors.length];

                let dataPoints = points.map(p => ({
                    x: new Date(p.window_time),
                    y: p.spot_count
                }));

                // Apply smoothing if enabled
                if (instanceRawSmoothingEnabled) {
                    dataPoints = applySmoothingToSNR(dataPoints, 5);
                }

                return {
                    label: instance,
                    data: dataPoints,
                    borderColor: color,
                    backgroundColor: color + '20',
                    borderWidth: 1.5,
                    tension: 0.4,
                    pointRadius: 0,
                    pointHoverRadius: 3
                };
            });

            if (instancePerformanceRawChart) {
                instancePerformanceRawChart.data.datasets = datasets;
                instancePerformanceRawChart.update();
            } else {
                const ctx = document.getElementById('instancePerformanceRawChart').getContext('2d');
                instancePerformanceRawChart = new Chart(ctx, {
                    type: 'line',
                    data: {
                        datasets: datasets
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: true,
                        plugins: {
                            legend: {
                                labels: { color: '#e2e8f0' },
                                position: 'top'
                            },
                            title: {
                                display: true,
                                text: 'Raw Spots per Instance Over Time (24 hours)',
                                color: '#f1f5f9',
                                font: { size: 16 }
                            },
                            tooltip: {
                                callbacks: {
                                    label: function(context) {
                                        return context.dataset.label + ': ' + context.parsed.y + ' spots';
                                    }
                                }
                            }
                        },
                        scales: {
                            x: {
                                type: 'time',
                                time: {
                                    unit: 'hour',
                                    displayFormats: {
                                        hour: 'HH:mm'
                                    }
                                },
                                ticks: { color: '#94a3b8' },
                                grid: { color: '#334155' },
                                title: {
                                    display: true,
                                    text: 'Time (UTC)',
                                    color: '#94a3b8'
                                }
                            },
                            y: {
                                beginAtZero: true,
                                ticks: {
                                    color: '#94a3b8',
                                    callback: function(value) {
                                        return value + ' spots';
                                    }
                                },
                                grid: { color: '#334155' },
                                title: {
                                    display: true,
                                    text: 'Spot Count',
                                    color: '#94a3b8'
                                }
                            }
                        }
                    }
                });
            }
        }

        function updateInstancePerformanceChart(performanceData) {
            if (!performanceData || Object.keys(performanceData).length === 0) return;

            // Store raw data for re-rendering when smoothing is toggled
            rawInstanceData = performanceData;

            // Sort instances alphabetically
            const instanceNames = Object.keys(performanceData).sort();

            // Generate colors for each instance
            const colors = [
                '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
                '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16',
                '#f97316', '#14b8a6', '#a855f7', '#22c55e'
            ];

            const datasets = instanceNames.map((instance, idx) => {
                const points = performanceData[instance];
                const color = colors[idx % colors.length];

                let dataPoints = points.map(p => ({
                    x: new Date(p.window_time),
                    y: p.spot_count
                }));

                // Apply smoothing if enabled
                if (instanceSmoothingEnabled) {
                    dataPoints = applySmoothingToSNR(dataPoints, 5);
                }

                return {
                    label: instance,
                    data: dataPoints,
                    borderColor: color,
                    backgroundColor: color + '20',
                    borderWidth: 1.5,
                    tension: 0.4,
                    pointRadius: 0,
                    pointHoverRadius: 3
                };
            });

            if (instancePerformanceChart) {
                instancePerformanceChart.data.datasets = datasets;
                instancePerformanceChart.update();
            } else {
                const ctx = document.getElementById('instancePerformanceChart').getContext('2d');
                instancePerformanceChart = new Chart(ctx, {
                    type: 'line',
                    data: {
                        datasets: datasets
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: true,
                        plugins: {
                            legend: {
                                labels: { color: '#e2e8f0' },
                                position: 'top'
                            },
                            title: {
                                display: true,
                                text: 'Spots per Instance Over Time (24 hours)',
                                color: '#f1f5f9',
                                font: { size: 16 }
                            },
                            tooltip: {
                                callbacks: {
                                    label: function(context) {
                                        return context.dataset.label + ': ' + context.parsed.y + ' spots';
                                    }
                                }
                            }
                        },
                        scales: {
                            x: {
                                type: 'time',
                                time: {
                                    unit: 'hour',
                                    displayFormats: {
                                        hour: 'HH:mm'
                                    }
                                },
                                ticks: { color: '#94a3b8' },
                                grid: { color: '#334155' },
                                title: {
                                    display: true,
                                    text: 'Time (UTC)',
                                    color: '#94a3b8'
                                }
                            },
                            y: {
                                beginAtZero: true,
                                ticks: {
                                    color: '#94a3b8',
                                    callback: function(value) {
                                        return value + ' spots';
                                    }
                                },
                                grid: { color: '#334155' },
                                title: {
                                    display: true,
                                    text: 'Spot Count',
                                    color: '#94a3b8'
                                }
                            }
                        }
                    }
                });
            }
        }

        // Store band charts globally
        const bandCharts = {};

        // Function to create band navigation buttons
        function createBandNavigation(bands, containerId, sectionPrefix) {
            const container = document.getElementById(containerId);
            if (!container) return;
            
            container.innerHTML = '';
            bands.forEach(band => {
                const btn = document.createElement('button');
                btn.className = 'band-nav-btn';
                btn.textContent = band;
                btn.style.borderColor = bandColors[band] || '#475569';
                btn.style.color = bandColors[band] || '#e2e8f0';
                btn.onclick = () => {
                    // Create the ID for this band section
                    const bandId = sectionPrefix + band.replace(/[^a-zA-Z0-9]/g, '_');
                    const targetElement = document.getElementById(bandId);
                    
                    if (targetElement) {
                        targetElement.scrollIntoView({ behavior: 'smooth', block: 'start' });
                        // Add offset for sticky nav
                        setTimeout(() => window.scrollBy(0, -100), 100);
                    }
                };
                container.appendChild(btn);
            });
        }

        function updateBandInstanceTable(instances, snrHistory) {
            // Store raw data for re-rendering when smoothing is toggled
            rawBandData = { instances, snrHistory };
            const container = document.getElementById('bandInstanceTables');
            container.innerHTML = '';

            // Create a grid container for the band sections (2 columns max)
            const gridContainer = document.createElement('div');
            gridContainer.style.display = 'grid';
            gridContainer.style.gridTemplateColumns = 'repeat(2, 1fr)';
            gridContainer.style.gap = '20px';

            // Collect all bands and organize data by band
            const bandData = {};
            
            // Sort instances alphabetically by name
            const sortedInstances = Object.values(instances).sort((a, b) =>
                a.Name.localeCompare(b.Name)
            );
            
            sortedInstances.forEach(inst => {
                Object.entries(inst.BandStats || {}).forEach(([band, stats]) => {
                    if (!bandData[band]) {
                        bandData[band] = [];
                    }
                    bandData[band].push({
                        name: inst.Name,
                        stats: stats
                    });
                });
            });

            // Sort bands properly
            const bands = sortBands(Object.keys(bandData));

            // Create a chart and table for each band
            bands.forEach(band => {
                const instanceList = bandData[band];
                
                // Sort instances alphabetically by name for consistent ordering
                instanceList.sort((a, b) => a.name.localeCompare(b.name));

                const chartId = ` + "`" + `bandChart_${band.replace(/[^a-zA-Z0-9]/g, '_')}` + "`" + `;
                const timeChartId = ` + "`" + `bandTimeChart_${band.replace(/[^a-zA-Z0-9]/g, '_')}` + "`" + `;
                const distanceChartId = ` + "`" + `bandDistanceChart_${band.replace(/[^a-zA-Z0-9]/g, '_')}` + "`" + `;

                const bandId = 'band_' + band.replace(/[^a-zA-Z0-9]/g, '_');
                const sectionHTML = ` + "`" + `
                    <div id="${bandId}" style="margin-bottom: 30px;">
                        <h3 style="color: #60a5fa; margin-bottom: 15px;">
                            <span class="badge badge-warning" style="font-size: 1.1em; padding: 6px 14px;">${band}</span>
                        </h3>
                        
                        <!-- Time Series Chart -->
                        <div style="background: #1e293b; padding: 20px; border-radius: 12px; margin-bottom: 15px; border: 1px solid #334155;">
                            <canvas id="${timeChartId}" style="max-height: 300px;"></canvas>
                        </div>

                        <!-- Distance Chart -->
                        <div style="background: #1e293b; padding: 20px; border-radius: 12px; margin-bottom: 15px; border: 1px solid #334155;">
                            <canvas id="${distanceChartId}" style="max-height: 300px;"></canvas>
                        </div>

                        <!-- Bar Chart -->
                        <div style="background: #1e293b; padding: 20px; border-radius: 12px; margin-bottom: 15px; border: 1px solid #334155;">
                            <canvas id="${chartId}" style="max-height: 250px;"></canvas>
                        </div>

                        <!-- Table -->
                        <table style="width: 100%;">
                            <thead>
                                <tr>
                                    <th>Instance</th>
                                    <th>Total Spots</th>
                                    <th>Unique</th>
                                    <th>Best SNR Wins</th>
                                    <th>Tied SNR</th>
                                    <th>Win Rate</th>
                                    <th>Avg SNR</th>
                                    <th>Min Dist</th>
                                    <th>Max Dist</th>
                                    <th>Avg Dist</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${instanceList.map(item => {
                                    const winRate = item.stats.TotalSpots > 0
                                        ? ((item.stats.BestSNRWins / item.stats.TotalSpots) * 100).toFixed(1)
                                        : '0.0';
                                    const minDist = item.stats.DistanceCount > 0 ? item.stats.MinDistance.toFixed(0) + ' km' : '-';
                                    const maxDist = item.stats.DistanceCount > 0 ? item.stats.MaxDistance.toFixed(0) + ' km' : '-';
                                    const avgDist = item.stats.DistanceCount > 0 ? item.stats.AverageDistance.toFixed(0) + ' km' : '-';
                                    return ` + "`" + `
                                        <tr>
                                            <td><span class="instance-name">${item.name}</span></td>
                                            <td>${item.stats.TotalSpots}</td>
                                            <td><span class="badge badge-success">${item.stats.UniqueSpots}</span></td>
                                            <td><span class="badge badge-primary">${item.stats.BestSNRWins}</span></td>
                                            <td><span class="badge badge-warning">${item.stats.TiedSNR || 0}</span></td>
                                            <td>
                                                ${winRate}%
                                                <div class="progress-bar">
                                                    <div class="progress-fill" style="width: ${winRate}%"></div>
                                                </div>
                                            </td>
                                            <td>${item.stats.AverageSNR.toFixed(1)} dB</td>
                                            <td>${minDist}</td>
                                            <td>${maxDist}</td>
                                            <td>${avgDist}</td>
                                        </tr>
                                    ` + "`" + `;
                                }).join('')}
                            </tbody>
                        </table>
                    </div>
                ` + "`" + `;

                const bandDiv = document.createElement('div');
                bandDiv.innerHTML = sectionHTML;
                gridContainer.appendChild(bandDiv);

                // Create charts after DOM is updated
                setTimeout(() => {
                    // Create time series chart
                    const timeCtx = document.getElementById(timeChartId);
                    if (timeCtx && snrHistory && snrHistory[band]) {
                        const bandHistory = snrHistory[band];
                        const instanceNames = Object.keys(bandHistory.instances || {}).sort();

                        const colors = [
                            '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
                            '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16',
                            '#f97316', '#14b8a6', '#a855f7', '#22c55e'
                        ];

                        const timeDatasets = instanceNames.map((instance, idx) => {
                            const points = bandHistory.instances[instance];
                            const color = colors[idx % colors.length];

                            let dataPoints = points.map(p => ({
                                x: new Date(p.window_time),
                                y: p.spot_count
                            }));

                            // Apply smoothing if enabled
                            if (bandSmoothingEnabled) {
                                dataPoints = applySmoothingToSNR(dataPoints, 5);
                            }

                            return {
                                label: instance,
                                data: dataPoints,
                                borderColor: color,
                                backgroundColor: color + '20',
                                borderWidth: 1.5,
                                tension: 0.4,
                                pointRadius: 0,
                                pointHoverRadius: 3
                            };
                        });

                        const timeChartKey = 'time_' + band;
                        if (bandCharts[timeChartKey]) {
                            bandCharts[timeChartKey].destroy();
                        }

                        bandCharts[timeChartKey] = new Chart(timeCtx, {
                            type: 'line',
                            data: {
                                datasets: timeDatasets
                            },
                            options: {
                                responsive: true,
                                maintainAspectRatio: true,
                                plugins: {
                                    legend: {
                                        labels: { color: '#e2e8f0' },
                                        position: 'top'
                                    },
                                    title: {
                                        display: true,
                                        text: ` + "`" + `${band} - Spots per Instance Over Time` + "`" + `,
                                        color: '#f1f5f9',
                                        font: { size: 16 }
                                    },
                                    tooltip: {
                                        callbacks: {
                                            label: function(context) {
                                                return context.dataset.label + ': ' + context.parsed.y + ' spots';
                                            }
                                        }
                                    }
                                },
                                scales: {
                                    x: {
                                        type: 'time',
                                        time: {
                                            unit: 'hour',
                                            displayFormats: {
                                                hour: 'HH:mm'
                                            }
                                        },
                                        ticks: { color: '#94a3b8' },
                                        grid: { color: '#334155' },
                                        title: {
                                            display: true,
                                            text: 'Time (UTC)',
                                            color: '#94a3b8'
                                        }
                                    },
                                    y: {
                                        beginAtZero: true,
                                        ticks: {
                                            color: '#94a3b8',
                                            callback: function(value) {
                                                return value + ' spots';
                                            }
                                        },
                                        grid: { color: '#334155' },
                                        title: {
                                            display: true,
                                            text: 'Spot Count',
                                            color: '#94a3b8'
                                        }
                                    }
                                }
                            }
                        });
                    }

                    // Create distance chart
                    const distanceCtx = document.getElementById(distanceChartId);
                    if (distanceCtx && snrHistory && snrHistory[band]) {
                        const bandHistory = snrHistory[band];
                        const instanceNames = Object.keys(bandHistory.instances || {}).sort();

                        const colors = [
                            '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
                            '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16',
                            '#f97316', '#14b8a6', '#a855f7', '#22c55e'
                        ];

                        const distanceDatasets = instanceNames.map((instance, idx) => {
                            const points = bandHistory.instances[instance];
                            const color = colors[idx % colors.length];

                            // Filter to only points with distance data
                            const dataPoints = points
                                .filter(p => p.distance_count > 0)
                                .map(p => ({
                                    x: new Date(p.window_time),
                                    y: p.average_distance
                                }));

                            return {
                                label: instance,
                                data: dataPoints,
                                borderColor: color,
                                backgroundColor: color + '20',
                                borderWidth: 1.5,
                                tension: 0.4,
                                pointRadius: 0,
                                pointHoverRadius: 3
                            };
                        });

                        const distanceChartKey = 'distance_' + band;
                        if (bandCharts[distanceChartKey]) {
                            bandCharts[distanceChartKey].destroy();
                        }

                        bandCharts[distanceChartKey] = new Chart(distanceCtx, {
                            type: 'line',
                            data: {
                                datasets: distanceDatasets
                            },
                            options: {
                                responsive: true,
                                maintainAspectRatio: true,
                                plugins: {
                                    legend: {
                                        labels: { color: '#e2e8f0' },
                                        position: 'top'
                                    },
                                    title: {
                                        display: true,
                                        text: ` + "`" + `${band} - Average Distance per Instance Over Time` + "`" + `,
                                        color: '#f1f5f9',
                                        font: { size: 16 }
                                    },
                                    tooltip: {
                                        callbacks: {
                                            label: function(context) {
                                                return context.dataset.label + ': ' + context.parsed.y.toFixed(0) + ' km';
                                            }
                                        }
                                    }
                                },
                                scales: {
                                    x: {
                                        type: 'time',
                                        time: {
                                            unit: 'hour',
                                            displayFormats: {
                                                hour: 'HH:mm'
                                            }
                                        },
                                        ticks: { color: '#94a3b8' },
                                        grid: { color: '#334155' },
                                        title: {
                                            display: true,
                                            text: 'Time (UTC)',
                                            color: '#94a3b8'
                                        }
                                    },
                                    y: {
                                        beginAtZero: true,
                                        ticks: {
                                            color: '#94a3b8',
                                            callback: function(value) {
                                                return value + ' km';
                                            }
                                        },
                                        grid: { color: '#334155' },
                                        title: {
                                            display: true,
                                            text: 'Distance (km)',
                                            color: '#94a3b8'
                                        }
                                    }
                                }
                            }
                        });
                    }

                    // Create bar chart
                    const ctx = document.getElementById(chartId);
                    if (!ctx) return;

                    const labels = instanceList.map(item => item.name);
                    const totalData = instanceList.map(item => item.stats.TotalSpots);
                    const bestSNRData = instanceList.map(item => item.stats.BestSNRWins || 0);
                    const tiedSNRData = instanceList.map(item => item.stats.TiedSNR || 0);
                    const uniqueData = instanceList.map(item => item.stats.UniqueSpots);

                    // Destroy existing chart if it exists
                    if (bandCharts[band]) {
                        bandCharts[band].destroy();
                    }

                    bandCharts[band] = new Chart(ctx, {
                        type: 'bar',
                        data: {
                            labels: labels,
                            datasets: [{
                                label: 'Total Spots',
                                data: totalData,
                                backgroundColor: '#3b82f6',
                                borderColor: '#2563eb',
                                borderWidth: 1
                            }, {
                                label: 'Best SNR Wins',
                                data: bestSNRData,
                                backgroundColor: '#8b5cf6',
                                borderColor: '#7c3aed',
                                borderWidth: 1
                            }, {
                                label: 'Tied SNR',
                                data: tiedSNRData,
                                backgroundColor: '#f59e0b',
                                borderColor: '#d97706',
                                borderWidth: 1
                            }, {
                                label: 'Unique Spots',
                                data: uniqueData,
                                backgroundColor: '#10b981',
                                borderColor: '#059669',
                                borderWidth: 1
                            }]
                        },
                        options: {
                            responsive: true,
                            maintainAspectRatio: true,
                            plugins: {
                                legend: {
                                    labels: { color: '#e2e8f0' }
                                },
                                title: {
                                    display: true,
                                    text: ` + "`" + `${band} - Instance Performance` + "`" + `,
                                    color: '#f1f5f9',
                                    font: { size: 16 }
                                }
                            },
                            scales: {
                                y: {
                                    beginAtZero: true,
                                    ticks: { color: '#94a3b8' },
                                    grid: { color: '#334155' }
                                },
                                x: {
                                    ticks: { color: '#94a3b8' },
                                    grid: { color: '#334155' }
                                }
                            }
                        }
                    });
                }, 0);
            });

            if (bands.length === 0) {
                container.innerHTML = '<p style="color: #94a3b8; text-align: center;">No band data available yet</p>';
            } else {
                container.appendChild(gridContainer);
            }
            
            // Create band navigation for Per Band tab
            if (bands.length > 0) {
                createBandNavigation(bands, 'perbandBandNav', 'band_');
            }
        }

        // Store country data for sorting
        let countryData = {};

        function updateCountrySummary(countries) {
            const summaryContainer = document.getElementById('countrySummary');
            if (!summaryContainer) return;
            
            let totalCallsigns = 0;
            let totalSpots = 0;
            const uniqueCountries = new Set();
            let highestSNR = { value: -999, country: '', band: '' };
            let lowestSNR = { value: 999, country: '', band: '' };
            let mostCommonCountry = { country: '', callsigns: 0, band: '' };
            let leastCommonCountry = { country: '', callsigns: 999999, band: '' };
            const bandCountryCounts = {};
            
            // Analyze all bands
            Object.entries(countries).forEach(([band, countryList]) => {
                bandCountryCounts[band] = countryList.length;
                
                countryList.forEach(c => {
                    uniqueCountries.add(c.country);
                    totalCallsigns += c.unique_callsigns;
                    totalSpots += c.total_spots;
                    
                    if (c.max_snr > highestSNR.value) {
                        highestSNR = { value: c.max_snr, country: c.country, band: band };
                    }
                    
                    if (c.min_snr < lowestSNR.value) {
                        lowestSNR = { value: c.min_snr, country: c.country, band: band };
                    }
                    
                    if (c.unique_callsigns > mostCommonCountry.callsigns) {
                        mostCommonCountry = { country: c.country, callsigns: c.unique_callsigns, band: band };
                    }
                    
                    if (c.unique_callsigns < leastCommonCountry.callsigns) {
                        leastCommonCountry = { country: c.country, callsigns: c.unique_callsigns, band: band };
                    }
                });
            });
            
            const sortedBands = Object.entries(bandCountryCounts).sort((a, b) => b[1] - a[1]);
            const mostCountriesBand = sortedBands[0];
            const fewestCountriesBand = sortedBands[sortedBands.length - 1];
            
            summaryContainer.innerHTML = ` + "`" + `
                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 20px; margin-bottom: 20px;">
                    <div style="background: #1e293b; padding: 20px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 8px; text-transform: uppercase;">Most Active Country</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #10b981; margin-bottom: 5px;">${mostCommonCountry.country}</div>
                        <div style="color: #64748b; font-size: 0.9em;">${mostCommonCountry.callsigns} unique callsigns on ${mostCommonCountry.band}</div>
                    </div>
                    
                    <div style="background: #1e293b; padding: 20px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 8px; text-transform: uppercase;">Least Active Country</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #f59e0b; margin-bottom: 5px;">${leastCommonCountry.country}</div>
                        <div style="color: #64748b; font-size: 0.9em;">${leastCommonCountry.callsigns} unique callsign${leastCommonCountry.callsigns !== 1 ? 's' : ''} on ${leastCommonCountry.band}</div>
                    </div>
                    
                    <div style="background: #1e293b; padding: 20px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 8px; text-transform: uppercase;">Highest SNR</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #3b82f6; margin-bottom: 5px;">${highestSNR.value} dB</div>
                        <div style="color: #64748b; font-size: 0.9em;">${highestSNR.country} on ${highestSNR.band}</div>
                    </div>
                    
                    <div style="background: #1e293b; padding: 20px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 8px; text-transform: uppercase;">Lowest SNR</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #ef4444; margin-bottom: 5px;">${lowestSNR.value} dB</div>
                        <div style="color: #64748b; font-size: 0.9em;">${lowestSNR.country} on ${lowestSNR.band}</div>
                    </div>
                    
                    <div style="background: #1e293b; padding: 20px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 8px; text-transform: uppercase;">Most Diverse Band</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #8b5cf6; margin-bottom: 5px;">${mostCountriesBand[0]}</div>
                        <div style="color: #64748b; font-size: 0.9em;">${mostCountriesBand[1]} countries heard</div>
                    </div>
                    
                    <div style="background: #1e293b; padding: 20px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 8px; text-transform: uppercase;">Least Diverse Band</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #ec4899; margin-bottom: 5px;">${fewestCountriesBand[0]}</div>
                        <div style="color: #64748b; font-size: 0.9em;">${fewestCountriesBand[1]} countries heard</div>
                    </div>
                </div>
                
                <div style="background: rgba(59, 130, 246, 0.1); padding: 15px; border-radius: 8px; border: 1px solid rgba(59, 130, 246, 0.3);">
                    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 15px; text-align: center;">
                        <div>
                            <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Active Bands</div>
                            <div style="font-size: 1.8em; font-weight: bold; color: #60a5fa;">${Object.keys(countries).length}</div>
                            <div style="color: #64748b; font-size: 0.85em;">with activity</div>
                        </div>
                        <div>
                            <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Total Countries</div>
                            <div style="font-size: 1.8em; font-weight: bold; color: #60a5fa;">${uniqueCountries.size}</div>
                            <div style="color: #64748b; font-size: 0.85em;">unique countries</div>
                        </div>
                        <div>
                            <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Total Callsigns</div>
                            <div style="font-size: 1.8em; font-weight: bold; color: #60a5fa;">${totalCallsigns}</div>
                            <div style="color: #64748b; font-size: 0.85em;">unique stations</div>
                        </div>
                        <div>
                            <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Total Spots</div>
                            <div style="font-size: 1.8em; font-weight: bold; color: #60a5fa;">${totalSpots}</div>
                            <div style="color: #64748b; font-size: 0.85em;">across all bands</div>
                        </div>
                    </div>
                </div>
            ` + "`" + `;
        }

        function updateCountryTables(countries) {
            const container = document.getElementById('countryTables');
            container.innerHTML = '';

            if (!countries || Object.keys(countries).length === 0) {
                container.innerHTML = '<p style="color: #94a3b8; text-align: center;">No country data available yet</p>';
                document.getElementById('countrySummary').innerHTML = '<p style="color: #94a3b8; text-align: center;">No country data available yet</p>';
                return;
            }

            // Store data globally for sorting
            countryData = countries;
            
            // Generate summary statistics
            updateCountrySummary(countries);

            // Sort bands properly
            const bands = sortBands(Object.keys(countries));

            bands.forEach(band => {
                const countryList = countries[band];
                if (!countryList || countryList.length === 0) return;

                // Sort by total spots descending (default)
                countryList.sort((a, b) => b.total_spots - a.total_spots);

                // Calculate unique countries and total callsigns for this band
                const uniqueCountriesInBand = countryList.length;
                const totalCallsignsInBand = countryList.reduce((sum, c) => sum + c.unique_callsigns, 0);

                const tableId = ` + "`" + `countryTable_${band.replace(/[^a-zA-Z0-9]/g, '_')}` + "`" + `;

                const bandId = 'country_' + band.replace(/[^a-zA-Z0-9]/g, '_');
                const tableHTML = ` + "`" + `
                    <div id="${bandId}" style="margin-bottom: 30px;">
                        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px;">
                            <h3 style="color: #60a5fa; margin: 0;">${band}</h3>
                            <div style="display: flex; gap: 20px; font-size: 0.9em;">
                                <div style="text-align: right;">
                                    <div style="color: #94a3b8; font-size: 0.85em;">Countries</div>
                                    <div style="color: #10b981; font-weight: bold; font-size: 1.2em;">${uniqueCountriesInBand}</div>
                                </div>
                                <div style="text-align: right;">
                                    <div style="color: #94a3b8; font-size: 0.85em;">Callsigns</div>
                                    <div style="color: #60a5fa; font-weight: bold; font-size: 1.2em;">${totalCallsignsInBand}</div>
                                </div>
                            </div>
                        </div>
                        <table id="${tableId}" style="width: 100%;">
                            <thead>
                                <tr>
                                    <th class="sortable" data-band="${band}" data-column="country" data-type="string">Country</th>
                                    <th class="sortable" data-band="${band}" data-column="unique_callsigns" data-type="number">Unique Callsigns</th>
                                    <th class="sortable desc" data-band="${band}" data-column="total_spots" data-type="number">Total Spots</th>
                                    <th class="sortable" data-band="${band}" data-column="min_snr" data-type="number">Min SNR</th>
                                    <th class="sortable" data-band="${band}" data-column="max_snr" data-type="number">Max SNR</th>
                                    <th class="sortable" data-band="${band}" data-column="avg_snr" data-type="number">Avg SNR</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${countryList.map(c => ` + "`" + `
                                    <tr>
                                        <td><strong>${c.country}</strong></td>
                                        <td><span class="badge badge-success">${c.unique_callsigns}</span></td>
                                        <td>${c.total_spots}</td>
                                        <td>${c.min_snr} dB</td>
                                        <td>${c.max_snr} dB</td>
                                        <td>${c.avg_snr.toFixed(1)} dB</td>
                                    </tr>
                                ` + "`" + `).join('')}
                            </tbody>
                        </table>
                    </div>
                ` + "`" + `;

                container.innerHTML += tableHTML;
            });

            // Add click handlers for sorting
            document.querySelectorAll('.sortable').forEach(header => {
                header.addEventListener('click', function() {
                    const band = this.dataset.band;
                    const column = this.dataset.column;
                    const type = this.dataset.type;
                    const tableId = ` + "`" + `countryTable_${band.replace(/[^a-zA-Z0-9]/g, '_')}` + "`" + `;
                    
                    // Toggle sort direction
                    const isAsc = this.classList.contains('asc');
                    
                    // Remove sort classes from all headers in this table
                    document.querySelectorAll(` + "`" + `#${tableId} .sortable` + "`" + `).forEach(h => {
                        h.classList.remove('asc', 'desc');
                    });
                    
                    // Add appropriate class to clicked header
                    this.classList.add(isAsc ? 'desc' : 'asc');
                    
                    // Sort the data
                    sortCountryTable(band, column, type, !isAsc);
                });
            });
            
            // Create band navigation for Countries tab
            if (bands.length > 0) {
                createBandNavigation(bands, 'countriesBandNav', 'country_');
            }
        }

        function updateRelationships(instances) {
            const container = document.getElementById('relationshipsContainer');
            
            if (!container) {
                console.error('relationshipsContainer not found');
                return;
            }
            
            if (!instances || Object.keys(instances).length === 0) {
                container.innerHTML = '<p style="color: #94a3b8; text-align: center; padding: 20px;">No instance data available yet. Waiting for WSPR decodes...</p>';
                return;
            }

            // Collect all bands with both tied and duplicate relationships
            const bandTies = {}; // band -> array of tie relationships
            const bandDuplicates = {}; // band -> array of duplicate relationships
            const bandTotalDuplicates = {}; // band -> total duplicates count
            const bandTotalSpots = {}; // band -> total spots count
            
            Object.values(instances).forEach(inst => {
                Object.entries(inst.BandStats || {}).forEach(([band, stats]) => {
                    // Initialize band data structures
                    if (!bandTies[band]) bandTies[band] = {};
                    if (!bandDuplicates[band]) bandDuplicates[band] = {};
                    if (!bandTotalDuplicates[band]) bandTotalDuplicates[band] = 0;
                    if (!bandTotalSpots[band]) bandTotalSpots[band] = 0;
                    
                    // Calculate totals
                    bandTotalDuplicates[band] += (stats.TotalSpots - stats.UniqueSpots);
                    bandTotalSpots[band] += stats.TotalSpots;
                    
                    // Process TiedWith relationships
                    if (stats.TiedWith) {
                        Object.entries(stats.TiedWith).forEach(([otherInstance, count]) => {
                            if (inst.Name === otherInstance) return;
                            const pair = [inst.Name, otherInstance].sort().join(' ↔ ');
                            if (!bandTies[band][pair]) {
                                bandTies[band][pair] = {
                                    instance1: inst.Name < otherInstance ? inst.Name : otherInstance,
                                    instance2: inst.Name < otherInstance ? otherInstance : inst.Name,
                                    count: 0
                                };
                            }
                            bandTies[band][pair].count += count;
                        });
                    }
                    
                    // Process DuplicatesWith relationships
                    if (stats.DuplicatesWith) {
                        Object.entries(stats.DuplicatesWith).forEach(([otherInstance, count]) => {
                            if (inst.Name === otherInstance) return;
                            const pair = [inst.Name, otherInstance].sort().join(' ↔ ');
                            if (!bandDuplicates[band][pair]) {
                                bandDuplicates[band][pair] = {
                                    instance1: inst.Name < otherInstance ? inst.Name : otherInstance,
                                    instance2: inst.Name < otherInstance ? otherInstance : inst.Name,
                                    count: 0
                                };
                            }
                            bandDuplicates[band][pair].count += count;
                        });
                    }
                });
            });

            // Get all bands that have either ties or duplicates
            const allBands = [...new Set([...Object.keys(bandTies), ...Object.keys(bandDuplicates)])];
            const bands = sortBands(allBands);
            
            if (bands.length === 0) {
                container.innerHTML = '<p style="color: #94a3b8; text-align: center; padding: 20px;">✓ No relationships found yet.<br><span style="font-size: 0.9em; opacity: 0.8;">All spots are unique to individual instances.</span></p>';
                return;
            }

            container.innerHTML = '';
            
            // Create per-band sections with side-by-side tables
            bands.forEach(band => {
                const ties = Object.values(bandTies[band] || {});
                const dups = Object.values(bandDuplicates[band] || {});
                
                // Divide counts by 2 (counted from both sides)
                ties.forEach(tie => tie.count = Math.round(tie.count / 2));
                dups.forEach(dup => dup.count = Math.round(dup.count / 2));
                
                // Sort by count descending
                ties.sort((a, b) => b.count - a.count);
                dups.sort((a, b) => b.count - a.count);
                
                const totalDups = bandTotalDuplicates[band] || 1;
                const totalSpots = bandTotalSpots[band] || 1;
                
                const bandId = 'relationship_' + band.replace(/[^a-zA-Z0-9]/g, '_');
                
                let html = ` + "`" + `
                    <div id="${bandId}" style="margin-bottom: 40px;">
                        <h3 style="color: #60a5fa; margin-bottom: 15px;">
                            <span class="badge badge-warning" style="font-size: 1.1em; padding: 6px 14px;">${band}</span>
                            <span style="font-size: 0.9em; color: #94a3b8; margin-left: 10px;">
                                Total Spots: ${totalSpots} | Duplicates: ${totalDups}
                            </span>
                        </h3>
                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 20px;">
                ` + "`" + `;
                
                // Left column: Tied SNR Relationships
                html += ` + "`" + `
                    <div style="background: #1e293b; padding: 20px; border-radius: 12px; border: 1px solid #334155;">
                        <h4 style="color: #f59e0b; margin-bottom: 15px; font-size: 1.1em;">
                            🤝 Tied SNR Relationships
                        </h4>
                ` + "`" + `;
                
                if (ties.length === 0) {
                    html += ` + "`" + `<p style="color: #94a3b8; text-align: center; padding: 20px; font-size: 0.9em;">No tied SNR relationships on this band</p>` + "`" + `;
                } else {
                    html += ` + "`" + `
                        <table style="width: 100%; font-size: 0.9em;">
                            <thead>
                                <tr>
                                    <th style="font-size: 0.85em;">Instance Pair</th>
                                    <th style="font-size: 0.85em;">Count</th>
                                    <th style="font-size: 0.85em;">% of Dups</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${ties.map(tie => {
                                    const percentage = ((tie.count / totalDups) * 100).toFixed(1);
                                    const barWidth = Math.min(percentage, 100);
                                    return ` + "`" + `
                                        <tr>
                                            <td style="padding: 8px;">
                                                <span class="instance-name" style="font-size: 0.9em;">${tie.instance1}</span>
                                                <span style="color: #f59e0b; margin: 0 4px;">↔</span>
                                                <span class="instance-name" style="font-size: 0.9em;">${tie.instance2}</span>
                                            </td>
                                            <td style="padding: 8px;"><span class="badge badge-warning">${tie.count}</span></td>
                                            <td style="padding: 8px;">
                                                ${percentage}%
                                                <div class="progress-bar" style="height: 6px;">
                                                    <div class="progress-fill" style="width: ${barWidth}%; background: linear-gradient(90deg, #f59e0b, #d97706);"></div>
                                                </div>
                                            </td>
                                        </tr>
                                    ` + "`" + `;
                                }).join('')}
                            </tbody>
                        </table>
                    ` + "`" + `;
                }
                
                html += ` + "`" + `</div>` + "`" + `;
                
                // Right column: All Duplicate Relationships
                html += ` + "`" + `
                    <div style="background: #1e293b; padding: 20px; border-radius: 12px; border: 1px solid #334155;">
                        <h4 style="color: #3b82f6; margin-bottom: 15px; font-size: 1.1em;">
                            🔗 All Duplicate Relationships
                        </h4>
                ` + "`" + `;
                
                if (dups.length === 0) {
                    html += ` + "`" + `<p style="color: #94a3b8; text-align: center; padding: 20px; font-size: 0.9em;">No duplicate relationships on this band</p>` + "`" + `;
                } else {
                    html += ` + "`" + `
                        <table style="width: 100%; font-size: 0.9em;">
                            <thead>
                                <tr>
                                    <th style="font-size: 0.85em;">Instance Pair</th>
                                    <th style="font-size: 0.85em;">Count</th>
                                    <th style="font-size: 0.85em;">% of Spots</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${dups.map(dup => {
                                    const percentage = ((dup.count / totalSpots) * 100).toFixed(1);
                                    const barWidth = Math.min(percentage, 100);
                                    return ` + "`" + `
                                        <tr>
                                            <td style="padding: 8px;">
                                                <span class="instance-name" style="font-size: 0.9em;">${dup.instance1}</span>
                                                <span style="color: #3b82f6; margin: 0 4px;">↔</span>
                                                <span class="instance-name" style="font-size: 0.9em;">${dup.instance2}</span>
                                            </td>
                                            <td style="padding: 8px;"><span class="badge badge-primary">${dup.count}</span></td>
                                            <td style="padding: 8px;">
                                                ${percentage}%
                                                <div class="progress-bar" style="height: 6px;">
                                                    <div class="progress-fill" style="width: ${barWidth}%; background: linear-gradient(90deg, #3b82f6, #2563eb);"></div>
                                                </div>
                                            </td>
                                        </tr>
                                    ` + "`" + `;
                                }).join('')}
                            </tbody>
                        </table>
                    ` + "`" + `;
                }
                
                html += ` + "`" + `
                        </div>
                    </div>
                </div>
                ` + "`" + `;
                
                container.innerHTML += html;
            });
            
            // Create band navigation
            if (bands.length > 0) {
                createBandNavigation(bands, 'relationshipsBandNav', 'relationship_');
            }
        }

        function updateMultiInstanceAnalysis(instances) {
            const container = document.getElementById('multiInstanceAnalysis');
            
            if (!container) {
                console.error('multiInstanceAnalysis container not found');
                return;
            }
            
            if (!instances || Object.keys(instances).length === 0) {
                container.innerHTML = '<p style="color: #94a3b8; text-align: center; padding: 20px;">No instance data available yet.</p>';
                return;
            }

            // Collect per-band analysis data
            const bandAnalysis = {};

            // Organize data by band
            Object.values(instances).forEach(inst => {
                Object.entries(inst.BandStats || {}).forEach(([band, stats]) => {
                    if (!bandAnalysis[band]) {
                        bandAnalysis[band] = {
                            instances: [],
                            totalSpots: 0,
                            uniqueCallsigns: new Set() // Track unique callsigns across all instances
                        };
                    }

                    bandAnalysis[band].instances.push({
                        name: inst.Name,
                        totalSpots: stats.TotalSpots,
                        uniqueSpots: stats.UniqueSpots,
                        bestSNRWins: stats.BestSNRWins,
                        tiedSNR: stats.TiedSNR || 0,
                        duplicatesWith: stats.DuplicatesWith || {}
                    });

                    bandAnalysis[band].totalSpots += stats.TotalSpots;
                });
            });

            // Calculate metrics for each band
            const bandMetrics = {};
            Object.entries(bandAnalysis).forEach(([band, data]) => {
                if (data.instances.length === 0) return;
                
                // Find best single instance
                const bestInstance = data.instances.reduce((best, inst) =>
                    inst.totalSpots > best.totalSpots ? inst : best
                );

                // Calculate total unique callsigns across all instances
                // Total unique = best instance spots + unique spots from all other instances
                // (UniqueSpots = callsigns ONLY that instance heard, not heard by others)
                let totalUniqueAcrossAll = bestInstance.totalSpots;
                data.instances.forEach(inst => {
                    if (inst.name !== bestInstance.name) {
                        totalUniqueAcrossAll += inst.uniqueSpots;
                    }
                });

                // Coverage gain = total unique / best single instance
                // This shows how much additional coverage you get from running multiple instances
                const coverageGain = bestInstance.totalSpots > 0
                    ? totalUniqueAcrossAll / bestInstance.totalSpots
                    : 1.0;
                
                // Calculate overlap for this specific band
                // Overlap = percentage of spots where multiple instances heard the same callsign
                // in the same 2-minute window (tracked via DuplicatesWith relationships)
                let bandDuplicateCount = 0;

                // For single instance, there can be no duplicates (no other instance to duplicate with)
                if (data.instances.length > 1) {
                    // Sum up all duplicate relationships for this band
                    // DuplicatesWith is counted from both sides, so divide by 2
                    data.instances.forEach(inst => {
                        if (inst.duplicatesWith) {
                            Object.values(inst.duplicatesWith).forEach(count => {
                                bandDuplicateCount += count;
                            });
                        }
                    });
                    // Divide by 2 since each duplicate is counted from both instances
                    bandDuplicateCount = Math.round(bandDuplicateCount / 2);
                }

                const overlapPercentage = data.totalSpots > 0
                    ? (bandDuplicateCount / data.totalSpots) * 100
                    : 0;
                // Calculate total SNR wins across all instances for proper percentage calculation
                const totalSNRWins = data.instances.reduce((sum, inst) => sum + inst.bestSNRWins, 0);

                
                // Calculate unique contribution percentage for each instance
                // SNR Win Rate = (this instance's SNR wins / total SNR wins across all instances) * 100
                // This shows what % of all duplicate resolutions this instance won
                const instanceContributions = data.instances.map(inst => {
                    return {
                        name: inst.name,
                        uniquePercent: inst.totalSpots > 0 ? (inst.uniqueSpots / inst.totalSpots) * 100 : 0,
                        winRate: totalSNRWins > 0 ? (inst.bestSNRWins / totalSNRWins) * 100 : 0
                    };
                });
                
                // Determine recommendation based on coverage gain, overlap, and individual instance contributions
                let recommendation, recommendationColor, recommendationIcon;
                if (data.instances.length === 1) {
                    recommendation = 'Single instance - no multi-instance analysis available';
                    recommendationColor = '#94a3b8';
                    recommendationIcon = 'ℹ️';
                } else {
                    // Check if any instance has very low unique contribution (<5%)
                    const hasVeryLowContribution = instanceContributions.some(inst => inst.uniquePercent < 5.0);

                    if (hasVeryLowContribution) {
                        recommendation = 'High redundancy. One or more instances provide minimal unique coverage (<5%) - consider repositioning.';
                        recommendationColor = '#ef4444';
                        recommendationIcon = '❌';
                    } else if (coverageGain >= 1.5) {
                        recommendation = 'Excellent diversity! Multiple instances provide significant coverage gain.';
                        recommendationColor = '#10b981';
                        recommendationIcon = '✅';
                    } else if (coverageGain >= 1.3) {
                        recommendation = 'Good setup. Multiple instances provide valuable additional coverage.';
                        recommendationColor = '#22c55e';
                        recommendationIcon = '👍';
                    } else if (coverageGain >= 1.15) {
                        recommendation = 'Moderate benefit. Consider optimizing weaker instances for better diversity.';
                        recommendationColor = '#f59e0b';
                        recommendationIcon = '⚠️';
                    } else if (coverageGain >= 1.05) {
                        recommendation = 'Limited benefit. Consider optimizing antenna placement or instance configuration.';
                        recommendationColor = '#f59e0b';
                        recommendationIcon = '⚠️';
                    } else if (overlapPercentage < 70) {
                        // Even with low coverage gain, if overlap is reasonable, it's still providing some value
                        recommendation = 'Modest benefit. Instances provide some additional coverage despite similar reception patterns.';
                        recommendationColor = '#f59e0b';
                        recommendationIcon = '⚠️';
                    } else {
                        recommendation = 'High redundancy. Instances are hearing mostly the same signals - consider repositioning.';
                        recommendationColor = '#ef4444';
                        recommendationIcon = '❌';
                    }
                }
                
                bandMetrics[band] = {
                    coverageGain,
                    overlapPercentage,
                    bestInstance: bestInstance.name,
                    bestInstanceSpots: bestInstance.totalSpots,
                    totalUniqueAcrossAll,
                    instanceCount: data.instances.length,
                    instanceContributions,
                    recommendation,
                    recommendationColor,
                    recommendationIcon
                };
            });

            // Sort bands properly
            const bands = sortBands(Object.keys(bandMetrics));
            
            if (bands.length === 0) {
                container.innerHTML = '<p style="color: #94a3b8; text-align: center; padding: 20px;">No band data available yet.</p>';
                return;
            }

            // Create summary section
            let html = '<div style="margin-bottom: 30px; padding: 20px; background: rgba(59, 130, 246, 0.1); border-radius: 12px; border: 1px solid rgba(59, 130, 246, 0.3);">';
            html += '<h3 style="color: #60a5fa; margin-bottom: 15px;">📈 Coverage Gain Analysis</h3>';
            html += '<p style="color: #cbd5e1; margin-bottom: 10px;">This analysis shows how much additional coverage you gain by running multiple instances per band.</p>';
            html += '<p style="color: #cbd5e1; font-size: 0.9em;"><strong>Coverage Gain</strong> = Total unique spots across all instances ÷ Best single instance spots</p>';
            html += '</div>';

            // Create per-band analysis
            bands.forEach(band => {
                const metrics = bandMetrics[band];
                const gainPercent = ((metrics.coverageGain - 1.0) * 100).toFixed(1);
                
                const bandId = 'value_' + band.replace(/[^a-zA-Z0-9]/g, '_');
                html += ` + "`" + `
                    <div id="${bandId}" style="margin-bottom: 30px; border: 2px solid #334155; border-radius: 12px; overflow: hidden;">
                        <div style="background: #334155; padding: 15px;">
                            <h3 style="color: #60a5fa; margin: 0;">
                                <span class="badge badge-warning" style="font-size: 1.1em; padding: 6px 14px;">${band}</span>
                                <span style="font-size: 0.9em; color: #94a3b8; margin-left: 10px;">${metrics.instanceCount} instance${metrics.instanceCount !== 1 ? 's' : ''}</span>
                            </h3>
                        </div>
                        
                        <div style="padding: 20px;">
                            <!-- Key Metrics -->
                            <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 15px; margin-bottom: 20px;">
                                <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                                    <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">COVERAGE GAIN</div>
                                    <div style="font-size: 1.8em; font-weight: bold; color: ${metrics.coverageGain >= 1.3 ? '#10b981' : metrics.coverageGain >= 1.15 ? '#f59e0b' : '#ef4444'};">
                                        ${metrics.coverageGain.toFixed(2)}x
                                    </div>
                                    <div style="color: #64748b; font-size: 0.85em; margin-top: 5px;">+${gainPercent}% more spots</div>
                                </div>
                                
                                <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                                    <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">OVERLAP</div>
                                    <div style="font-size: 1.8em; font-weight: bold; color: ${metrics.overlapPercentage < 50 ? '#10b981' : metrics.overlapPercentage < 70 ? '#f59e0b' : '#ef4444'};">
                                        ${metrics.overlapPercentage.toFixed(1)}%
                                    </div>
                                    <div style="color: #64748b; font-size: 0.85em; margin-top: 5px;">${metrics.overlapPercentage < 50 ? 'Good diversity' : metrics.overlapPercentage < 70 ? 'Moderate' : 'High redundancy'}</div>
                                </div>
                                
                                <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                                    <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">BEST INSTANCE</div>
                                    <div style="font-size: 1.2em; font-weight: bold; color: #60a5fa; margin-bottom: 5px;">
                                        ${metrics.bestInstance}
                                    </div>
                                    <div style="color: #64748b; font-size: 0.85em;">${metrics.bestInstanceSpots} spots</div>
                                </div>
                                
                                <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                                    <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">TOTAL UNIQUE</div>
                                    <div style="font-size: 1.8em; font-weight: bold; color: #3b82f6;">
                                        ${metrics.totalUniqueAcrossAll}
                                    </div>
                                    <div style="color: #64748b; font-size: 0.85em; margin-top: 5px;">Combined coverage</div>
                                </div>
                            </div>
                            
                            <!-- Recommendation -->
                            <div style="background: rgba(${metrics.recommendationColor === '#10b981' ? '16, 185, 129' : metrics.recommendationColor === '#22c55e' ? '34, 197, 94' : metrics.recommendationColor === '#f59e0b' ? '245, 158, 11' : metrics.recommendationColor === '#f97316' ? '249, 115, 22' : '239, 68, 68'}, 0.1); padding: 15px; border-radius: 8px; border: 2px solid ${metrics.recommendationColor}; margin-bottom: 20px;">
                                <div style="font-size: 1.1em; font-weight: 600; color: ${metrics.recommendationColor}; margin-bottom: 8px;">
                                    ${metrics.recommendationIcon} ${metrics.recommendation}
                                </div>
                            </div>
                            
                            <!-- Instance Contributions -->
                            ${metrics.instanceCount > 1 ? ` + "`" + `
                            <div style="margin-top: 20px;">
                                <h4 style="color: #cbd5e1; margin-bottom: 15px; font-size: 1em;">Instance Contributions</h4>
                                <table style="width: 100%; font-size: 0.9em;">
                                    <thead>
                                        <tr>
                                            <th style="text-align: left; padding: 10px; background: #1e293b;">Instance</th>
                                            <th style="text-align: center; padding: 10px; background: #1e293b;">Unique %</th>
                                            <th style="text-align: center; padding: 10px; background: #1e293b;">SNR Win Rate</th>
                                            <th style="text-align: left; padding: 10px; background: #1e293b;">Assessment</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        ${metrics.instanceContributions.map(inst => {
                                            let assessment, assessmentColor;
                                            if (inst.uniquePercent >= 25) {
                                                assessment = 'Excellent contribution';
                                                assessmentColor = '#10b981';
                                            } else if (inst.uniquePercent >= 15) {
                                                assessment = 'Good contribution';
                                                assessmentColor = '#22c55e';
                                            } else if (inst.uniquePercent >= 10) {
                                                assessment = 'Moderate contribution';
                                                assessmentColor = '#f59e0b';
                                            } else {
                                                assessment = 'Limited contribution';
                                                assessmentColor = '#ef4444';
                                            }
                                            
                                            return ` + "`" + `
                                                <tr style="border-top: 1px solid #334155;">
                                                    <td style="padding: 10px;"><span class="instance-name">${inst.name}</span></td>
                                                    <td style="padding: 10px; text-align: center;">
                                                        <span style="font-weight: 600; color: ${inst.uniquePercent >= 20 ? '#10b981' : inst.uniquePercent >= 10 ? '#f59e0b' : '#ef4444'};">
                                                            ${inst.uniquePercent.toFixed(1)}%
                                                        </span>
                                                    </td>
                                                    <td style="padding: 10px; text-align: center;">
                                                        <span style="font-weight: 600; color: #3b82f6;">
                                                            ${inst.winRate.toFixed(1)}%
                                                        </span>
                                                    </td>
                                                    <td style="padding: 10px; color: ${assessmentColor}; font-size: 0.9em;">
                                                        ${assessment}
                                                    </td>
                                                </tr>
                                            ` + "`" + `;
                                        }).join('')}
                                    </tbody>
                                </table>
                            </div>
                            ` + "`" + ` : ''}
                        </div>
                    </div>
                ` + "`" + `;
            });

            container.innerHTML = html;
            
            // Create band navigation for Value tab
            if (bands.length > 0) {
                createBandNavigation(bands, 'valueBandNav', 'value_');
            }
        }

        function sortCountryTable(band, column, type, ascending) {
            const countryList = [...countryData[band]];
            
            countryList.sort((a, b) => {
                let aVal = a[column];
                let bVal = b[column];
                
                if (type === 'number') {
                    aVal = parseFloat(aVal);
                    bVal = parseFloat(bVal);
                }
                
                if (ascending) {
                    return aVal > bVal ? 1 : aVal < bVal ? -1 : 0;
                } else {
                    return aVal < bVal ? 1 : aVal > bVal ? -1 : 0;
                }
            });
            
            // Update table body
            const tableId = ` + "`" + `countryTable_${band.replace(/[^a-zA-Z0-9]/g, '_')}` + "`" + `;
            const tbody = document.querySelector(` + "`" + `#${tableId} tbody` + "`" + `);
            
            tbody.innerHTML = countryList.map(c => ` + "`" + `
                <tr>
                    <td><strong>${c.country}</strong></td>
                    <td><span class="badge badge-success">${c.unique_callsigns}</span></td>
                    <td>${c.total_spots}</td>
                    <td>${c.min_snr} dB</td>
                    <td>${c.max_snr} dB</td>
                    <td>${c.avg_snr.toFixed(1)} dB</td>
                </tr>
            ` + "`" + `).join('');
        }

        // Store SNR charts globally
        const snrCharts = {};

        // Apply moving average smoothing to data (works for both SNR and spot count data)
        function applySmoothingToSNR(data, windowSize = 5) {
            if (data.length < windowSize) return data;

            const smoothed = [];
            for (let i = 0; i < data.length; i++) {
                const start = Math.max(0, i - Math.floor(windowSize / 2));
                const end = Math.min(data.length, i + Math.ceil(windowSize / 2));
                const window = data.slice(start, end);
                const sum = window.reduce((acc, point) => acc + point.y, 0);
                smoothed.push({
                    x: data[i].x,
                    y: sum / window.length
                });
            }
            return smoothed;
        }

        // Apply moving average smoothing to simple array data
        function applySmoothingToArray(data, windowSize = 5) {
            if (data.length < windowSize) return data;

            const smoothed = [];
            for (let i = 0; i < data.length; i++) {
                const start = Math.max(0, i - Math.floor(windowSize / 2));
                const end = Math.min(data.length, i + Math.ceil(windowSize / 2));
                const window = data.slice(start, end);
                const sum = window.reduce((acc, val) => acc + val, 0);
                smoothed.push(sum / window.length);
            }
            return smoothed;
        }

        function updateSNRHistoryCharts(snrHistory) {
            // Store raw data for re-rendering when smoothing is toggled
            rawSNRData = snrHistory;
            const container = document.getElementById('snrHistoryCharts');
            
            if (!snrHistory || Object.keys(snrHistory).length === 0) {
                container.innerHTML = '<p style="color: #94a3b8; text-align: center;">No SNR history data available yet</p>';
                return;
            }

            // Sort bands properly
            const bands = sortBands(Object.keys(snrHistory));

            container.innerHTML = '';

            bands.forEach(band => {
                const bandData = snrHistory[band];
                if (!bandData || !bandData.instances || Object.keys(bandData.instances).length === 0) {
                    return;
                }

                const chartId = ` + "`" + `snrChart_${band.replace(/[^a-zA-Z0-9]/g, '_')}` + "`" + `;

                const bandId = 'snr_' + band.replace(/[^a-zA-Z0-9]/g, '_');
                const chartHTML = ` + "`" + `
                    <div id="${bandId}" style="margin-bottom: 40px;">
                        <h3 style="color: #60a5fa; margin-bottom: 15px;">
                            <span class="badge badge-warning" style="font-size: 1.1em; padding: 6px 14px;">${band}</span>
                            <span style="font-size: 0.8em; color: #94a3b8; margin-left: 10px;">Average SNR Over Time</span>
                        </h3>
                        <div style="background: #1e293b; padding: 20px; border-radius: 12px; border: 1px solid #334155;">
                            <canvas id="${chartId}" style="max-height: 400px;"></canvas>
                        </div>
                    </div>
                ` + "`" + `;

                container.innerHTML += chartHTML;

                // Create chart after DOM is updated
                setTimeout(() => {
                    const ctx = document.getElementById(chartId);
                    if (!ctx) return;

                    // Sort instances alphabetically
                    const instanceNames = Object.keys(bandData.instances).sort();

                    // Generate colors for each instance
                    const colors = [
                        '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
                        '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16',
                        '#f97316', '#14b8a6', '#a855f7', '#22c55e'
                    ];

                    const datasets = instanceNames.map((instance, idx) => {
                        const points = bandData.instances[instance];
                        const color = colors[idx % colors.length];

                        let dataPoints = points.map(p => ({
                            x: new Date(p.window_time),
                            y: p.average_snr
                        }));

                        // Apply smoothing if enabled
                        if (snrSmoothingEnabled) {
                            dataPoints = applySmoothingToSNR(dataPoints, 5);
                        }

                        return {
                            label: instance,
                            data: dataPoints,
                            borderColor: color,
                            backgroundColor: color + '20',
                            borderWidth: 1.5,
                            tension: 0.4,
                            pointRadius: 0,
                            pointHoverRadius: 3
                        };
                    });

                    // Destroy existing chart if it exists
                    if (snrCharts[band]) {
                        snrCharts[band].destroy();
                    }

                    snrCharts[band] = new Chart(ctx, {
                        type: 'line',
                        data: {
                            datasets: datasets
                        },
                        options: {
                            responsive: true,
                            maintainAspectRatio: true,
                            plugins: {
                                legend: {
                                    labels: { color: '#e2e8f0' },
                                    position: 'top'
                                },
                                title: {
                                    display: true,
                                    text: ` + "`" + `${band} - Average SNR per Instance` + "`" + `,
                                    color: '#f1f5f9',
                                    font: { size: 16 }
                                },
                                tooltip: {
                                    callbacks: {
                                        label: function(context) {
                                            return context.dataset.label + ': ' + context.parsed.y.toFixed(1) + ' dB';
                                        }
                                    }
                                }
                            },
                            scales: {
                                x: {
                                    type: 'time',
                                    time: {
                                        unit: 'minute',
                                        displayFormats: {
                                            minute: 'HH:mm'
                                        }
                                    },
                                    ticks: { color: '#94a3b8' },
                                    grid: { color: '#334155' },
                                    title: {
                                        display: true,
                                        text: 'Time (UTC)',
                                        color: '#94a3b8'
                                    }
                                },
                                y: {
                                    beginAtZero: false,
                                    ticks: {
                                        color: '#94a3b8',
                                        callback: function(value) {
                                            return value + ' dB';
                                        }
                                    },
                                    grid: { color: '#334155' },
                                    title: {
                                        display: true,
                                        text: 'Average SNR (dB)',
                                        color: '#94a3b8'
                                    }
                                }
                            }
                        }
                    });
                }, 0);
            });

            if (bands.length === 0) {
                container.innerHTML = '<p style="color: #94a3b8; text-align: center;">No SNR history data available yet</p>';
            }
            
            // Create band navigation for SNR tab
            if (bands.length > 0) {
                createBandNavigation(bands, 'snrBandNav', 'snr_');
            }
        }

        // Add SNR smoothing toggle handler
        document.getElementById('snrSmoothingToggle').addEventListener('change', function(e) {
            snrSmoothingEnabled = e.target.checked;
            // Re-render SNR charts with current data
            if (rawSNRData && Object.keys(rawSNRData).length > 0) {
                updateSNRHistoryCharts(rawSNRData);
            }
        });

        // Add band performance smoothing toggle handler
        document.getElementById('bandSmoothingToggle').addEventListener('change', function(e) {
            bandSmoothingEnabled = e.target.checked;
            // Re-render band instance tables with current data
            if (rawBandData && rawBandData.instances) {
                updateBandInstanceTable(rawBandData.instances, rawBandData.snrHistory);
            }
        });

        // Add instance performance smoothing toggle handler
        document.getElementById('instanceSmoothingToggle').addEventListener('change', function(e) {
            instanceSmoothingEnabled = e.target.checked;
            // Re-render instance performance chart with current data
            if (rawInstanceData && Object.keys(rawInstanceData).length > 0) {
                updateInstancePerformanceChart(rawInstanceData);
            }
        });

        // Add raw instance performance smoothing toggle handler
        document.getElementById('instanceRawSmoothingToggle').addEventListener('change', function(e) {
            instanceRawSmoothingEnabled = e.target.checked;
            // Re-render raw instance performance chart with current data
            if (rawInstanceRawData && Object.keys(rawInstanceRawData).length > 0) {
                updateInstancePerformanceRawChart(rawInstanceRawData);
            }
        });

        // Add spots over time smoothing toggle handler
        document.getElementById('spotsSmoothingToggle').addEventListener('change', function(e) {
            spotsSmoothingEnabled = e.target.checked;
            // Re-render spots chart with current data
            if (rawWindowsData && rawWindowsData.length > 0) {
                updateCharts(rawWindowsData);
            }
        });

        // Load spots data
        async function loadSpots() {
            const sourceFilter = document.getElementById('spotSourceFilter').value;
            const bandFilter = document.getElementById('spotBandFilter').value;
            const timeFilter = parseInt(document.getElementById('spotTimeFilter').value);
            const submittedFilter = document.getElementById('spotSubmittedFilter').value;

            // Calculate time range
            const endTime = new Date();
            const startTime = new Date(endTime.getTime() - (timeFilter * 60 * 60 * 1000));

            try {
                let url;
                const params = new URLSearchParams();
                
                if (bandFilter) params.append('band', bandFilter);
                params.append('start_time', startTime.toISOString());
                params.append('end_time', endTime.toISOString());

                if (sourceFilter === 'deduped') {
                    if (submittedFilter) params.append('submitted', submittedFilter);
                    url = '/api/spots/deduped?' + params.toString();
                } else {
                    params.append('instance', sourceFilter);
                    url = '/api/spots/raw?' + params.toString();
                }

                const response = await fetch(url);
                currentSpots = await response.json();

                // Update summary
                updateSpotsSummary(currentSpots, sourceFilter === 'deduped');

                // Display spots
                displaySpots(currentSpots, sourceFilter === 'deduped');
            } catch (error) {
                console.error('Error loading spots:', error);
                document.getElementById('spotsTableBody').innerHTML =
                    '<tr><td colspan="9" style="text-align: center; padding: 40px; color: #ef4444;">Error loading spots</td></tr>';
            }
        }

        // Update spots summary
        function updateSpotsSummary(spots, isDeduped) {
            const container = document.getElementById('spotsSummary');
            
            const totalSpots = spots.length;
            let successfulSpots = 0;
            let failedSpots = 0;
            const uniqueCallsigns = new Set();
            const bands = new Set();
            const bandCallsigns = {}; // Track unique callsigns per band

            spots.forEach(spot => {
                uniqueCallsigns.add(spot.callsign);
                bands.add(spot.band);
                
                if (!bandCallsigns[spot.band]) {
                    bandCallsigns[spot.band] = new Set();
                }
                bandCallsigns[spot.band].add(spot.callsign);
                
                if (isDeduped) {
                    if (spot.submitted) successfulSpots++;
                    else failedSpots++;
                }
            });

            let html = ` + "`" + `
                <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                    <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Total Spots</div>
                    <div style="font-size: 1.5em; font-weight: bold; color: #60a5fa;">${totalSpots}</div>
                </div>
                <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                    <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Unique Callsigns</div>
                    <div style="font-size: 1.5em; font-weight: bold; color: #10b981;">${uniqueCallsigns.size}</div>
                </div>
                <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                    <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Bands Active</div>
                    <div style="font-size: 1.5em; font-weight: bold; color: #8b5cf6;">${bands.size}</div>
                </div>
            ` + "`" + `;

            if (isDeduped) {
                html += ` + "`" + `
                    <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Successfully Sent</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #10b981;">${successfulSpots}</div>
                    </div>
                    <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Failed to Send</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #ef4444;">${failedSpots}</div>
                    </div>
                ` + "`" + `;
            }

            container.innerHTML = html;

            // Add band breakdown section below the summary
            if (Object.keys(bandCallsigns).length > 0) {
                const sortedBands = sortBands(Object.keys(bandCallsigns));
                const bandBreakdownHTML = ` + "`" + `
                    <div style="grid-column: 1 / -1; background: rgba(139, 92, 246, 0.1); padding: 15px; border-radius: 8px; border: 1px solid rgba(139, 92, 246, 0.3); margin-top: 15px;">
                        <div style="color: #cbd5e1; font-size: 0.9em; margin-bottom: 10px; font-weight: 600;">Unique Callsigns per Band:</div>
                        <div style="display: flex; flex-wrap: wrap; gap: 12px; align-items: center;">
                            ${sortedBands.map(band => {
                                const count = bandCallsigns[band].size;
                                const bandColor = bandColors[band] || '#8b5cf6';
                                return ` + "`" + `
                                    <div style="display: flex; align-items: center; gap: 6px;">
                                        <span class="badge" style="background: ${bandColor}; color: white; font-size: 0.9em;">${band}</span>
                                        <span style="color: #e2e8f0; font-weight: 600;">${count}</span>
                                    </div>
                                ` + "`" + `;
                            }).join('')}
                        </div>
                    </div>
                ` + "`" + `;
                container.innerHTML += bandBreakdownHTML;
            }
        }

        // Display spots in table with pagination
        function displaySpots(spots, isDeduped) {
            const tbody = document.getElementById('spotsTableBody');
            
            if (spots.length === 0) {
                tbody.innerHTML = '<tr><td colspan="9" style="text-align: center; padding: 40px; color: #94a3b8;">No spots found for selected filters</td></tr>';
                document.getElementById('spotsCount').textContent = 'Showing 0 spots';
                document.getElementById('spotsPagination').innerHTML = '';
                return;
            }

            // Sort spots
            const sortedSpots = [...spots].sort((a, b) => {
                let aVal = a[spotSortColumn];
                let bVal = b[spotSortColumn];

                if (spotSortColumn === 'timestamp') {
                    aVal = new Date(aVal).getTime();
                    bVal = new Date(bVal).getTime();
                } else if (spotSortColumn === 'frequency') {
                    aVal = parseInt(aVal);
                    bVal = parseInt(bVal);
                } else if (typeof aVal === 'string') {
                    aVal = aVal.toLowerCase();
                    bVal = bVal.toLowerCase();
                }

                if (spotSortAscending) {
                    return aVal > bVal ? 1 : aVal < bVal ? -1 : 0;
                } else {
                    return aVal < bVal ? 1 : aVal > bVal ? -1 : 0;
                }
            });

            // Calculate pagination
            const totalSpots = sortedSpots.length;
            const perPage = spotsPerPage === 'all' ? totalSpots : parseInt(spotsPerPage);
            const totalPages = Math.ceil(totalSpots / perPage);
            
            // Ensure current page is valid
            if (currentPage > totalPages) currentPage = totalPages;
            if (currentPage < 1) currentPage = 1;
            
            // Get spots for current page
            const startIdx = (currentPage - 1) * perPage;
            const endIdx = spotsPerPage === 'all' ? totalSpots : Math.min(startIdx + perPage, totalSpots);
            const pageSpots = sortedSpots.slice(startIdx, endIdx);

            tbody.innerHTML = pageSpots.map(spot => {
                const timestamp = new Date(spot.timestamp);
                const freqMHz = (spot.frequency / 1000000).toFixed(4);
                
                let statusHtml = '';
                if (isDeduped) {
                    if (spot.submitted) {
                        statusHtml = '<span class="badge badge-success">✓ Sent</span>';
                    } else {
                        const errorMsg = spot.error || 'Unknown error';
                        statusHtml = ` + "`" + `<span class="badge" style="background: #ef4444; color: white;" title="${errorMsg}">✗ Failed</span>` + "`" + `;
                    }
                }

                const bandColor = bandColors[spot.band] || '#f59e0b';
                
                return ` + "`" + `
                    <tr>
                        <td>${timestamp.toLocaleString()}</td>
                        <td><strong>${spot.callsign}</strong></td>
                        <td>${spot.locator}</td>
                        <td style="color: ${spot.snr >= 0 ? '#10b981' : '#ef4444'};">${spot.snr > 0 ? '+' : ''}${spot.snr} dB</td>
                        <td>${freqMHz} MHz</td>
                        <td><span class="badge" style="background: ${bandColor}; color: white;">${spot.band}</span></td>
                        <td>${spot.dbm} dBm</td>
                        <td>${spot.country || '-'}</td>
                        <td>${spot.instance || '-'}</td>
                        <td>${statusHtml}</td>
                    </tr>
                ` + "`" + `;
            }).join('');

            // Update count and pagination
            document.getElementById('spotsCount').textContent = ` + "`" + `Showing ${startIdx + 1}-${endIdx} of ${totalSpots} spot${totalSpots !== 1 ? 's' : ''}` + "`" + `;
            updatePagination(totalPages);
        }

        // Update pagination controls
        function updatePagination(totalPages) {
            const container = document.getElementById('spotsPagination');
            
            if (totalPages <= 1) {
                container.innerHTML = '';
                return;
            }

            let html = '';
            
            // Previous button
            html += ` + "`" + `
                <button class="control-btn" onclick="changePage(${currentPage - 1})" ${currentPage === 1 ? 'disabled' : ''}
                    style="${currentPage === 1 ? 'opacity: 0.3; cursor: not-allowed;' : ''}">
                    ← Prev
                </button>
            ` + "`" + `;

            // Page numbers
            const maxButtons = 5;
            let startPage = Math.max(1, currentPage - Math.floor(maxButtons / 2));
            let endPage = Math.min(totalPages, startPage + maxButtons - 1);
            
            if (endPage - startPage < maxButtons - 1) {
                startPage = Math.max(1, endPage - maxButtons + 1);
            }

            if (startPage > 1) {
                html += ` + "`" + `<button class="control-btn" onclick="changePage(1)">1</button>` + "`" + `;
                if (startPage > 2) {
                    html += ` + "`" + `<span style="color: #94a3b8; padding: 0 5px;">...</span>` + "`" + `;
                }
            }

            for (let i = startPage; i <= endPage; i++) {
                const isActive = i === currentPage;
                html += ` + "`" + `
                    <button class="control-btn" onclick="changePage(${i})"
                        style="${isActive ? 'background: #60a5fa; border-color: #60a5fa; color: white;' : ''}">
                        ${i}
                    </button>
                ` + "`" + `;
            }

            if (endPage < totalPages) {
                if (endPage < totalPages - 1) {
                    html += ` + "`" + `<span style="color: #94a3b8; padding: 0 5px;">...</span>` + "`" + `;
                }
                html += ` + "`" + `<button class="control-btn" onclick="changePage(${totalPages})">${totalPages}</button>` + "`" + `;
            }

            // Next button
            html += ` + "`" + `
                <button class="control-btn" onclick="changePage(${currentPage + 1})" ${currentPage === totalPages ? 'disabled' : ''}
                    style="${currentPage === totalPages ? 'opacity: 0.3; cursor: not-allowed;' : ''}">
                    Next →
                </button>
            ` + "`" + `;

            container.innerHTML = html;
        }

        // Change page
        function changePage(page) {
            currentPage = page;
            const sourceFilter = document.getElementById('spotSourceFilter').value;
            displaySpots(currentSpots, sourceFilter === 'deduped');
        }

        // Export spots to CSV
        function exportSpots() {
            if (currentSpots.length === 0) {
                alert('No spots to export');
                return;
            }

            const sourceFilter = document.getElementById('spotSourceFilter').value;
            const isDeduped = sourceFilter === 'deduped';

            let csv = 'Timestamp,Callsign,Locator,SNR,Frequency,Band,Power,';
            if (isDeduped) {
                csv += 'Instance,Submitted,Error\n';
            } else {
                csv += 'Country\n';
            }

            currentSpots.forEach(spot => {
                const timestamp = new Date(spot.timestamp).toISOString();
                const freqMHz = (spot.frequency / 1000000).toFixed(4);
                
                csv += ` + "`" + `${timestamp},${spot.callsign},${spot.locator},${spot.snr},${freqMHz},${spot.band},${spot.dbm},` + "`" + `;
                
                if (isDeduped) {
                    csv += ` + "`" + `${spot.instance || ''},${spot.submitted ? 'Yes' : 'No'},"${spot.error || ''}"\n` + "`" + `;
                } else {
                    csv += ` + "`" + `${spot.country || ''}\n` + "`" + `;
                }
            });

            const blob = new Blob([csv], { type: 'text/csv' });
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = ` + "`" + `wspr_spots_${new Date().toISOString().split('T')[0]}.csv` + "`" + `;
            a.click();
            window.URL.revokeObjectURL(url);
        }

        // Spots tab variables
        let currentSpots = [];
        let allLoadedSpots = [];
        let spotSortColumn = 'timestamp';
        let spotSortAscending = false;
        let selectedSpotBand = 'all';
        let currentPage = 1;
        let spotsPerPage = 100;

        // Load spots data
        async function loadSpots() {
            const sourceFilter = document.getElementById('spotSourceFilter').value;
            const timeFilter = parseInt(document.getElementById('spotTimeFilter').value);
            const submittedFilter = document.getElementById('spotSubmittedFilter').value;

            const endTime = new Date();
            const startTime = new Date(endTime.getTime() - (timeFilter * 60 * 60 * 1000));

            try {
                let url;
                const params = new URLSearchParams();
                
                params.append('start_time', startTime.toISOString());
                params.append('end_time', endTime.toISOString());

                if (sourceFilter === 'deduped') {
                    if (submittedFilter) params.append('submitted', submittedFilter);
                    url = '/api/spots/deduped?' + params.toString();
                } else {
                    params.append('instance', sourceFilter);
                    url = '/api/spots/raw?' + params.toString();
                }

                const response = await fetch(url);
                allLoadedSpots = await response.json();

                filterAndDisplaySpots(sourceFilter === 'deduped');
            } catch (error) {
                console.error('Error loading spots:', error);
                document.getElementById('spotsTableBody').innerHTML =
                    '<tr><td colspan="9" style="text-align: center; padding: 40px; color: #ef4444;">Error loading spots</td></tr>';
            }
        }

        function filterAndDisplaySpots(isDeduped) {
            const callsignSearch = document.getElementById('spotCallsignSearch').value.toUpperCase();
            const countryFilter = document.getElementById('spotCountryFilter').value;
            
            let filtered = allLoadedSpots;
            
            if (selectedSpotBand && selectedSpotBand !== 'all') {
                filtered = filtered.filter(spot => spot.band === selectedSpotBand);
            }
            
            if (callsignSearch) {
                filtered = filtered.filter(spot => spot.callsign.toUpperCase().includes(callsignSearch));
            }
            
            if (countryFilter) {
                filtered = filtered.filter(spot => spot.country === countryFilter);
            }
            
            currentSpots = filtered;
            updateSpotsSummary(currentSpots, isDeduped);
            displaySpots(currentSpots, isDeduped);
            
            // Update country dropdown with available countries from loaded spots
            updateCountryDropdown(allLoadedSpots);
        }
        
        function updateCountryDropdown(spots) {
            const countrySelect = document.getElementById('spotCountryFilter');
            const currentValue = countrySelect.value;
            
            // Get unique countries
            const countries = new Set();
            spots.forEach(spot => {
                if (spot.country) countries.add(spot.country);
            });
            
            // Rebuild dropdown
            const sortedCountries = Array.from(countries).sort();
            countrySelect.innerHTML = '<option value="">All Countries</option>';
            sortedCountries.forEach(country => {
                const option = document.createElement('option');
                option.value = country;
                option.textContent = country;
                if (country === currentValue) option.selected = true;
                countrySelect.appendChild(option);
            });
        }

        function setSpotBandFilter(band) {
            selectedSpotBand = band;
            currentPage = 1;
            
            document.querySelectorAll('#spotBandButtons .filter-btn').forEach(btn => {
                if (btn.dataset.band === band) {
                    btn.classList.add('active');
                    btn.classList.remove('inactive');
                } else {
                    btn.classList.remove('active');
                    btn.classList.add('inactive');
                }
            });
            
            const sourceFilter = document.getElementById('spotSourceFilter').value;
            filterAndDisplaySpots(sourceFilter === 'deduped');
        }

        // Initialize spots tab
        async function initSpotsTab() {
            try {
                const instances = await fetch('/api/spots/instances').then(r => r.json());
                const sourceSelect = document.getElementById('spotSourceFilter');
                
                instances.sort().forEach(instance => {
                    const option = document.createElement('option');
                    option.value = instance;
                    option.textContent = ` + "`" + `Instance: ${instance}` + "`" + `;
                    sourceSelect.appendChild(option);
                });
            } catch (error) {
                console.error('Error loading instances:', error);
            }

            // Create band filter buttons
            const bandButtonsContainer = document.getElementById('spotBandButtons');
            const bands = ['all', '2200m', '630m', '160m', '80m', '60m', '40m', '30m', '20m', '17m', '15m', '12m', '10m'];
            bands.forEach(band => {
                const btn = document.createElement('button');
                btn.className = 'filter-btn' + (band === 'all' ? ' active' : ' inactive');
                btn.dataset.band = band;
                btn.textContent = band === 'all' ? 'All Bands' : band;
                if (band !== 'all') {
                    btn.style.borderColor = bandColors[band] || '#475569';
                    btn.style.color = bandColors[band] || '#e2e8f0';
                } else {
                    btn.style.borderColor = '#60a5fa';
                    btn.style.color = '#60a5fa';
                }
                btn.onclick = () => setSpotBandFilter(band);
                bandButtonsContainer.appendChild(btn);
            });

            document.getElementById('spotSourceFilter').addEventListener('change', () => {
                const sourceFilter = document.getElementById('spotSourceFilter').value;
                const submittedFilter = document.getElementById('spotSubmittedFilter').parentElement;
                submittedFilter.style.display = sourceFilter === 'deduped' ? 'block' : 'none';
                currentPage = 1;
                loadSpots();
            });
            document.getElementById('spotTimeFilter').addEventListener('change', () => {
                currentPage = 1;
                loadSpots();
            });
            document.getElementById('spotSubmittedFilter').addEventListener('change', () => {
                currentPage = 1;
                loadSpots();
            });
            
            // Per-page selector
            document.getElementById('spotsPerPage').addEventListener('change', (e) => {
                spotsPerPage = e.target.value;
                currentPage = 1;
                const sourceFilter = document.getElementById('spotSourceFilter').value;
                displaySpots(currentSpots, sourceFilter === 'deduped');
            });
            
            // Real-time callsign search
            document.getElementById('spotCallsignSearch').addEventListener('input', () => {
                currentPage = 1;
                const sourceFilter = document.getElementById('spotSourceFilter').value;
                filterAndDisplaySpots(sourceFilter === 'deduped');
            });
            
            // Country filter
            document.getElementById('spotCountryFilter').addEventListener('change', () => {
                currentPage = 1;
                const sourceFilter = document.getElementById('spotSourceFilter').value;
                filterAndDisplaySpots(sourceFilter === 'deduped');
            });

            document.querySelectorAll('#spotsTable .sortable').forEach(header => {
                header.addEventListener('click', function() {
                    const column = this.dataset.column;
                    
                    if (spotSortColumn === column) {
                        spotSortAscending = !spotSortAscending;
                    } else {
                        spotSortColumn = column;
                        spotSortAscending = false;
                    }

                    document.querySelectorAll('#spotsTable .sortable').forEach(h => {
                        h.classList.remove('asc', 'desc');
                    });
                    this.classList.add(spotSortAscending ? 'asc' : 'desc');

                    const sourceFilter = document.getElementById('spotSourceFilter').value;
                    displaySpots(currentSpots, sourceFilter === 'deduped');
                });
            });

            loadSpots();
        }

        // Gaps tab functions
        let allGapsData = {}; // Store all gaps data for filtering

        async function loadGaps() {
            const timeFilter = parseInt(document.getElementById('gapsTimeFilter').value);
            
            try {
                const response = await fetch(` + "`" + `/api/spots/gaps?hours=${timeFilter}` + "`" + `);
                const gaps = await response.json();
                
                // Store all gaps data
                allGapsData = gaps;
                
                // Populate instance dropdown
                populateGapsInstanceFilter(gaps);
                
                // Display with current filter
                displayGaps(gaps, timeFilter);
            } catch (error) {
                console.error('Error loading gaps:', error);
                document.getElementById('gapsDetails').innerHTML =
                    '<p style="color: #ef4444; text-align: center; padding: 40px;">Error loading gap analysis</p>';
            }
        }

        function populateGapsInstanceFilter(gaps) {
            const select = document.getElementById('gapsInstanceFilter');
            const currentValue = select.value;
            
            // Get all instance names
            const instances = Object.keys(gaps).filter(inst => inst !== 'deduped').sort();
            
            // Rebuild dropdown
            let html = '<option value="all">All Instances</option>';
            html += '<option value="deduped">Deduped (Sent to WSPRNet)</option>';
            
            instances.forEach(instance => {
                html += '<option value="' + instance + '">Instance: ' + instance + '</option>';
            });
            
            select.innerHTML = html;
            
            // Restore previous selection if it still exists
            if (currentValue && (currentValue === 'all' || currentValue === 'deduped' || instances.includes(currentValue))) {
                select.value = currentValue;
            }
        }

        function filterGapsByInstance() {
            displayGaps(allGapsData, parseInt(document.getElementById('gapsTimeFilter').value));
        }

        function filterCommonGaps(gaps) {
            // Find cycles that appear as gaps across multiple bands or instances
            const cycleCounts = {}; // cycle time -> count of how many band/instance combinations have this gap
            const cycleDetails = {}; // cycle time -> array of {instance, band}
            
            // Count occurrences of each missing cycle
            Object.entries(gaps).forEach(([instance, bandGaps]) => {
                bandGaps.forEach(gap => {
                    gap.missing_cycles.forEach(cycle => {
                        if (!cycleCounts[cycle]) {
                            cycleCounts[cycle] = 0;
                            cycleDetails[cycle] = [];
                        }
                        cycleCounts[cycle]++;
                        cycleDetails[cycle].push({ instance: instance, band: gap.band });
                    });
                });
            });
            
            // Find cycles that appear in multiple places (threshold: at least 2)
            const commonCycles = new Set();
            Object.entries(cycleCounts).forEach(([cycle, count]) => {
                if (count >= 2) {
                    commonCycles.add(cycle);
                }
            });
            
            // Filter gaps to only include common cycles
            const filtered = {};
            Object.entries(gaps).forEach(([instance, bandGaps]) => {
                const filteredBandGaps = [];
                bandGaps.forEach(gap => {
                    const commonMissingCycles = gap.missing_cycles.filter(cycle => commonCycles.has(cycle));
                    if (commonMissingCycles.length > 0) {
                        filteredBandGaps.push({
                            ...gap,
                            missing_cycles: commonMissingCycles,
                            gap_count: commonMissingCycles.length,
                            coverage_rate: ((gap.total_cycles - commonMissingCycles.length) / gap.total_cycles) * 100
                        });
                    }
                });
                if (filteredBandGaps.length > 0) {
                    filtered[instance] = filteredBandGaps;
                }
            });
            
            return filtered;
        }

        function displayGaps(gaps, hoursBack) {
            const summaryContainer = document.getElementById('gapsSummary');
            const detailsContainer = document.getElementById('gapsDetails');
            
            // Apply instance filter
            const instanceFilter = document.getElementById('gapsInstanceFilter').value;
            let filteredGaps = gaps;
            
            if (instanceFilter !== 'all') {
                // Filter to show only selected instance
                filteredGaps = {};
                if (gaps[instanceFilter]) {
                    filteredGaps[instanceFilter] = gaps[instanceFilter];
                }
            }
            
            // Apply gap type filter (common gaps)
            const gapTypeFilter = document.getElementById('gapsTypeFilter').value;
            if (gapTypeFilter === 'common') {
                filteredGaps = filterCommonGaps(filteredGaps);
            }
            
            if (!filteredGaps || Object.keys(filteredGaps).length === 0) {
                summaryContainer.innerHTML = '';
                detailsContainer.innerHTML = '<p style="color: #10b981; text-align: center; padding: 40px; font-size: 1.2em;">✓ No gaps found! All WSPR cycles have spots.</p>';
                document.getElementById('gapsBandNav').innerHTML = '';
                return;
            }

            // Collect all bands with gaps for navigation
            const bandsWithGaps = new Set();
            Object.values(filteredGaps).forEach(bandGaps => {
                bandGaps.forEach(gap => bandsWithGaps.add(gap.band));
            });
            const sortedBandsWithGaps = sortBands(Array.from(bandsWithGaps));

            // Create band navigation
            createBandNavigation(sortedBandsWithGaps, 'gapsBandNav', 'gap_');

            // Calculate summary statistics
            let totalGaps = 0;
            let instancesWithGaps = 0;
            let worstCoverage = 100;
            let worstInstance = '';
            let worstBand = '';

            Object.entries(filteredGaps).forEach(([instance, bandGaps]) => {
                if (bandGaps.length > 0) {
                    instancesWithGaps++;
                    bandGaps.forEach(gap => {
                        totalGaps += gap.gap_count;
                        if (gap.coverage_rate < worstCoverage) {
                            worstCoverage = gap.coverage_rate;
                            worstInstance = instance;
                            worstBand = gap.band;
                        }
                    });
                }
            });

            // Display summary
            summaryContainer.innerHTML = ` + "`" + `
                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 15px;">
                    <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Instances with Gaps</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #f59e0b;">${instancesWithGaps}</div>
                    </div>
                    <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Total Missing Cycles</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #ef4444;">${totalGaps}</div>
                    </div>
                    <div style="background: #1e293b; padding: 15px; border-radius: 8px; border: 1px solid #334155;">
                        <div style="color: #94a3b8; font-size: 0.85em; margin-bottom: 5px;">Worst Coverage</div>
                        <div style="font-size: 1.5em; font-weight: bold; color: #ef4444;">${worstCoverage.toFixed(1)}%</div>
                        <div style="color: #64748b; font-size: 0.85em; margin-top: 5px;">${worstInstance} on ${worstBand}</div>
                    </div>
                </div>
            ` + "`" + `;

            // Reorganize data by band first, then instance
            const bandData = {};
            Object.entries(filteredGaps).forEach(([instance, bandGaps]) => {
                bandGaps.forEach(gap => {
                    if (!bandData[gap.band]) {
                        bandData[gap.band] = [];
                    }
                    bandData[gap.band].push({
                        instance: instance,
                        ...gap
                    });
                });
            });

            // Display details per band
            let html = '';
            const sortedBands = sortBands(Object.keys(bandData));

            sortedBands.forEach(band => {
                const instanceGaps = bandData[band];
                if (instanceGaps.length === 0) return;

                // Sort by coverage rate (worst first)
                instanceGaps.sort((a, b) => a.coverage_rate - b.coverage_rate);

                const bandColor = bandColors[band] || '#f59e0b';
                const bandId = 'gap_' + band.replace(/[^a-zA-Z0-9]/g, '_');

                html += ` + "`" + `
                    <div id="${bandId}" style="margin-bottom: 30px; border: 2px solid #334155; border-radius: 12px; overflow: hidden;">
                        <div style="background: #334155; padding: 15px;">
                            <h3 style="color: #60a5fa; margin: 0;">
                                <span class="badge" style="background: ${bandColor}; color: white; font-size: 1.1em; padding: 6px 14px;">${band}</span>
                            </h3>
                        </div>
                        <div style="padding: 20px;">
                ` + "`" + `;

                instanceGaps.forEach(gap => {
                    const coverageColor = gap.coverage_rate >= 90 ? '#10b981' : gap.coverage_rate >= 75 ? '#f59e0b' : '#ef4444';
                    const maxDisplay = 20;
                    const displayCycles = gap.missing_cycles.slice(0, maxDisplay);
                    const remaining = gap.missing_cycles.length - maxDisplay;
                    const displayName = gap.instance === 'deduped' ? '📤 Deduped (Sent to WSPRNet)' : ` + "`" + `🖥️ ${gap.instance}` + "`" + `;
                    const gapDataId = ` + "`" + `gapdata_${gap.instance}_${band.replace(/[^a-zA-Z0-9]/g, '_')}` + "`" + `;

                    html += ` + "`" + `
                        <div style="background: #1e293b; padding: 20px; border-radius: 8px; margin-bottom: 15px; border: 1px solid #334155;">
                            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px;">
                                <div>
                                    <span style="color: #e2e8f0; font-weight: 600; font-size: 1.1em;">${displayName}</span>
                                </div>
                                <div style="text-align: right;">
                                    <div style="font-size: 1.5em; font-weight: bold; color: ${coverageColor};">
                                        ${gap.coverage_rate.toFixed(1)}%
                                    </div>
                                    <div style="color: #94a3b8; font-size: 0.85em;">Coverage Rate</div>
                                </div>
                            </div>

                            <div style="display: grid; grid-template-columns: repeat(2, 1fr) auto; gap: 15px; margin-bottom: 15px; align-items: end;">
                                <div>
                                    <div style="color: #94a3b8; font-size: 0.85em;">Missing Cycles</div>
                                    <div style="font-size: 1.3em; font-weight: bold; color: #ef4444;">${gap.gap_count}</div>
                                </div>
                                <div>
                                    <div style="color: #94a3b8; font-size: 0.85em;">Total Expected</div>
                                    <div style="font-size: 1.3em; font-weight: bold; color: #60a5fa;">${gap.total_cycles}</div>
                                </div>
                                <div>
                                    <button class="control-btn" onclick="downloadGapCSV('${gap.instance}', '${band}', '${gapDataId}')" style="white-space: nowrap;">💾 Export CSV</button>
                                </div>
                            </div>
                            <div id="${gapDataId}" style="display: none;">${JSON.stringify(gap.missing_cycles)}</div>

                            <div style="background: #0f172a; padding: 15px; border-radius: 6px; border: 1px solid #334155;">
                                <div style="color: #94a3b8; font-size: 0.9em; margin-bottom: 10px; font-weight: 600;">Coverage Timeline:</div>
                                <div id="timeline_${gap.instance}_${band.replace(/[^a-zA-Z0-9]/g, '_')}" class="gap-timeline" style="min-height: 80px;"></div>
                        </div>
                            </div>
                    ` + "`" + `;
                });

                html += ` + "`" + `
                        </div>
                    </div>
                ` + "`" + `;
            });

            detailsContainer.innerHTML = html;
            
            // Render timelines after HTML is inserted
            renderAllGapTimelines(filteredGaps, hoursBack);
        }

        // Timeline visualization for WSPR cycle gaps
        function renderGapTimeline(containerId, gapData, hoursBack) {
            const container = document.getElementById(containerId);
            if (!container) return;

            // Subtract 4 minutes from now to exclude incomplete WSPR cycles (matches backend)
            let endTime = new Date(Date.now() - (4 * 60 * 1000));
            
            // Round endTime to 2-minute boundary
            const endUnix = Math.floor(endTime.getTime() / 1000);
            const roundedEndUnix = Math.floor(endUnix / 120) * 120;
            endTime = new Date(roundedEndUnix * 1000);
            
            const startTime = new Date(endTime.getTime() - (hoursBack * 60 * 60 * 1000));
            const startMinutes = Math.floor(startTime.getMinutes() / 2) * 2;
            startTime.setMinutes(startMinutes, 0, 0);
            
            const allCycles = [];
            for (let t = new Date(startTime); t <= endTime; t = new Date(t.getTime() + 120000)) {
                allCycles.push(new Date(t));
            }
            
            const missingSet = new Set(gapData.missing_cycles);
            
            let html = '<div style="display: flex; flex-direction: column; gap: 8px;">';
            html += '<div style="display: flex; align-items: center; margin-bottom: 5px;">';
            html += '<div style="width: 80px; flex-shrink: 0;"></div>';
            html += '<div style="flex: 1; position: relative; height: 20px;">';
            
            const hourLabels = [];
            for (let i = 0; i < allCycles.length; i++) {
                const cycle = allCycles[i];
                if (cycle.getMinutes() === 0) {
                    hourLabels.push({
                        index: i,
                        label: cycle.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
                    });
                }
            }
            
            hourLabels.forEach((label) => {
                const position = (label.index / allCycles.length) * 100;
                html += '<span style="position: absolute; left: ' + position + '%; transform: translateX(-50%); font-size: 0.75em; color: #64748b;">' + label.label + '</span>';
            });
            
            html += '</div></div>';
            html += '<div style="display: flex; align-items: center;">';
            html += '<div style="width: 80px; flex-shrink: 0; font-size: 0.85em; color: #94a3b8; text-align: right; padding-right: 10px;">Coverage:</div>';
            html += '<div style="flex: 1; display: flex; gap: 1px; height: 40px; background: #0f172a; border-radius: 4px; overflow: hidden; padding: 2px;">';
            
            allCycles.forEach(cycle => {
                const timeStr = cycle.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
                const isMissing = missingSet.has(timeStr);
                const color = isMissing ? '#ef4444' : '#10b981';
                const title = isMissing ? 'Missing: ' + timeStr : 'Coverage: ' + timeStr;
                html += '<div style="flex: 1; background: ' + color + '; min-width: 2px; border-radius: 2px;" title="' + title + '"></div>';
            });
            
            html += '</div></div>';
            html += '<div style="display: flex; align-items: center; gap: 20px; margin-top: 8px; font-size: 0.85em;">';
            html += '<div style="width: 80px; flex-shrink: 0;"></div>';
            html += '<div style="display: flex; gap: 15px;">';
            html += '<div class="timeline-legend-item" data-type="spots" style="display: flex; align-items: center; gap: 6px; cursor: pointer; user-select: none; padding: 4px 8px; border-radius: 4px; transition: background 0.2s;" onmouseover="this.style.background=\'#1e293b\'" onmouseout="this.style.background=\'transparent\'" onclick="toggleTimelineData(this, \'spots\')">';
            html += '<div style="width: 16px; height: 16px; background: #10b981; border-radius: 3px;"></div>';
            html += '<span style="color: #94a3b8;">Spots Received</span>';
            html += '</div>';
            html += '<div class="timeline-legend-item" data-type="gaps" style="display: flex; align-items: center; gap: 6px; cursor: pointer; user-select: none; padding: 4px 8px; border-radius: 4px; transition: background 0.2s;" onmouseover="this.style.background=\'#1e293b\'" onmouseout="this.style.background=\'transparent\'" onclick="toggleTimelineData(this, \'gaps\')">';
            html += '<div style="width: 16px; height: 16px; background: #ef4444; border-radius: 3px;"></div>';
            html += '<span style="color: #94a3b8;">Missing Cycles</span>';
            html += '</div>';
            html += '</div></div></div>';
            
            container.innerHTML = html;
        }

        function renderAllGapTimelines(gapsData, hoursBack) {
            setTimeout(() => {
                Object.entries(gapsData).forEach(([instance, bandGaps]) => {
                    bandGaps.forEach(gap => {
                        const timelineId = 'timeline_' + gap.instance + '_' + gap.band.replace(/[^a-zA-Z0-9]/g, '_');
                        renderGapTimeline(timelineId, gap, hoursBack);
                    });
                });
            }, 100);
        }

        // Download gap data as CSV
        function downloadGapCSV(instance, band, dataId) {
            const dataElement = document.getElementById(dataId);
            if (!dataElement) {
                console.error('Gap data not found');
                return;
            }

            const missingCycles = JSON.parse(dataElement.textContent);
            if (!missingCycles || missingCycles.length === 0) {
                alert('No missing cycles to export');
                return;
            }

            // Get current date for the time range
            const timeFilter = parseInt(document.getElementById('gapsTimeFilter').value);
            const endTime = new Date(Date.now() - (4 * 60 * 1000)); // Subtract 4 minutes (matches backend)
            const startTime = new Date(endTime.getTime() - (timeFilter * 60 * 60 * 1000));

            // Create CSV content
            let csv = 'Instance,Band,Missing Cycle Time,Date,Time Range\n';
            const dateStr = new Date().toISOString().split('T')[0];
            const timeRange = ` + "`" + `${startTime.toISOString()} to ${endTime.toISOString()}` + "`" + `;

            missingCycles.forEach(cycle => {
                // Parse the cycle time (format: "HH:MM")
                const displayInstance = instance === 'deduped' ? 'Deduped (Sent to WSPRNet)' : instance;
                csv += ` + "`" + `"${displayInstance}","${band}","${cycle}","${dateStr}","${timeRange}"\n` + "`" + `;
            });

            // Create and download the file
            const blob = new Blob([csv], { type: 'text/csv' });
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = ` + "`" + `wspr_gaps_${instance}_${band}_${dateStr}.csv` + "`" + `;
            a.click();
            window.URL.revokeObjectURL(url);
        }

        // Toggle timeline data visibility (spots or gaps)
        function toggleTimelineData(element, type) {
            const container = element.closest('.gap-timeline').parentElement;
            const timelineDiv = container.querySelector('.gap-timeline');
            const allBars = timelineDiv.querySelectorAll('[style*="flex: 1"]');
            
            // Toggle active state
            element.classList.toggle('timeline-hidden');
            
            // Update visual state
            if (element.classList.contains('timeline-hidden')) {
                element.style.opacity = '0.3';
                element.style.textDecoration = 'line-through';
            } else {
                element.style.opacity = '1';
                element.style.textDecoration = 'none';
            }
            
            // Get current visibility states
            const legendItems = container.querySelectorAll('.timeline-legend-item');
            const spotsHidden = Array.from(legendItems).find(item => item.dataset.type === 'spots')?.classList.contains('timeline-hidden');
            const gapsHidden = Array.from(legendItems).find(item => item.dataset.type === 'gaps')?.classList.contains('timeline-hidden');
            
            // Update bar visibility
            allBars.forEach(bar => {
                const isGap = bar.style.background.includes('#ef4444') || bar.style.background.includes('rgb(239, 68, 68)');
                const isSpot = bar.style.background.includes('#10b981') || bar.style.background.includes('rgb(16, 185, 129)');
                
                if ((isGap && gapsHidden) || (isSpot && spotsHidden)) {
                    bar.style.opacity = '0';
                    bar.style.visibility = 'hidden';
                } else {
                    bar.style.opacity = '1';
                    bar.style.visibility = 'visible';
                }
            });
        }

        function initGapsTab() {
            // Add event listener for time filter
            document.getElementById('gapsTimeFilter').addEventListener('change', loadGaps);
            
            // Add event listener for instance filter
            document.getElementById('gapsInstanceFilter').addEventListener('change', filterGapsByInstance);
            
            // Add event listener for gap type filter
            document.getElementById('gapsTypeFilter').addEventListener('change', filterGapsByInstance);
            
            // Initial load
            loadGaps();
        }

        // Initialize map and filters on load
        initMap();
        initBandFilters();
        initSpotsTab();
        initGapsTab();

        // Initial load
        fetchData();

        // Auto-refresh every 120 seconds
        setInterval(fetchData, 120000);
        
        // Auto-refresh spots tab every 120 seconds if active
        setInterval(() => {
            const spotsTab = document.getElementById('spots');
            if (spotsTab && spotsTab.classList.contains('active')) {
                loadSpots();
            }
        }, 120000);

        // Auto-refresh gaps tab every 120 seconds if active
        setInterval(() => {
            const gapsTab = document.getElementById('gaps');
            if (gapsTab && gapsTab.classList.contains('active')) {
                loadGaps();
            }
        }, 120000);
    </script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}

// handleRawSpots returns raw spots from instances with optional filters
func (ws *WebServer) handleRawSpots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if ws.spotWriter == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Spot writer not initialized",
			"spots": []interface{}{},
		})
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	instance := query.Get("instance")
	band := query.Get("band")
	startTimeStr := query.Get("start_time")
	endTimeStr := query.Get("end_time")

	var startTime, endTime time.Time
	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = t
		}
	}

	spots := ws.spotWriter.GetRawSpots(instance, band, startTime, endTime)
	_ = json.NewEncoder(w).Encode(spots)
}

// handleDedupedSpots returns deduped spots with optional filters
func (ws *WebServer) handleDedupedSpots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if ws.spotWriter == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Spot writer not initialized",
			"spots": []interface{}{},
		})
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	band := query.Get("band")
	startTimeStr := query.Get("start_time")
	endTimeStr := query.Get("end_time")
	submittedStr := query.Get("submitted")

	var startTime, endTime time.Time
	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = t
		}
	}

	var submittedOnly *bool
	if submittedStr != "" {
		val := submittedStr == "true"
		submittedOnly = &val
	}

	spots := ws.spotWriter.GetDedupedSpots(band, startTime, endTime, submittedOnly)
	_ = json.NewEncoder(w).Encode(spots)
}

// handleSpotInstances returns list of instance names that have spots
func (ws *WebServer) handleSpotInstances(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if ws.spotWriter == nil {
		_ = json.NewEncoder(w).Encode([]string{})
		return
	}

	instances := ws.spotWriter.GetInstanceNames()
	_ = json.NewEncoder(w).Encode(instances)
}

// handleSpotGaps returns gap analysis showing missing WSPR cycles
func (ws *WebServer) handleSpotGaps(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if ws.spotWriter == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Spot writer not initialized",
			"gaps":  map[string]interface{}{},
		})
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	hoursBackStr := query.Get("hours")
	hoursBack := 24 // default to 24 hours
	if hoursBackStr != "" {
		if h, err := time.ParseDuration(hoursBackStr + "h"); err == nil {
			hoursBack = int(h.Hours())
		}
	}

	gaps := ws.spotWriter.AnalyzeGaps(hoursBack)
	_ = json.NewEncoder(w).Encode(gaps)
}
