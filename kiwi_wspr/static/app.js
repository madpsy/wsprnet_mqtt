let config = {};
let statusData = null;
let kiwiStatusData = {};
let userToBandMapping = {};
let usersModalInterval = null;
let currentModalInstance = null;
let activeUsersData = {};

async function loadConfig() {
    try {
        const response = await fetch('/api/config');
        config = await response.json();
        console.log('Loaded config:', config);
        updateUI();
    } catch (e) {
        showAlert('Failed to load configuration: ' + e.message, 'error');
    }
}

async function loadStatus() {
    try {
        const response = await fetch('/api/status');
        statusData = await response.json();
        updateStatusIndicators();
        updateBandConnectionStatus();
    } catch (e) {
        console.error('Failed to load status:', e);
    }
}

async function loadKiwiStatus() {
    try {
        const response = await fetch('/api/kiwi/status');
        kiwiStatusData = await response.json();
        updateInstanceHeaders();
    } catch (e) {
        console.error('Failed to load KiwiSDR status:', e);
    }
}

async function loadActiveUsers() {
    try {
        const response = await fetch('/api/kiwi/users');
        activeUsersData = await response.json();
        updateBandConnectionStatus();
    } catch (e) {
        console.error('Failed to load active users:', e);
    }
}

async function loadUserMapping() {
    try {
        const response = await fetch('/api/kiwi/user-mapping');
        userToBandMapping = await response.json();
    } catch (e) {
        console.error('Failed to load user mapping:', e);
    }
}

function updateStatusIndicators() {
    if (!statusData) return;
    
    // Update MQTT status indicator
    updateMQTTStatus();
    
    // Update band status indicators
    updateBandStatuses();
}

function updateMQTTStatus() {
    const mqttStatusEl = document.getElementById('mqtt-status-indicator');
    if (!mqttStatusEl) return;
    
    const isConnected = statusData.mqtt && statusData.mqtt.connected;
    const isEnabled = statusData.mqtt && statusData.mqtt.enabled;
    
    if (isEnabled && isConnected) {
        mqttStatusEl.innerHTML = '<span style="color: #28a745; font-size: 20px;">●</span> Connected';
        mqttStatusEl.style.color = '#28a745';
    } else if (isEnabled && !isConnected) {
        mqttStatusEl.innerHTML = '<span style="color: #dc3545; font-size: 20px;">●</span> Disconnected';
        mqttStatusEl.style.color = '#dc3545';
    } else {
        mqttStatusEl.innerHTML = '<span style="color: #6c757d; font-size: 20px;">●</span> Disabled';
        mqttStatusEl.style.color = '#6c757d';
    }
}

function updateBandStatuses() {
    if (!statusData || !statusData.bands) return;
    
    // Create a map of band statuses by name
    const bandStatusMap = {};
    statusData.bands.forEach(band => {
        bandStatusMap[band.name] = band;
    });
    
    // Update each band's status indicator in the UI
    config.WSPRBands?.forEach((band, idx) => {
        const bandStatus = bandStatusMap[band.Name];
        if (!bandStatus) return;
        
        // Find the band element and update its status
        const bandElements = document.querySelectorAll('.item');
        bandElements.forEach(el => {
            const bandNameEl = el.querySelector('strong');
            if (bandNameEl && bandNameEl.textContent.includes(band.Name)) {
                // Remove existing status indicator if present
                let statusIndicator = el.querySelector('.connection-status');
                if (!statusIndicator) {
                    statusIndicator = document.createElement('span');
                    statusIndicator.className = 'connection-status';
                    statusIndicator.style.marginLeft = '10px';
                    bandNameEl.parentNode.insertBefore(statusIndicator, bandNameEl.nextSibling);
                }
                
                // Update status indicator based on state
                const state = bandStatus.state || 'disabled';
                
                if (state === 'connected') {
                    // Green dot - successfully receiving SND data
                    const decodeInfo = bandStatus.last_decode_time
                        ? ` (${bandStatus.last_decode_count} spots, ${formatTimeAgo(bandStatus.last_decode_time)})`
                        : ' (waiting for decode)';
                    statusIndicator.innerHTML = `<span style="color: #28a745; font-size: 16px;">●</span>${decodeInfo}`;
                } else if (state === 'waiting') {
                    // Orange dot - coordinator started, waiting for first recording
                    statusIndicator.innerHTML = '<span style="color: #ff8c00; font-size: 16px;">●</span> (waiting)';
                } else if (state === 'disconnected') {
                    // Grey dot - was connected but not receiving data now
                    const decodeInfo = bandStatus.last_decode_time
                        ? ` (${bandStatus.last_decode_count} spots, ${formatTimeAgo(bandStatus.last_decode_time)})`
                        : '';
                    statusIndicator.innerHTML = `<span style="color: #6c757d; font-size: 16px;">●</span> (disconnected${decodeInfo})`;
                } else if (state === 'failed') {
                    // Red dot - recording failed
                    const errorMsg = bandStatus.error ? ` - ${bandStatus.error}` : '';
                    const errorTitle = bandStatus.error ? ` title="${bandStatus.error}"` : '';
                    statusIndicator.innerHTML = `<span style="color: #dc3545; font-size: 16px;"${errorTitle}>●</span> (failed${errorMsg})`;
                } else {
                    // No indicator for disabled bands
                    statusIndicator.innerHTML = '';
                }
            }
        });
    });
}

