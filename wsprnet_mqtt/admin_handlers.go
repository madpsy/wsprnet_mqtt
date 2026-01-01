package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// AdminHandler handles admin-related HTTP requests
type AdminHandler struct {
	config         *Config
	configFile     string
	sessionManager *SessionManager
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(config *Config, configFile string) *AdminHandler {
	return &AdminHandler{
		config:         config,
		configFile:     configFile,
		sessionManager: NewSessionManager(),
	}
}

// IsAdminEnabled checks if admin access is enabled
func (ah *AdminHandler) IsAdminEnabled() bool {
	return ah.config.AdminPassword != ""
}

// AuthMiddleware checks if the user is authenticated
func (ah *AdminHandler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if admin is enabled
		if !ah.IsAdminEnabled() {
			http.Error(w, "Admin access is not enabled", http.StatusForbidden)
			return
		}

		// Get session cookie
		cookie, err := r.Cookie("admin_session")
		if err != nil {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}

		// Validate session
		if !ah.sessionManager.ValidateSession(cookie.Value) {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

// HandleAdminLogin handles the admin login page
func (ah *AdminHandler) HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if !ah.IsAdminEnabled() {
		http.Error(w, "Admin access is not enabled. Set admin_password in config.yaml to enable.", http.StatusForbidden)
		return
	}

	if r.Method == "GET" {
		ah.serveLoginPage(w)
		return
	}

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		password := r.FormValue("password")
		if password == ah.config.AdminPassword {
			// Create session
			token := ah.sessionManager.CreateSession()

			// Set cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "admin_session",
				Value:    token,
				Path:     "/",
				MaxAge:   86400, // 24 hours
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})

			http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
			return
		}

		// Invalid password
		ah.serveLoginPage(w)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// HandleAdminLogout handles admin logout
func (ah *AdminHandler) HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie("admin_session")
	if err == nil {
		// Delete session
		ah.sessionManager.DeleteSession(cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// HandleAdminDashboard serves the admin dashboard
func (ah *AdminHandler) HandleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	html := ah.getAdminDashboardHTML()
	w.Write([]byte(html))
}

// HandleGetConfig returns the current configuration
func (ah *AdminHandler) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ah.config)
}

// HandleUpdateConfig updates the configuration
func (ah *AdminHandler) HandleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var newConfig Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse config: %v", err), http.StatusBadRequest)
		return
	}

	// Validate new config
	if err := newConfig.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid configuration: %v", err), http.StatusBadRequest)
		return
	}

	// Save to file
	data, err := yaml.Marshal(&newConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal config: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(ah.configFile, data, 0644); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write config file: %v", err), http.StatusInternalServerError)
		return
	}

	// Update in-memory config
	*ah.config = newConfig

	log.Println("Configuration updated via admin interface - triggering application restart")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Configuration saved successfully. Application will restart in 2 seconds...",
	})

	// Trigger application exit after a short delay to allow response to be sent
	go func() {
		time.Sleep(2 * time.Second)
		log.Println("Exiting application for restart after config update")
		os.Exit(0)
	}()
}

// HandleExportConfig exports the current configuration as a YAML file
func (ah *AdminHandler) HandleExportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the config file
	data, err := os.ReadFile(ah.configFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read config file: %v", err), http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=config.yaml")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write the file content
	if _, err := w.Write(data); err != nil {
		log.Printf("Error writing config export: %v", err)
	}
}

// HandleImportConfig imports a configuration from an uploaded YAML file
func (ah *AdminHandler) HandleImportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (10MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
		return
	}

	// Get the uploaded file
	file, _, err := r.FormFile("config")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get uploaded file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read file content
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read uploaded file: %v", err), http.StatusInternalServerError)
		return
	}

	// Parse and validate the configuration
	var newConfig Config
	if err := yaml.Unmarshal(data, &newConfig); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse config file: %v", err), http.StatusBadRequest)
		return
	}

	// Validate new config
	if err := newConfig.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid configuration: %v", err), http.StatusBadRequest)
		return
	}

	// Save to file
	if err := os.WriteFile(ah.configFile, data, 0644); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write config file: %v", err), http.StatusInternalServerError)
		return
	}

	// Update in-memory config
	*ah.config = newConfig

	log.Println("Configuration imported via admin interface - triggering application restart")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Configuration imported successfully. Application will restart in 2 seconds...",
	}); err != nil {
		log.Printf("Error encoding import response: %v", err)
	}

	// Trigger application exit after a short delay to allow response to be sent
	go func() {
		time.Sleep(2 * time.Second)
		log.Println("Exiting application for restart after config import")
		os.Exit(0)
	}()
}

