// Timeline visualization for WSPR cycle gaps
// This file contains the JavaScript code to render timeline graphs showing coverage and gaps

function renderGapTimeline(containerId, gapData, hoursBack) {
    const container = document.getElementById(containerId);
    if (!container) {
        console.error('Timeline container not found:', containerId);
        return;
    }

    // Calculate time range (to match backend logic exactly)
    const endTime = new Date();
    const startTime = new Date(endTime.getTime() - (hoursBack * 60 * 60 * 1000));
    
    // Round to 2-minute boundaries using Unix timestamp (to match backend)
    // Backend does: startTime = time.Unix((startTime.Unix()/120)*120, 0)
    const startUnix = Math.floor(startTime.getTime() / 1000);
    const roundedStartUnix = Math.floor(startUnix / 120) * 120;
    const roundedStartTime = new Date(roundedStartUnix * 1000);
    
    // Generate all expected cycles
    const allCycles = [];
    for (let t = new Date(roundedStartTime); t <= endTime; t = new Date(t.getTime() + 120000)) {
        allCycles.push(new Date(t));
    }
    
    // Convert missing cycles to Set for fast lookup
    const missingSet = new Set(gapData.missing_cycles);
    
    // Build timeline HTML
    let html = '<div style="display: flex; flex-direction: column; gap: 8px;">';
    
    // Time labels (show every hour) - use UTC to match backend
    html += '<div style="display: flex; align-items: center; margin-bottom: 5px;">';
    html += '<div style="width: 80px; flex-shrink: 0;"></div>'; // Spacer for alignment
    html += '<div style="flex: 1; display: flex; justify-content: space-between; font-size: 0.75em; color: #64748b;">';
    
    const hourLabels = [];
    for (let i = 0; i < allCycles.length; i++) {
        const cycle = allCycles[i];
        if (cycle.getUTCMinutes() === 0) {
            hourLabels.push({
                index: i,
                label: cycle.toISOString().substr(11, 5) // Format as HH:MM in UTC
            });
        }
    }
    
    hourLabels.forEach((label, idx) => {
        const position = (label.index / allCycles.length) * 100;
        html += `<span style="position: absolute; left: ${position}%; transform: translateX(-50%);">${label.label}</span>`;
    });
    
    html += '</div></div>';
    
    // Timeline bar
    html += '<div style="display: flex; align-items: center;">';
    html += '<div style="width: 80px; flex-shrink: 0; font-size: 0.85em; color: #94a3b8; text-align: right; padding-right: 10px;">Coverage:</div>';
    html += '<div style="flex: 1; display: flex; gap: 1px; height: 40px; background: #0f172a; border-radius: 4px; overflow: hidden; padding: 2px;">';
    
    // Render each cycle as a block - use UTC to match backend
    allCycles.forEach(cycle => {
        const timeStr = cycle.toISOString().substr(11, 5); // Format as HH:MM in UTC
        const isMissing = missingSet.has(timeStr);
        const color = isMissing ? '#ef4444' : '#10b981';
        const title = isMissing ? `Missing: ${timeStr} UTC` : `Coverage: ${timeStr} UTC`;
        const width = `${100 / allCycles.length}%`;
        
        html += `<div style="flex: 1; background: ${color}; min-width: 2px; border-radius: 2px;" title="${title}"></div>`;
    });
    
    html += '</div></div>';
    
    // Legend
    html += '<div style="display: flex; align-items: center; gap: 20px; margin-top: 8px; font-size: 0.85em;">';
    html += '<div style="width: 80px; flex-shrink: 0;"></div>'; // Spacer
    html += '<div style="display: flex; gap: 15px;">';
    html += '<div style="display: flex; align-items: center; gap: 6px;">';
    html += '<div style="width: 16px; height: 16px; background: #10b981; border-radius: 3px;"></div>';
    html += '<span style="color: #94a3b8;">Spots Received</span>';
    html += '</div>';
    html += '<div style="display: flex; align-items: center; gap: 6px;">';
    html += '<div style="width: 16px; height: 16px; background: #ef4444; border-radius: 3px;"></div>';
    html += '<span style="color: #94a3b8;">Missing Cycles</span>';
    html += '</div>';
    html += '</div></div>';
    
    html += '</div>';
    
    container.innerHTML = html;
}

// Function to render all timelines after gaps data is loaded
function renderAllGapTimelines(gapsData, hoursBack) {
    // Wait for DOM to be ready
    setTimeout(() => {
        Object.entries(gapsData).forEach(([instance, bandGaps]) => {
            bandGaps.forEach(gap => {
                const timelineId = `timeline_${gap.instance}_${gap.band.replace(/[^a-zA-Z0-9]/g, '_')}`;
                renderGapTimeline(timelineId, gap, hoursBack);
            });
        });
    }, 100);
}