function formatTimeAgo(isoString) {
    const date = new Date(isoString);
    const now = new Date();
    const seconds = Math.floor((now - date) / 1000);
    
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    return `${days}d ago`;
}

function updateUI() {
    document.getElementById('mqtt-enabled').checked = config.MQTT.Enabled || false;
    document.getElementById('mqtt-host').value = config.MQTT.Host || 'localhost';
    document.getElementById('mqtt-port').value = config.MQTT.Port || 1883;
    document.getElementById('mqtt-use-tls').checked = config.MQTT.UseTLS || false;
    document.getElementById('mqtt-prefix').value = config.MQTT.TopicPrefix || '';
    document.getElementById('mqtt-username').value = config.MQTT.Username || '';
    document.getElementById('mqtt-password').value = config.MQTT.Password || '';
    document.getElementById('mqtt-qos').value = config.MQTT.QoS !== undefined ? config.MQTT.QoS : 0;
    document.getElementById('mqtt-retain').checked = config.MQTT.Retain || false;
    
    updateInstancesAndBands();
}

function updateInstancesAndBands() {
    const container = document.getElementById('instances-container');
    container.innerHTML = '';
    
    if (!config.KiwiInstances || config.KiwiInstances.length === 0) {
        container.innerHTML = '<div style="padding: 20px; text-align: center; color: #666;">No instances configured</div>';
        return;
    }
    
    config.KiwiInstances.forEach((inst, instIdx) => {
        // Create instance card
        const instanceCard = document.createElement('div');
        instanceCard.style.marginBottom = '20px';
        instanceCard.style.border = '2px solid #ddd';
        instanceCard.style.borderRadius = '8px';
        instanceCard.style.padding = '15px';
        instanceCard.style.backgroundColor = inst.Enabled ? '#fff' : '#f5f5f5';
        
        // Instance header
        const instanceHeader = document.createElement('div');
        instanceHeader.className = 'instance-header';
        instanceHeader.setAttribute('data-instance-idx', instIdx);
        instanceHeader.style.display = 'flex';
        instanceHeader.style.justifyContent = 'space-between';
        instanceHeader.style.alignItems = 'center';
        instanceHeader.style.marginBottom = '15px';
        instanceHeader.style.paddingBottom = '10px';
        instanceHeader.style.borderBottom = '1px solid #ddd';
        
        // Get KiwiSDR status for this instance
        const kiwiStatus = kiwiStatusData[inst.Name];
        let statusLine = `${inst.Host}:${inst.Port}`;
        
        // Add user count if available (make it clickable)
        if (kiwiStatus && !kiwiStatus.error && kiwiStatus.users && kiwiStatus.users_max) {
            statusLine += ` • <span class="users-clickable" onclick="showUsersModal('${inst.Name}')">Users: (${kiwiStatus.users}/${kiwiStatus.users_max})</span>`;
        }
        
        // Show KiwiSDR name if available
        let kiwiNameLine = '';
        if (kiwiStatus && !kiwiStatus.error && kiwiStatus.name) {
            kiwiNameLine = `<div style="font-size: 12px; color: #888; margin-top: 3px; font-style: italic;">${kiwiStatus.name}</div>`;
        }
        
        // Show antenna if available
        let antennaLine = '';
        if (kiwiStatus && !kiwiStatus.error && kiwiStatus.antenna) {
            antennaLine = `<div style="font-size: 12px; color: #888; margin-top: 3px;">📡 Antenna: ${kiwiStatus.antenna}</div>`;
        }
        
        instanceHeader.innerHTML = `
            <div>
                <h3 style="margin: 0; display: inline-block;">🌐 ${inst.Name}</h3>
                <span class="status ${inst.Enabled ? 'status-enabled' : 'status-disabled'}" style="margin-left: 10px;">
                    ${inst.Enabled ? 'Enabled' : 'Disabled'}
                </span>
                <div style="font-size: 13px; color: #666; margin-top: 5px;">
                    ${statusLine}
                </div>
                ${kiwiNameLine}
                ${antennaLine}
            </div>
            <div class="item-actions">
                ${kiwiStatus && !kiwiStatus.error ? `<button class="btn btn-secondary" data-action="info">Info</button>` : ''}
                <button class="btn btn-secondary" data-action="toggle">
                    ${inst.Enabled ? 'Disable' : 'Enable'}
                </button>
                <button class="btn btn-secondary" data-action="edit">Edit</button>
                <button class="btn btn-danger" data-action="delete">Delete</button>
            </div>
        `;

        // Attach event listeners to buttons
        const actionsDiv = instanceHeader.querySelector('.item-actions');
        const buttons = actionsDiv.querySelectorAll('button');
        buttons.forEach(btn => {
            const action = btn.getAttribute('data-action');
            if (action === 'info') {
                btn.addEventListener('click', () => showKiwiInfo(inst.Name));
            } else if (action === 'toggle') {
                btn.addEventListener('click', () => toggleInstance(instIdx));
            } else if (action === 'edit') {
                btn.addEventListener('click', () => editInstance(instIdx));
            } else if (action === 'delete') {
                btn.addEventListener('click', () => deleteInstance(instIdx));
            }
        });
        
        instanceCard.appendChild(instanceHeader);
        
        // Get bands for this instance and sort them
        let instanceBands = config.WSPRBands ? config.WSPRBands.filter(b => b.Instance === inst.Name) : [];
        
        // Sort bands: enabled first, then by frequency
        instanceBands.sort((a, b) => {
            // First priority: enabled status (enabled bands first)
            if (a.Enabled !== b.Enabled) {
                return b.Enabled ? 1 : -1;
            }
            // Second priority: frequency (ascending)
            return a.Frequency - b.Frequency;
        });
        
        // Bands list
        const bandsContainer = document.createElement('div');
        bandsContainer.style.marginLeft = '20px';
        
        if (instanceBands.length === 0) {
            bandsContainer.innerHTML = '<div style="padding: 10px; color: #999; font-style: italic;">No bands configured for this instance</div>';
        } else {
            const bandsList = document.createElement('ul');
            bandsList.className = 'item-list';
            bandsList.style.marginTop = '10px';
            
            instanceBands.forEach(band => {
                const bandIdx = config.WSPRBands.indexOf(band);
                const li = document.createElement('li');
                li.className = 'item';
                li.setAttribute('data-band-name', band.Name);
                li.setAttribute('data-instance-name', inst.Name);
                
                // Grey out bands if instance is disabled
                const isInstanceDisabled = !inst.Enabled;
                li.style.backgroundColor = isInstanceDisabled ? '#e8e8e8' : '#f9f9f9';
                li.style.opacity = isInstanceDisabled ? '0.6' : '1';
                
                li.innerHTML = `
                    <div class="item-info">
                        <strong>📻 ${band.Name}</strong> - ${band.Frequency} kHz
                        <span class="status ${band.Enabled ? 'status-enabled' : 'status-disabled'}">
                            ${band.Enabled ? 'Enabled' : 'Disabled'}
                        </span>
                        <span class="band-connection-status" style="margin-left: 10px;"></span>
                        ${isInstanceDisabled ? '<span style="color: #999; font-size: 12px; margin-left: 10px;">(Instance Disabled)</span>' : ''}
                    </div>
                    <div class="item-actions">
                        <button class="btn ${band.Enabled ? 'btn-danger' : 'btn-success'}" data-action="toggle" ${isInstanceDisabled ? 'disabled style="opacity: 0.5; cursor: not-allowed;"' : ''}>
                            ${band.Enabled ? 'Disable' : 'Enable'}
                        </button>
                        <button class="btn btn-secondary" data-action="duplicate" ${isInstanceDisabled ? 'disabled style="opacity: 0.5; cursor: not-allowed;"' : ''}>Duplicate</button>
                        <button class="btn btn-danger" data-action="delete" ${isInstanceDisabled ? 'disabled style="opacity: 0.5; cursor: not-allowed;"' : ''}>Delete</button>
                    </div>
                `;

                // Attach event listeners to band buttons
                const bandActionsDiv = li.querySelector('.item-actions');
                const bandButtons = bandActionsDiv.querySelectorAll('button');
                bandButtons.forEach(btn => {
                    if (btn.disabled) return; // Skip disabled buttons

                    const action = btn.getAttribute('data-action');
                    if (action === 'toggle') {
                        btn.addEventListener('click', () => toggleBand(bandIdx));
                    } else if (action === 'duplicate') {
                        btn.addEventListener('click', () => duplicateBand(bandIdx));
                    } else if (action === 'delete') {
                        btn.addEventListener('click', () => deleteBand(bandIdx));
                    }
                });

                bandsList.appendChild(li);
            });
            
            bandsContainer.appendChild(bandsList);
        }
        
        // Button container for instance actions
        const buttonContainer = document.createElement('div');
        buttonContainer.style.marginTop = '10px';
        buttonContainer.style.marginLeft = '20px';
        buttonContainer.style.display = 'flex';
        buttonContainer.style.gap = '10px';
        buttonContainer.style.flexWrap = 'wrap';
        
        // Add band button for this instance
        const addBandBtn = document.createElement('button');
        addBandBtn.className = 'btn btn-success';
        addBandBtn.textContent = '+ Add Band to ' + inst.Name;
        
        // Enable All button
        const enableAllBtn = document.createElement('button');
        enableAllBtn.className = 'btn btn-primary';
        enableAllBtn.textContent = '✓ Enable All Bands';
        enableAllBtn.onclick = () => enableAllBands(inst.Name);
        
        // Disable All button
        const disableAllBtn = document.createElement('button');
        disableAllBtn.className = 'btn btn-secondary';
        disableAllBtn.textContent = '✗ Disable All Bands';
        disableAllBtn.onclick = () => disableAllBands(inst.Name);
        
        // Disable buttons based on instance state
        if (!inst.Enabled) {
            // If instance is disabled, disable all buttons
            addBandBtn.disabled = true;
            addBandBtn.style.opacity = '0.5';
            addBandBtn.style.cursor = 'not-allowed';
            
            enableAllBtn.disabled = true;
            enableAllBtn.style.opacity = '0.5';
            enableAllBtn.style.cursor = 'not-allowed';
            
            disableAllBtn.disabled = true;
            disableAllBtn.style.opacity = '0.5';
            disableAllBtn.style.cursor = 'not-allowed';
        } else {
            // Instance is enabled - allow adding bands
            addBandBtn.onclick = () => addBandToInstance(inst.Name);
            
            // Disable enable/disable all buttons only if there are no bands
            if (instanceBands.length === 0) {
                enableAllBtn.disabled = true;
                enableAllBtn.style.opacity = '0.5';
                enableAllBtn.style.cursor = 'not-allowed';
                
                disableAllBtn.disabled = true;
                disableAllBtn.style.opacity = '0.5';
                disableAllBtn.style.cursor = 'not-allowed';
            }
        }
        
        buttonContainer.appendChild(addBandBtn);
        if (instanceBands.length > 0) {
            buttonContainer.appendChild(enableAllBtn);
            buttonContainer.appendChild(disableAllBtn);
        }
        
        instanceCard.appendChild(bandsContainer);
        instanceCard.appendChild(buttonContainer);
        container.appendChild(instanceCard);
    });
}