// HandleSyncKiwis previews or applies sync of MQTT instances from kiwi_wspr config
func (ah *AdminHandler) HandleSyncKiwis(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if this is a preview or apply request
	var request struct {
		Apply bool `json:"apply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		// If no body, treat as preview request
		request.Apply = false
	}

	// Try to load kiwi_wspr config
	kiwiConfig, err := LoadKiwiWSPRConfig("/app/kiwi_wspr_data/config.yaml")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load kiwi_wspr config: %v", err), http.StatusInternalServerError)
		return
	}

	// Build a map of existing instances by name and topic_prefix for quick lookup
	existingByName := make(map[string]*InstanceConfig)
	existingByTopic := make(map[string]*InstanceConfig)
	for i := range ah.config.MQTT.Instances {
		inst := &ah.config.MQTT.Instances[i]
		existingByName[inst.Name] = inst
		existingByTopic[inst.TopicPrefix] = inst
	}

	// Track changes
	type Change struct {
		Type        string `json:"type"` // "add" or "update"
		Name        string `json:"name"`
		TopicPrefix string `json:"topic_prefix"`
		OldName     string `json:"old_name,omitempty"`
		OldTopic    string `json:"old_topic,omitempty"`
	}
	var changes []Change

	// Process each enabled kiwi instance
	for _, kiwiInst := range kiwiConfig.KiwiInstances {
		if !kiwiInst.Enabled {
			continue
		}

		// Get the MQTT topic prefix for this instance
		topicPrefix := kiwiConfig.GetMQTTTopicPrefix(kiwiInst.Name)

		// Check if instance exists by name or topic
		var existingInst *InstanceConfig
		if inst, ok := existingByName[kiwiInst.Name]; ok {
			existingInst = inst
		} else if inst, ok := existingByTopic[topicPrefix]; ok {
			existingInst = inst
		}

		if existingInst != nil {
			// Check if update is needed
			if existingInst.Name != kiwiInst.Name || existingInst.TopicPrefix != topicPrefix {
				changes = append(changes, Change{
					Type:        "update",
					Name:        kiwiInst.Name,
					TopicPrefix: topicPrefix,
					OldName:     existingInst.Name,
					OldTopic:    existingInst.TopicPrefix,
				})
			}
		} else {
			// New instance
			changes = append(changes, Change{
				Type:        "add",
				Name:        kiwiInst.Name,
				TopicPrefix: topicPrefix,
			})
		}
	}

	// If no changes, return early
	if len(changes) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "No changes needed - all instances are already in sync",
			"changes": []Change{},
		})
		return
	}

	// If this is just a preview, return the changes
	if !request.Apply {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "preview",
			"message": fmt.Sprintf("Found %d change(s) to apply", len(changes)),
			"changes": changes,
		})
		return
	}

	// Apply the changes
	addedCount := 0
	updatedCount := 0

	for _, change := range changes {
		if change.Type == "add" {
			ah.config.MQTT.Instances = append(ah.config.MQTT.Instances, InstanceConfig{
				Name:        change.Name,
				TopicPrefix: change.TopicPrefix,
			})
			addedCount++
		} else if change.Type == "update" {
			// Find and update the instance
			for i := range ah.config.MQTT.Instances {
				inst := &ah.config.MQTT.Instances[i]
				if inst.Name == change.OldName || inst.TopicPrefix == change.OldTopic {
					inst.Name = change.Name
					inst.TopicPrefix = change.TopicPrefix
					updatedCount++
					break
				}
			}
		}
	}

	// Save updated config to file
	data, err := yaml.Marshal(ah.config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal config: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(ah.configFile, data, 0644); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write config file: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Synced kiwi instances: %d added, %d updated", addedCount, updatedCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Configuration saved successfully. Application will restart in 2 seconds..."),
		"added":   addedCount,
		"updated": updatedCount,
	})

	// Trigger application exit after a short delay to allow response to be sent
	go func() {
		time.Sleep(2 * time.Second)
		log.Println("Exiting application for restart after kiwi sync")
		os.Exit(0)
	}()
}

// serveLoginPage serves the login HTML page
func (ah *AdminHandler) serveLoginPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Admin Login - WSPR MQTT Aggregator</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .login-container {
            background: white;
            padding: 40px;
            border-radius: 12px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.3);
            width: 100%;
            max-width: 400px;
        }
        h1 {
            color: #333;
            margin-bottom: 10px;
            font-size: 24px;
        }
        .subtitle {
            color: #666;
            margin-bottom: 30px;
            font-size: 14px;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            margin-bottom: 8px;
            color: #333;
            font-weight: 600;
        }
        input[type="password"] {
            width: 100%;
            padding: 12px;
            border: 2px solid #e2e8f0;
            border-radius: 8px;
            font-size: 16px;
            transition: border-color 0.3s;
        }
        input[type="password"]:focus {
            outline: none;
            border-color: #667eea;
        }
        button {
            width: 100%;
            padding: 12px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s;
        }
        button:hover {
            transform: translateY(-2px);
        }
        button:active {
            transform: translateY(0);
        }
        .back-link {
            text-align: center;
            margin-top: 20px;
        }
        .back-link a {
            color: #667eea;
            text-decoration: none;
            font-size: 14px;
        }
        .back-link a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <h1>🔐 Admin Login</h1>
        <p class="subtitle">WSPR MQTT Aggregator</p>
        <form method="POST" action="/admin/login">
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required autofocus>
            </div>
            <button type="submit">Login</button>
        </form>
        <div class="back-link">
            <a href="/">← Back to Dashboard</a>
        </div>
    </div>
</body>
</html>`
	w.Write([]byte(html))
}

// getAdminDashboardHTML returns the admin dashboard HTML
func (ah *AdminHandler) getAdminDashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Admin Dashboard - WSPR MQTT Aggregator</title>
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
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 30px;
            border-radius: 12px;
            margin-bottom: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.3);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        h1 {
            font-size: 2em;
        }
        .logout-btn {
            padding: 10px 20px;
            background: rgba(255,255,255,0.2);
            color: white;
            border: 2px solid white;
            border-radius: 8px;
            cursor: pointer;
            font-weight: 600;
            text-decoration: none;
            transition: all 0.2s;
        }
        .logout-btn:hover {
            background: rgba(255,255,255,0.3);
        }
        .container {
            background: #1e293b;
            padding: 30px;
            border-radius: 12px;
            border: 1px solid #334155;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        .section-title {
            font-size: 1.5em;
            margin-bottom: 20px;
            color: #60a5fa;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            margin-bottom: 8px;
            color: #94a3b8;
            font-weight: 600;
        }
        input[type="text"],
        input[type="number"],
        input[type="password"] {
            width: 100%;
            padding: 12px;
            background: #0f172a;
            border: 2px solid #334155;
            border-radius: 8px;
            color: #e2e8f0;
            font-size: 14px;
        }
        input:focus {
            outline: none;
            border-color: #60a5fa;
        }
        .checkbox-group {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        input[type="checkbox"] {
            width: 20px;
            height: 20px;
            cursor: pointer;
        }
        .btn {
            padding: 12px 24px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s;
            margin-right: 10px;
        }
        .btn:hover {
            transform: translateY(-2px);
        }
        .btn-secondary {
            background: #334155;
        }
        .btn-danger {
            background: #ef4444;
        }
        .instance-list {
            margin-top: 20px;
        }
        .instance-item {
            background: #0f172a;
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 10px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            border: 1px solid #334155;
        }
        .instance-info {
            flex: 1;
        }
        .instance-name {
            font-weight: 600;
            color: #60a5fa;
            margin-bottom: 5px;
        }
        .instance-prefix {
            color: #94a3b8;
            font-size: 0.9em;
        }
        .message {
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 20px;
            display: none;
        }
        .message.success {
            background: #10b981;
            color: white;
        }
        .message.error {
            background: #ef4444;
            color: white;
        }
        .grid-2col {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
        }
        @media (max-width: 768px) {
            .grid-2col {
                grid-template-columns: 1fr;
            }
        }
        .back-link {
            text-align: center;
            margin-top: 20px;
        }
        .back-link a {
            color: #60a5fa;
            text-decoration: none;
        }
        .back-link a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="header">
        <div>
            <h1>⚙️ Admin Dashboard</h1>
            <div style="opacity: 0.9; margin-top: 5px;">WSPR MQTT Aggregator Configuration</div>
        </div>
        <a href="/admin/logout" class="logout-btn">Logout</a>
    </div>

    <div id="message" class="message"></div>

    <div class="container">
        <h2 class="section-title">Receiver Configuration</h2>
        <div class="grid-2col">
            <div class="form-group">
                <label for="callsign">Callsign</label>
                <input type="text" id="callsign" placeholder="e.g., W1ABC">
            </div>
            <div class="form-group">
                <label for="locator">Locator</label>
                <input type="text" id="locator" placeholder="e.g., FN42">
            </div>
        </div>
    </div>

    <div class="container">
        <h2 class="section-title">
            MQTT Configuration
            <span id="mqtt-status-indicator" style="margin-left: 15px; font-size: 0.8em; font-weight: normal;">
                <span style="color: #6c757d; font-size: 20px;">●</span> Unknown
            </span>
        </h2>
        <div class="form-group">
            <label for="broker">Broker URL</label>
            <input type="text" id="broker" placeholder="e.g., tcp://localhost:1883">
        </div>
        <div class="grid-2col">
            <div class="form-group">
                <label for="username">Username (optional)</label>
                <input type="text" id="username">
            </div>
            <div class="form-group">
                <label for="password">Password (optional)</label>
                <input type="password" id="password">
            </div>
        </div>
        <div class="form-group">
            <label for="qos">QoS Level</label>
            <input type="number" id="qos" min="0" max="2" value="0">
        </div>
        <div class="form-group">
            <button class="btn btn-secondary" onclick="testMQTT()">🔌 Test MQTT Connection</button>
        </div>
    </div>

    <div class="container">
        <h2 class="section-title">MQTT Instances</h2>
        <div id="instanceList" class="instance-list"></div>
        <button class="btn" onclick="addInstance()">+ Add Instance</button>
        <button class="btn btn-secondary" onclick="syncKiwis()">🔄 Sync Kiwis</button>
    </div>

    <div class="container">
        <h2 class="section-title">Other Settings</h2>
        <div class="grid-2col">
            <div class="form-group">
                <label for="webPort">Web Port</label>
                <input type="number" id="webPort" value="9009">
            </div>
            <div class="form-group">
                <label for="persistenceFile">Persistence File</label>
                <input type="text" id="persistenceFile" value="wsprnet_stats.json">
            </div>
        </div>
        <div class="form-group checkbox-group">
            <input type="checkbox" id="dryRun">
            <label for="dryRun" style="margin-bottom: 0;">Dry Run Mode (don't send to WSPRNet)</label>
        </div>
        <div class="form-group">
            <label for="adminPassword">Admin Password</label>
            <input type="password" id="adminPassword" placeholder="Leave empty to disable admin access">
        </div>
    </div>

    <div class="container">
        <button class="btn" onclick="saveConfig()">💾 Save Configuration</button>
        <button class="btn btn-secondary" onclick="loadConfig()">🔄 Reload</button>
        <button class="btn btn-secondary" onclick="exportConfig()">📥 Export Config</button>
        <button class="btn btn-secondary" onclick="importConfig()">📤 Import Config</button>
    </div>

    <div class="container">
        <h2 class="section-title">⚠️ Danger Zone</h2>
        <p style="color: #94a3b8; margin-bottom: 20px;">
            These actions cannot be undone. Use with caution.
        </p>
        <button class="btn btn-danger" onclick="clearAllStatistics()">🗑️ Delete All Statistics</button>
    </div>

    <div class="back-link">
        <a href="/">← Back to Main Dashboard</a>
    </div>

    <script>
        let config = {};

        // Load configuration on page load
        window.addEventListener('DOMContentLoaded', loadConfig);

        async function loadConfig() {
            try {
                const response = await fetch('/admin/api/config');
                config = await response.json();
                
                // Populate form fields
                document.getElementById('callsign').value = config.receiver.callsign || '';
                document.getElementById('locator').value = config.receiver.locator || '';
                document.getElementById('broker').value = config.mqtt.broker || '';
                document.getElementById('username').value = config.mqtt.username || '';
                document.getElementById('password').value = config.mqtt.password || '';
                document.getElementById('qos').value = config.mqtt.qos || 0;
                document.getElementById('webPort').value = config.web_port || 9009;
                document.getElementById('persistenceFile').value = config.persistence_file || 'wsprnet_stats.json';
                document.getElementById('dryRun').checked = config.dry_run || false;
                document.getElementById('adminPassword').value = config.admin_password || '';
                
                // Render instances
                renderInstances();
            } catch (error) {
                showMessage('Failed to load configuration: ' + error.message, 'error');
            }
        }

        let mqttStatus = null;

        function renderInstances(forceRebuild = false) {
            const container = document.getElementById('instanceList');
            
            if (!config.mqtt.instances || config.mqtt.instances.length === 0) {
                container.innerHTML = '<p style="color: #94a3b8;">No instances configured</p>';
                return;
            }
            
            // Rebuild if forced or container is empty (first render)
            if (forceRebuild || container.children.length === 0) {
                container.innerHTML = '';
                config.mqtt.instances.forEach((instance, index) => {
                    const div = document.createElement('div');
                    div.className = 'instance-item';
                    div.id = 'instance-' + index;
                    
                    // Get current message count if available
                    const msgCount = (mqttStatus && mqttStatus.instance_counts)
                        ? (mqttStatus.instance_counts[instance.name] || 0).toLocaleString()
                        : '0';
                    
                    div.innerHTML = ` + "`" + `
                        <div class="instance-info">
                            <div class="instance-name">${instance.name}</div>
                            <div class="instance-prefix">Topic Prefix: ${instance.topic_prefix}</div>
                            <div class="instance-prefix" style="color: #60a5fa; margin-top: 5px;">
                                Messages: <span id="msg-count-${index}">${msgCount}</span>
                            </div>
                        </div>
                        <div>
                            <button class="btn btn-secondary" onclick="editInstance(${index})">Edit</button>
                            <button class="btn btn-danger" onclick="deleteInstance(${index})">Delete</button>
                        </div>
                    ` + "`" + `;
                    container.appendChild(div);
                });
            } else {
                // Just update message counts without rebuilding DOM
                config.mqtt.instances.forEach((instance, index) => {
                    const msgCountEl = document.getElementById('msg-count-' + index);
                    if (msgCountEl && mqttStatus && mqttStatus.instance_counts) {
                        const count = mqttStatus.instance_counts[instance.name] || 0;
                        msgCountEl.textContent = count.toLocaleString();
                    }
                });
            }
        }

        function addInstance() {
            const name = prompt('Instance name:');
            if (!name) return;
            
            const topicPrefix = prompt('Topic prefix:');
            if (!topicPrefix) return;
            
            if (!config.mqtt.instances) {
                config.mqtt.instances = [];
            }
            
            config.mqtt.instances.push({
                name: name,
                topic_prefix: topicPrefix
            });
            
            renderInstances(true);
        }

        function editInstance(index) {
            const instance = config.mqtt.instances[index];
            
            const name = prompt('Instance name:', instance.name);
            if (name === null) return;
            
            const topicPrefix = prompt('Topic prefix:', instance.topic_prefix);
            if (topicPrefix === null) return;
            
            config.mqtt.instances[index] = {
                name: name,
                topic_prefix: topicPrefix
            };
            
            renderInstances(true);
        }

        function deleteInstance(index) {
            if (!confirm('Are you sure you want to delete this instance?')) return;
            
            config.mqtt.instances.splice(index, 1);
            renderInstances(true);
        }

        async function saveConfig() {
            // Show confirmation dialog
            if (!confirm('⚠️ Warning: Saving the configuration will restart the application.\n\nDo you want to continue?')) {
                return;
            }

            // Build config object from form
            const newConfig = {
                receiver: {
                    callsign: document.getElementById('callsign').value,
                    locator: document.getElementById('locator').value
                },
                mqtt: {
                    broker: document.getElementById('broker').value,
                    username: document.getElementById('username').value,
                    password: document.getElementById('password').value,
                    qos: parseInt(document.getElementById('qos').value),
                    instances: config.mqtt.instances || []
                },
                web_port: parseInt(document.getElementById('webPort').value),
                dry_run: document.getElementById('dryRun').checked,
                persistence_file: document.getElementById('persistenceFile').value,
                admin_password: document.getElementById('adminPassword').value
            };
            
            try {
                const response = await fetch('/admin/api/config', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(newConfig)
                });
                
                if (!response.ok) {
                    const error = await response.text();
                    throw new Error(error);
                }
                
                const result = await response.json();
                
                // Show countdown overlay
                showRestartCountdown();
            } catch (error) {
                showMessage('Failed to save configuration: ' + error.message, 'error');
            }
        }

        function showRestartCountdown() {
            // Create overlay
            const overlay = document.createElement('div');
            overlay.style.cssText = 'position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0, 0, 0, 0.9); display: flex; align-items: center; justify-content: center; z-index: 9999; animation: fadeIn 0.3s;';
            
            const content = document.createElement('div');
            content.style.cssText = 'background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 40px 60px; border-radius: 16px; text-align: center; box-shadow: 0 20px 60px rgba(0,0,0,0.5);';
            
            const icon = document.createElement('div');
            icon.style.cssText = 'font-size: 64px; margin-bottom: 20px; animation: pulse 1s infinite;';
            icon.textContent = '🔄';
            
            const title = document.createElement('h2');
            title.style.cssText = 'font-size: 28px; margin-bottom: 10px; color: white;';
            title.textContent = 'Configuration Saved';
            
            const message = document.createElement('p');
            message.style.cssText = 'font-size: 18px; margin-bottom: 20px; color: rgba(255,255,255,0.9);';
            message.textContent = 'Application restarting in';
            
            const countdown = document.createElement('div');
            countdown.style.cssText = 'font-size: 72px; font-weight: bold; color: white; margin: 20px 0; font-family: monospace;';
            countdown.textContent = '5';
            
            content.appendChild(icon);
            content.appendChild(title);
            content.appendChild(message);
            content.appendChild(countdown);
            overlay.appendChild(content);
            document.body.appendChild(overlay);
            
            // Add animations
            const style = document.createElement('style');
            style.textContent = '@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } } @keyframes pulse { 0%, 100% { transform: scale(1); } 50% { transform: scale(1.1); } }';
            document.head.appendChild(style);
            
            // Countdown
            let count = 5;
            const interval = setInterval(() => {
                count--;
                countdown.textContent = count;
                
                if (count === 0) {
                    clearInterval(interval);
                    countdown.textContent = '0';
                    message.textContent = 'Restarting now...';
                    
                    // Refresh page after a short delay
                    setTimeout(() => {
                        window.location.reload();
                    }, 1000);
                }
            }, 1000);
        }

        function showMessage(text, type) {
            const messageDiv = document.getElementById('message');
            messageDiv.textContent = text;
            messageDiv.className = 'message ' + type;
            messageDiv.style.display = 'block';
            
            setTimeout(() => {
                messageDiv.style.display = 'none';
            }, 5000);
        }

        async function testMQTT() {
            // Get current MQTT settings from UI
            const mqttConfig = {
                broker: document.getElementById('broker').value,
                username: document.getElementById('username').value,
                password: document.getElementById('password').value,
                qos: parseInt(document.getElementById('qos').value)
            };

            // Validate required fields
            if (!mqttConfig.broker) {
                showMessage('❌ Please enter MQTT broker URL', 'error');
                return;
            }

            // Show testing message
            showMessage('🔄 Testing MQTT connection...', 'success');
            
            // Update status indicator to show testing
            const statusEl = document.getElementById('mqtt-status-indicator');
            statusEl.innerHTML = '<span style="color: #f59e0b; font-size: 20px;">●</span> Testing...';

            try {
                const response = await fetch('/admin/api/mqtt/test', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(mqttConfig)
                });

                const result = await response.json();

                if (result.success) {
                    showMessage(result.message, 'success');
                    statusEl.innerHTML = '<span style="color: #28a745; font-size: 20px;">●</span> Connected';
                } else {
                    showMessage(result.message, 'error');
                    statusEl.innerHTML = '<span style="color: #dc3545; font-size: 20px;">●</span> Failed';
                }
            } catch (e) {
                showMessage('❌ Error testing MQTT: ' + e.message, 'error');
                statusEl.innerHTML = '<span style="color: #dc3545; font-size: 20px;">●</span> Error';
            }
        }

        // Poll MQTT status periodically
        async function updateMQTTStatus() {
            try {
                const response = await fetch('/api/mqtt/status');
                mqttStatus = await response.json();
                
                // Update MQTT status indicator
                const statusEl = document.getElementById('mqtt-status-indicator');
                if (mqttStatus.connected) {
                    statusEl.innerHTML = '<span style="color: #28a745; font-size: 20px;">●</span> Connected';
                } else {
                    statusEl.innerHTML = '<span style="color: #dc3545; font-size: 20px;">●</span> Disconnected';
                }
                
                // Update instance message counts
                renderInstances();
            } catch (error) {
                console.error('Failed to update MQTT status:', error);
            }
        }

        // Clear all statistics
        async function clearAllStatistics() {
            // Show confirmation dialog with strong warning
            if (!confirm('⚠️ WARNING: This will permanently delete ALL statistics!\n\n' +
                         'This includes:\n' +
                         '• All spot history\n' +
                         '• Instance performance data\n' +
                         '• SNR history\n' +
                         '• Country statistics\n' +
                         '• WSPRNet submission counts\n\n' +
                         'This action CANNOT be undone!\n\n' +
                         'Are you absolutely sure you want to continue?')) {
                return;
            }

            // Second confirmation
            if (!confirm('Are you REALLY sure? This will delete everything and start fresh.')) {
                return;
            }

            try {
                const response = await fetch('/admin/api/stats/clear', {
                    method: 'POST'
                });

                if (!response.ok) {
                    const error = await response.text();
                    throw new Error(error);
                }

                const result = await response.json();
                showMessage('✅ ' + result.message + ' - Refreshing page...', 'success');

                // Refresh the page after a short delay
                setTimeout(() => {
                    window.location.reload();
                }, 2000);
            } catch (error) {
                showMessage('❌ Failed to clear statistics: ' + error.message, 'error');
            }
        }

        // Export configuration to YAML file
        async function exportConfig() {
            try {
                const response = await fetch('/admin/api/config/export');
                if (!response.ok) {
                    throw new Error('Failed to export configuration');
                }

                const blob = await response.blob();
                const url = window.URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;

                // Generate filename with timestamp
                const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);
                a.download = 'wsprnet-config-' + timestamp + '.yaml';

                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                window.URL.revokeObjectURL(url);

                showMessage('✅ Configuration exported successfully', 'success');
            } catch (error) {
                showMessage('❌ Failed to export configuration: ' + error.message, 'error');
            }
        }

        // Import configuration from YAML file
        function importConfig() {
            // Create file input element
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = '.yaml,.yml,.json';

            input.onchange = async (e) => {
                const file = e.target.files[0];
                if (!file) return;

                // Show confirmation dialog
                if (!confirm('⚠️ Warning: Importing a configuration will replace your current settings and restart the application.\\n\\nDo you want to continue?')) {
                    return;
                }

                try {
                    const formData = new FormData();
                    formData.append('config', file);

                    const response = await fetch('/admin/api/config/import', {
                        method: 'POST',
                        body: formData
                    });

                    if (!response.ok) {
                        const error = await response.text();
                        throw new Error(error);
                    }

                    const result = await response.json();

                    // Show countdown overlay
                    showRestartCountdown();
                } catch (error) {
                    showMessage('❌ Failed to import configuration: ' + error.message, 'error');
                }
            };

            input.click();
        }

        // Sync kiwi instances from kiwi_wspr config
        async function syncKiwis() {
            try {
                showMessage('🔄 Checking for changes...', 'success');

                // First, get preview of changes
                const previewResponse = await fetch('/admin/api/kiwi/sync', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ apply: false })
                });

                if (!previewResponse.ok) {
                    const error = await previewResponse.text();
                    throw new Error(error);
                }

                const preview = await previewResponse.json();

                // If no changes, just show message
                if (preview.changes.length === 0) {
                    showMessage('✅ ' + preview.message, 'success');
                    return;
                }

                // Show modal with changes
                showSyncModal(preview.changes);
            } catch (error) {
                showMessage('❌ Failed to check kiwi instances: ' + error.message, 'error');
            }
        }

        // Show modal with sync changes
        function showSyncModal(changes) {
            // Create modal overlay
            const overlay = document.createElement('div');
            overlay.style.cssText = 'position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0, 0, 0, 0.8); display: flex; align-items: center; justify-content: center; z-index: 9999; animation: fadeIn 0.3s;';

            const modal = document.createElement('div');
            modal.style.cssText = 'background: #1e293b; padding: 30px; border-radius: 12px; max-width: 600px; width: 90%; max-height: 80vh; overflow-y: auto; border: 2px solid #334155;';

            const title = document.createElement('h2');
            title.style.cssText = 'color: #60a5fa; margin-bottom: 20px; font-size: 24px;';
            title.textContent = '🔄 Sync Kiwi Instances';

            const description = document.createElement('p');
            description.style.cssText = 'color: #94a3b8; margin-bottom: 20px;';
            description.textContent = 'The following changes will be applied:';

            const changesList = document.createElement('div');
            changesList.style.cssText = 'margin-bottom: 20px;';

            changes.forEach(change => {
                const changeItem = document.createElement('div');
                changeItem.style.cssText = 'background: #0f172a; padding: 15px; border-radius: 8px; margin-bottom: 10px; border-left: 4px solid ' + (change.type === 'add' ? '#10b981' : '#f59e0b');

                if (change.type === 'add') {
                    changeItem.innerHTML = ` + "`" + `
                        <div style="color: #10b981; font-weight: 600; margin-bottom: 5px;">➕ Add New Instance</div>
                        <div style="color: #e2e8f0;">Name: <strong>${change.name}</strong></div>
                        <div style="color: #94a3b8; font-size: 0.9em;">Topic: ${change.topic_prefix}</div>
                    ` + "`" + `;
                } else {
                    changeItem.innerHTML = ` + "`" + `
                        <div style="color: #f59e0b; font-weight: 600; margin-bottom: 5px;">🔄 Update Instance</div>
                        <div style="color: #e2e8f0;">Name: <span style="color: #ef4444; text-decoration: line-through;">${change.old_name}</span> → <strong>${change.name}</strong></div>
                        <div style="color: #94a3b8; font-size: 0.9em;">Topic: <span style="color: #ef4444; text-decoration: line-through;">${change.old_topic}</span> → ${change.topic_prefix}</div>
                    ` + "`" + `;
                }

                changesList.appendChild(changeItem);
            });

            const warning = document.createElement('div');
            warning.style.cssText = 'background: rgba(239, 68, 68, 0.1); border: 1px solid #ef4444; padding: 15px; border-radius: 8px; margin-bottom: 20px;';
            warning.innerHTML = '<div style="color: #ef4444; font-weight: 600; margin-bottom: 5px;">⚠️ Warning</div><div style="color: #fca5a5;">Saving these changes will restart the application.</div>';

            const buttonContainer = document.createElement('div');
            buttonContainer.style.cssText = 'display: flex; gap: 10px; justify-content: flex-end;';

            const cancelBtn = document.createElement('button');
            cancelBtn.textContent = 'Cancel';
            cancelBtn.className = 'btn btn-secondary';
            cancelBtn.onclick = () => document.body.removeChild(overlay);

            const saveBtn = document.createElement('button');
            saveBtn.textContent = '💾 Save & Restart';
            saveBtn.className = 'btn';
            saveBtn.onclick = async () => {
                document.body.removeChild(overlay);
                await applySyncChanges();
            };

            buttonContainer.appendChild(cancelBtn);
            buttonContainer.appendChild(saveBtn);

            modal.appendChild(title);
            modal.appendChild(description);
            modal.appendChild(changesList);
            modal.appendChild(warning);
            modal.appendChild(buttonContainer);
            overlay.appendChild(modal);
            document.body.appendChild(overlay);
        }

        // Apply sync changes
        async function applySyncChanges() {
            try {
                showMessage('🔄 Applying changes...', 'success');

                const response = await fetch('/admin/api/kiwi/sync', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ apply: true })
                });

                if (!response.ok) {
                    const error = await response.text();
                    throw new Error(error);
                }

                const result = await response.json();

                // Show countdown overlay
                showRestartCountdown();
            } catch (error) {
                showMessage('❌ Failed to apply changes: ' + error.message, 'error');
            }
        }

        // Start status polling on page load
        window.addEventListener('DOMContentLoaded', function() {
            // Initial status update
            updateMQTTStatus();
            // Poll every 5 seconds
            setInterval(updateMQTTStatus, 5000);
        });
    </script>
</body>
</html>`
}