function showModal(title, fields, onSave) {
    const modal = document.createElement('div');
    modal.className = 'modal';
    modal.innerHTML = `
        <div class="modal-content">
            <div class="modal-header">
                <h3>${title}</h3>
                <button class="modal-close" onclick="this.closest('.modal').remove()">×</button>
            </div>
            <div class="modal-body">
                ${fields.map(f => {
                    if (f.options) {
                        // Dropdown field
                        return `
                            <div class="form-group">
                                <label>${f.label}</label>
                                <select id="modal-${f.id}">
                                    ${f.options.map(opt => `<option value="${opt}" ${opt === f.value ? 'selected' : ''}>${opt}</option>`).join('')}
                                </select>
                            </div>
                        `;
                    } else {
                        // Regular input field
                        return `
                            <div class="form-group">
                                <label>${f.label}</label>
                                <input type="${f.type || 'text'}" id="modal-${f.id}" value="${f.value || ''}" placeholder="${f.placeholder || ''}">
                            </div>
                        `;
                    }
                }).join('')}
            </div>
            <div class="modal-footer">
                <button class="btn btn-secondary" onclick="this.closest('.modal').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="saveModal()">Save</button>
            </div>
        </div>
    `;
    
    modal.querySelector('.btn-primary').onclick = () => {
        const values = {};
        fields.forEach(f => {
            const input = document.getElementById('modal-' + f.id);
            values[f.id] = f.type === 'number' ? parseFloat(input.value) : input.value;
        });
        onSave(values);
        modal.remove();
    };
    
    document.body.appendChild(modal);
}

function addInstance() {
    showModal('Add KiwiSDR Instance', [
        { id: 'name', label: 'Instance Name', placeholder: 'kiwi1' },
        { id: 'host', label: 'Host', placeholder: '44.31.241.9' },
        { id: 'port', label: 'Port', type: 'number', value: '8073' },
        { id: 'password', label: 'Password (optional)', type: 'password' },
        { id: 'mqtt_topic_prefix', label: 'MQTT Topic Prefix (optional - overrides global)', placeholder: 'Leave empty to use global prefix' }
    ], (values) => {
        if (!config.KiwiInstances) config.KiwiInstances = [];
        config.KiwiInstances.push({
            Name: values.name,
            Host: values.host,
            Port: parseInt(values.port),
            Password: values.password,
            MQTTTopicPrefix: values.mqtt_topic_prefix || '',
            Enabled: true
        });
        updateInstancesAndBands();
    });
}

function editInstance(idx) {
    const inst = config.KiwiInstances[idx];
    const oldName = inst.Name;
    showModal('Edit KiwiSDR Instance', [
        { id: 'name', label: 'Instance Name', value: inst.Name },
        { id: 'host', label: 'Host', value: inst.Host },
        { id: 'port', label: 'Port', type: 'number', value: inst.Port },
        { id: 'password', label: 'Password (optional)', type: 'password', value: inst.Password },
        { id: 'mqtt_topic_prefix', label: 'MQTT Topic Prefix (optional - overrides global)', value: inst.MQTTTopicPrefix || '', placeholder: 'Leave empty to use global prefix' }
    ], (values) => {
        const newName = values.name;
        
        // Update the instance
        config.KiwiInstances[idx] = {
            Name: newName,
            Host: values.host,
            Port: parseInt(values.port),
            Password: values.password,
            MQTTTopicPrefix: values.mqtt_topic_prefix || '',
            Enabled: config.KiwiInstances[idx].Enabled !== undefined ? config.KiwiInstances[idx].Enabled : true
        };
        
        // If the name changed, update all bands associated with this instance
        if (oldName !== newName && config.WSPRBands) {
            config.WSPRBands.forEach(band => {
                if (band.Instance === oldName) {
                    band.Instance = newName;
                }
            });
        }
        
        updateInstancesAndBands();
    });
}

function deleteInstance(idx) {
    if (confirm('Delete this instance?')) {
        config.KiwiInstances.splice(idx, 1);
        updateInstancesAndBands();
    }
}

function toggleInstance(idx) {
    config.KiwiInstances[idx].Enabled = !config.KiwiInstances[idx].Enabled;
    updateInstancesAndBands();
}

function addBandToInstance(instanceName) {
    showModal('Add WSPR Band to ' + instanceName, [
        { id: 'name', label: 'Band Name', placeholder: '20m' },
        { id: 'frequency', label: 'Frequency (kHz)', type: 'number', placeholder: '14097.0' }
    ], (values) => {
        if (!config.WSPRBands) config.WSPRBands = [];
        config.WSPRBands.push({
            Name: values.name,
            Frequency: parseFloat(values.frequency),
            Instance: instanceName,
            Enabled: true
        });
        updateInstancesAndBands();
    });
}

function addBand() {
    const instanceNames = config.KiwiInstances ? config.KiwiInstances.map(i => i.Name) : [];
    
    if (instanceNames.length === 0) {
        showAlert('Please add at least one KiwiSDR instance first', 'error');
        return;
    }
    
    showModal('Add WSPR Band', [
        { id: 'name', label: 'Band Name', placeholder: '20m' },
        { id: 'frequency', label: 'Frequency (kHz)', type: 'number', placeholder: '14097.0' },
        { id: 'instance', label: 'Instance Name', options: instanceNames, value: instanceNames[0] }
    ], (values) => {
        if (!config.WSPRBands) config.WSPRBands = [];
        config.WSPRBands.push({
            Name: values.name,
            Frequency: parseFloat(values.frequency),
            Instance: values.instance,
            Enabled: true
        });
        updateInstancesAndBands();
    });
}

function toggleBand(idx) {
    config.WSPRBands[idx].Enabled = !config.WSPRBands[idx].Enabled;
    updateInstancesAndBands();
}

function duplicateBand(idx) {
    const band = config.WSPRBands[idx];
    
    // Get list of instances excluding the current one
    const otherInstances = config.KiwiInstances
        .filter(inst => inst.Name !== band.Instance)
        .map(inst => inst.Name);
    
    if (otherInstances.length === 0) {
        showAlertModal('Cannot Duplicate Band', '❌ No other instances available. Please add another KiwiSDR instance first to duplicate this band.');
        return;
    }
    
    showModal('Duplicate Band: ' + band.Name, [
        {
            id: 'instance',
            label: 'Duplicate to Instance',
            options: otherInstances,
            value: otherInstances[0]
        }
    ], (values) => {
        // Check if target instance already has a band with this frequency
        const existingBand = config.WSPRBands?.find(b =>
            b.Instance === values.instance && b.Frequency === band.Frequency
        );

        if (existingBand) {
            showAlertModal('Cannot Duplicate Band',
                `❌ Instance "${values.instance}" already has a band with frequency ${band.Frequency} kHz (${existingBand.Name}). Cannot duplicate.`
            );
            return;
        }

        if (!config.WSPRBands) config.WSPRBands = [];
        config.WSPRBands.push({
            Name: band.Name,
            Frequency: band.Frequency,
            Instance: values.instance,
            Enabled: true  // Enable by default
        });
        updateInstancesAndBands();
        showAlert(`✅ Band "${band.Name}" duplicated to ${values.instance}`, 'success');
    });
}

function deleteBand(idx) {
    if (confirm('Delete this band?')) {
        config.WSPRBands.splice(idx, 1);
        updateInstancesAndBands();
    }
}

function enableAllBands(instanceName) {
    if (!config.WSPRBands) return;
    
    let count = 0;
    config.WSPRBands.forEach(band => {
        if (band.Instance === instanceName && !band.Enabled) {
            band.Enabled = true;
            count++;
        }
    });
    
    updateInstancesAndBands();
    
    if (count > 0) {
        showAlert(`✅ Enabled ${count} band${count !== 1 ? 's' : ''} for ${instanceName}`, 'success');
    } else {
        showAlert(`ℹ️ All bands for ${instanceName} are already enabled`, 'success');
    }
}

function disableAllBands(instanceName) {
    if (!config.WSPRBands) return;
    
    let count = 0;
    config.WSPRBands.forEach(band => {
        if (band.Instance === instanceName && band.Enabled) {
            band.Enabled = false;
            count++;
        }
    });
    
    updateInstancesAndBands();
    
    if (count > 0) {
        showAlert(`✅ Disabled ${count} band${count !== 1 ? 's' : ''} for ${instanceName}`, 'success');
    } else {
        showAlert(`ℹ️ All bands for ${instanceName} are already disabled`, 'success');
    }
}

async function saveConfig() {
    config.MQTT.Enabled = document.getElementById('mqtt-enabled').checked;
    config.MQTT.Host = document.getElementById('mqtt-host').value;
    config.MQTT.Port = parseInt(document.getElementById('mqtt-port').value);
    config.MQTT.UseTLS = document.getElementById('mqtt-use-tls').checked;
    config.MQTT.TopicPrefix = document.getElementById('mqtt-prefix').value;
    config.MQTT.Username = document.getElementById('mqtt-username').value;
    config.MQTT.Password = document.getElementById('mqtt-password').value;
    config.MQTT.QoS = parseInt(document.getElementById('mqtt-qos').value);
    config.MQTT.Retain = document.getElementById('mqtt-retain').checked;

    try {
        const response = await fetch('/api/config/save', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(config)
        });

        // Scroll to top to show the alert message
        window.scrollTo({ top: 0, behavior: 'smooth' });

        if (response.ok) {
            showAlert('✅ Configuration saved successfully!', 'success');
            
            // Immediately refresh status to show updated MQTT connection and band states
            await loadStatus();
            await loadKiwiStatus();
        } else {
            const error = await response.text();
            showAlert('❌ Error: ' + error, 'error');
        }
    } catch (e) {
        showAlert('❌ Error: ' + e.message, 'error');
    }
}

function exportConfig() {
    // Create a JSON blob from the current config
    const configJson = JSON.stringify(config, null, 2);
    const blob = new Blob([configJson], { type: 'application/json' });
    
    // Create download link
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'kiwi_wspr_config_' + new Date().toISOString().split('T')[0] + '.json';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    
    showAlert('✅ Configuration exported successfully!', 'success');
}

function importConfig(event) {
    const file = event.target.files[0];
    if (!file) return;
    
    const reader = new FileReader();
    reader.onload = function(e) {
        try {
            const importedConfig = JSON.parse(e.target.result);
            
            // Validate that it looks like a valid config
            if (!importedConfig.MQTT || !importedConfig.KiwiInstances) {
                showAlert('❌ Invalid configuration file format', 'error');
                return;
            }
            
            // Update the config
            config = importedConfig;
            updateUI();
            showAlert('✅ Configuration imported successfully! Remember to save.', 'success');
        } catch (err) {
            showAlert('❌ Error parsing configuration file: ' + err.message, 'error');
        }
    };
    reader.readAsText(file);
    
    // Reset the file input so the same file can be imported again
    event.target.value = '';
}

function showAlertModal(title, message) {
    const modal = document.createElement('div');
    modal.className = 'modal';
    modal.innerHTML = `
        <div class="modal-content">
            <div class="modal-header">
                <h3>${title}</h3>
                <button class="modal-close" onclick="this.closest('.modal').remove()">×</button>
            </div>
            <div class="modal-body">
                <p style="margin: 0; line-height: 1.6;">${message}</p>
            </div>
            <div class="modal-footer">
                <button class="btn btn-primary" onclick="this.closest('.modal').remove()">OK</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function showAlert(message, type) {
    const alert = document.getElementById('alert');
    alert.className = 'alert alert-' + type;
    alert.textContent = message;
    alert.style.display = 'block';
    setTimeout(() => { alert.style.display = 'none'; }, 5000);
}

async function testMQTT() {
    // Get current MQTT settings from UI
    const mqttConfig = {
        enabled: true, // Always true for testing
        host: document.getElementById('mqtt-host').value,
        port: parseInt(document.getElementById('mqtt-port').value),
        use_tls: document.getElementById('mqtt-use-tls').checked,
        username: document.getElementById('mqtt-username').value,
        password: document.getElementById('mqtt-password').value,
        topic_prefix: document.getElementById('mqtt-prefix').value,
        qos: parseInt(document.getElementById('mqtt-qos').value),
        retain: document.getElementById('mqtt-retain').checked
    };

    // Validate required fields
    if (!mqttConfig.host) {
        showAlert('❌ Please enter MQTT broker host', 'error');
        return;
    }

    if (!mqttConfig.port || mqttConfig.port < 1 || mqttConfig.port > 65535) {
        showAlert('❌ Please enter a valid MQTT broker port (1-65535)', 'error');
        return;
    }

    // Show testing message
    showAlert('🔄 Testing MQTT connection...', 'success');

    try {
        const response = await fetch('/api/mqtt/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(mqttConfig)
        });

        const result = await response.json();

        if (result.success) {
            showAlert(result.message, 'success');
        } else {
            showAlert(result.message, 'error');
        }
    } catch (e) {
        showAlert('❌ Error testing MQTT: ' + e.message, 'error');
    }
}

function showKiwiInfo(instanceName) {
    const kiwiStatus = kiwiStatusData[instanceName];
    
    if (!kiwiStatus || kiwiStatus.error) {
        showAlertModal('KiwiSDR Info', `❌ Unable to retrieve status for ${instanceName}`);
        return;
    }
    
    const infoHTML = `
        <div style="line-height: 1.8;">
            <p style="margin: 10px 0;"><strong>Status:</strong> ${kiwiStatus.status || 'N/A'}</p>
            <p style="margin: 10px 0;"><strong>Offline:</strong> ${kiwiStatus.offline || 'N/A'}</p>
            <p style="margin: 10px 0;"><strong>Name:</strong> ${kiwiStatus.name || 'N/A'}</p>
            <p style="margin: 10px 0;"><strong>Users:</strong> ${kiwiStatus.users || '0'}/${kiwiStatus.users_max || '0'}</p>
            <p style="margin: 10px 0;"><strong>Location:</strong> ${kiwiStatus.loc || 'N/A'}</p>
            <p style="margin: 10px 0;"><strong>Software Version:</strong> ${kiwiStatus.sw_version || 'N/A'}</p>
            <p style="margin: 10px 0;"><strong>Antenna:</strong> ${kiwiStatus.antenna || 'N/A'}</p>
            <p style="margin: 10px 0; font-size: 12px; color: #888;"><strong>Last Updated:</strong> ${new Date(kiwiStatus.last_update).toLocaleString()}</p>
        </div>
    `;
    
    showAlertModal(`KiwiSDR Info: ${instanceName}`, infoHTML);
}

function updateInstanceHeaders() {
    // Only update the instance header info without rebuilding the entire DOM
    if (!config.KiwiInstances) return;
    
    config.KiwiInstances.forEach((inst, instIdx) => {
        const kiwiStatus = kiwiStatusData[inst.Name];
        
        // Find the instance header by data attribute
        const instanceHeader = document.querySelector(`.instance-header[data-instance-idx="${instIdx}"]`);
        if (!instanceHeader) return;
        
        // Build status line
        let statusLine = `${inst.Host}:${inst.Port}`;
        if (kiwiStatus && !kiwiStatus.error && kiwiStatus.users && kiwiStatus.users_max) {
            statusLine += ` • <span class="users-clickable" onclick="showUsersModal('${inst.Name}')">Users: (${kiwiStatus.users}/${kiwiStatus.users_max})</span>`;
        }
        
        // Show KiwiSDR name if available
        let kiwiNameLine = '';
        if (kiwiStatus && !kiwiStatus.error && kiwiStatus.name) {
            kiwiNameLine = `<div style="font-size: 12px; color: #888; margin-top: 3px; font-style: italic;">${kiwiStatus.name}</div>`;
        }
        
        // Show antenna if available
        let antennaLine = '';
        if (kiwiStatus && !kiwiStatus.error && kiwiStatus.antenna) {
            antennaLine = `<div style="font-size: 12px; color: #888; margin-top: 3px;">📡 Antenna: ${kiwiStatus.antenna}</div>`;
        }
        
        // Update only the header content
        instanceHeader.innerHTML = `
            <div>
                <h3 style="margin: 0; display: inline-block;">🌐 ${inst.Name}</h3>
                <span class="status ${inst.Enabled ? 'status-enabled' : 'status-disabled'}" style="margin-left: 10px;">
                    ${inst.Enabled ? 'Enabled' : 'Disabled'}
                </span>
                <div style="font-size: 13px; color: #666; margin-top: 5px;">
                    ${statusLine}
                </div>
                ${kiwiNameLine}
                ${antennaLine}
            </div>
            <div class="item-actions">
                ${kiwiStatus && !kiwiStatus.error ? `<button class="btn btn-secondary" data-action="info">Info</button>` : ''}
                <button class="btn btn-secondary" data-action="toggle">
                    ${inst.Enabled ? 'Disable' : 'Enable'}
                </button>
                <button class="btn btn-secondary" data-action="edit">Edit</button>
                <button class="btn btn-danger" data-action="delete">Delete</button>
            </div>
        `;

        // Re-attach event listeners to buttons after innerHTML replacement
        const actionsDiv = instanceHeader.querySelector('.item-actions');
        if (actionsDiv) {
            const buttons = actionsDiv.querySelectorAll('button');
            buttons.forEach(btn => {
                const action = btn.getAttribute('data-action');
                if (action === 'info') {
                    btn.addEventListener('click', () => showKiwiInfo(inst.Name));
                } else if (action === 'toggle') {
                    btn.addEventListener('click', () => toggleInstance(instIdx));
                } else if (action === 'edit') {
                    btn.addEventListener('click', () => editInstance(instIdx));
                } else if (action === 'delete') {
                    btn.addEventListener('click', () => deleteInstance(instIdx));
                }
            });
        }
    });
}

// Update band connection status based on receiving_data from status API
function updateBandConnectionStatus() {
    if (!statusData || !statusData.bands) return;

    // Create a map of band statuses by name
    const bandStatusMap = {};
    statusData.bands.forEach(band => {
        bandStatusMap[band.name] = band;
    });

    // Update each band's connection status
    document.querySelectorAll('.item[data-band-name]').forEach(bandEl => {
        const bandName = bandEl.getAttribute('data-band-name');
        const statusSpan = bandEl.querySelector('.band-connection-status');

        if (!statusSpan) return;

        const bandStatus = bandStatusMap[bandName];
        if (!bandStatus) {
            statusSpan.innerHTML = '';
            return;
        }

        // Use the receiving_data flag from the API
        const isReceivingData = bandStatus.receiving_data || false;
        const reconnectCount = bandStatus.reconnect_count || 0;

        // Build reconnection badge - always show it
        const badgeColor = reconnectCount > 0 ? '#ff8c00' : '#28a745';
        const reconnectBadge = ` <span class="status" style="background-color: ${badgeColor}; color: white; font-size: 11px; padding: 2px 6px;">🔄 ${reconnectCount}</span>`;

        // Update the badge
        if (isReceivingData) {
            statusSpan.innerHTML = `<span class="status status-enabled">Receiving Data</span>${reconnectBadge}`;
        } else if (bandStatus.enabled) {
            statusSpan.innerHTML = `<span class="status status-disabled">Not Receiving</span>${reconnectBadge}`;
        } else {
            statusSpan.innerHTML = '';
        }
    });
}

// Start status polling
function startStatusPolling() {
    // Load status immediately
    loadStatus();
    loadKiwiStatus();
    
    // Poll every 10 seconds
    setInterval(loadStatus, 10000);
    setInterval(loadKiwiStatus, 10000);
}

// Update users modal content (internal function)
async function updateUsersModalContent(instanceName) {
    const modalBody = document.getElementById('users-modal-body');

    try {
        // Load user mapping first
        await loadUserMapping();

        const response = await fetch('/api/kiwi/users');
        const usersData = await response.json();

        // Get users for this instance
        const users = usersData[instanceName] || [];

        if (users.length === 0) {
            modalBody.innerHTML = `
                <div style="text-align: center; padding: 20px; color: #666;">
                    <p>No active users found for ${instanceName}</p>
                    <p style="font-size: 0.9em; margin-top: 10px;">Users will appear here when connections are active.</p>
                </div>
            `;
            return;
        }

        // Build users display
        let html = `<div class="user-instance">`;
        html += `<h3>${instanceName} - ${users.length} Active User${users.length !== 1 ? 's' : ''}</h3>`;

        users.forEach(user => {
            // URL decode the name and location
            let rawName = decodeURIComponent(user.n || '(no identity)');

            // Check if this is a generated user ID and translate to band name
            let displayName = rawName;
            if (userToBandMapping[rawName]) {
                displayName = `Decoder - ${userToBandMapping[rawName]}`;
            }

            const location = decodeURIComponent(user.g || 'Unknown location');
            const freqKHz = (user.f / 1000).toFixed(1);
            const mode = user.m || 'N/A';
            const time = user.t || 'N/A';
            const ackTime = user.rs || 'N/A';

            html += `
                <div class="user-card">
                    <div class="user-card-header">
                        <div>
                            <div class="user-name">${displayName}</div>
                            <div class="user-location">📍 ${location}</div>
                        </div>
                    </div>
                    <div class="user-details">
                        <div class="user-detail"><strong>Frequency:</strong> ${freqKHz} kHz</div>
                        <div class="user-detail"><strong>Mode:</strong> ${mode.toUpperCase()}</div>
                        <div class="user-detail"><strong>Connected:</strong> ${time}</div>
                        <div class="user-detail"><strong>Ack:</strong> ${ackTime}</div>
                    </div>
                </div>
            `;
        });

        html += `</div>`;
        modalBody.innerHTML = html;

    } catch (error) {
        console.error('Error loading users:', error);
        modalBody.innerHTML = `
            <div style="text-align: center; padding: 20px; color: #ef4444;">
                <p>❌ Failed to load active users</p>
                <p style="font-size: 0.9em; margin-top: 10px;">${error.message}</p>
            </div>
        `;
    }
}

// Show users modal for a specific instance
async function showUsersModal(instanceName) {
    const modal = document.getElementById('users-modal');
    const modalBody = document.getElementById('users-modal-body');

    // Store current instance for auto-refresh
    currentModalInstance = instanceName;

    // Show modal with loading message
    modalBody.innerHTML = '<p>Loading active users...</p>';
    modal.classList.add('show');

    // Load initial content
    await updateUsersModalContent(instanceName);

    // Set up auto-refresh every 2 seconds while modal is open
    if (usersModalInterval) {
        clearInterval(usersModalInterval);
    }
    usersModalInterval = setInterval(() => {
        if (currentModalInstance) {
            updateUsersModalContent(currentModalInstance);
        }
    }, 2000);
}

// Close users modal
function closeUsersModal() {
    const modal = document.getElementById('users-modal');
    modal.classList.remove('show');

    // Stop auto-refresh
    if (usersModalInterval) {
        clearInterval(usersModalInterval);
        usersModalInterval = null;
    }
    currentModalInstance = null;
}

// Show restart modal
function showRestartModal() {
    const modal = document.getElementById('restart-modal');
    modal.classList.add('show');
}

// Close restart modal
function closeRestartModal() {
    const modal = document.getElementById('restart-modal');
    modal.classList.remove('show');
}

// Confirm and execute restart
async function confirmRestart() {
    closeRestartModal();
    
    // Show alert that restart is in progress
    showAlert('🔄 Restarting application... The page will reload automatically.', 'success');
    
    try {
        const response = await fetch('/api/restart', {
            method: 'POST'
        });
        
        if (response.ok) {
            // Wait a moment for the process to exit
            setTimeout(() => {
                // Try to reload the page every 2 seconds until the server is back
                const reloadInterval = setInterval(() => {
                    fetch('/api/config')
                        .then(response => {
                            if (response.ok) {
                                clearInterval(reloadInterval);
                                window.location.reload();
                            }
                        })
                        .catch(() => {
                            // Server not ready yet, keep trying
                        });
                }, 2000);
            }, 1000);
        } else {
            showAlert('❌ Failed to restart application', 'error');
        }
    } catch (e) {
        // This is expected as the server will disconnect
        // Start polling for server to come back
        setTimeout(() => {
            const reloadInterval = setInterval(() => {
                fetch('/api/config')
                    .then(response => {
                        if (response.ok) {
                            clearInterval(reloadInterval);
                            window.location.reload();
                        }
                    })
                    .catch(() => {
                        // Server not ready yet, keep trying
                    });
            }, 2000);
        }, 1000);
    }
}

// Close modal when clicking outside
document.addEventListener('DOMContentLoaded', () => {
    const usersModal = document.getElementById('users-modal');
    if (usersModal) {
        usersModal.addEventListener('click', (e) => {
            if (e.target === usersModal) {
                closeUsersModal();
            }
        });
    }
    
    const restartModal = document.getElementById('restart-modal');
    if (restartModal) {
        restartModal.addEventListener('click', (e) => {
            if (e.target === restartModal) {
                closeRestartModal();
            }
        });
    }
});

// Load config and start status polling on page load
async function initApp() {
    await loadConfig();
    await loadUserMapping();
    startStatusPolling();
}

initApp();
