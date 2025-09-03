// Global state
let currentBatchWindow = [];
let currentBatchIndex = 0;
let totalBatches = 0;
let batchWindowSize = 10; // Normal window size
let maxWindowSize = 50;    // Maximum window size when going back

// Initialize the application
document.addEventListener('DOMContentLoaded', function() {
    // Load initial data
    nextBatch();
    
    // Set up event listeners
    setupEventListeners();
});

function setupEventListeners() {
    // Auto-advance checkbox
    document.getElementById('auto-advance').addEventListener('change', function(e) {
        if (e.target.checked) {
            // TODO: Implement auto-advance functionality
            console.log('Auto-advance enabled');
        }
    });
    
    // Start time input
    document.getElementById('start-time').addEventListener('change', function(e) {
        // TODO: Implement time-based navigation
        console.log('Start time changed:', e.target.value);
    });
    
    // Window slider
    document.getElementById('window-slider').addEventListener('input', function(e) {
        updateWindowPosition();
    });
}

// Load the next batch of data
async function nextBatch() {
	try {
		const startBatch = currentBatchWindow.length > 0 ? 
			Math.max(...currentBatchWindow.map(r => r.batch_number)) + 1 : 0;
		
		const runSelect = document.getElementById('experiment-run');
		if (!runSelect) {
			console.error('Experiment run dropdown not found');
			return;
		}
		
		const runID = runSelect.value;
		if (!runID) {
			console.error('No experiment run selected');
			return;
		}
		
		console.log('Loading next batch for run_id:', runID, 'starting from batch:', startBatch);
		
		const response = await fetch(`/api/batches?start_batch=${startBatch}&limit=1&run_id=${runID}`);
		if (!response.ok) {
			throw new Error(`HTTP error! status: ${response.status}`);
		}
		
		const results = await response.json();
		console.log('Received results:', results.length, 'items');
		
		if (results.length > 0) {
			// Add new batch to beginning of window (most recent at top)
			currentBatchWindow.unshift(...results);
			
			// If we're back to normal operation, trim to normal window size
			if (currentBatchWindow.length > batchWindowSize * 10) { // Estimate 10 clusters per batch
				currentBatchWindow = currentBatchWindow.slice(0, batchWindowSize * 10);
			}
			
			// Update display
			updateDisplay();
			updateStatus();
			
			// Update window slider
			updateWindowSlider();
		} else {
			console.log('No more batches available');
		}
	} catch (error) {
		console.error('Error loading next batch:', error);
		showError('Failed to load next batch');
	}
}

// Load the previous batch
async function previousBatch() {
	if (currentBatchWindow.length === 0) {
		return;
	}
	
	try {
		const earliestBatch = Math.min(...currentBatchWindow.map(r => r.batch_number));
		if (earliestBatch <= 0) {
			console.log('Already at the earliest batch');
			return;
		}
		
		const runID = document.getElementById('experiment-run').value;
		const response = await fetch(`/api/batches?start_batch=${earliestBatch - 1}&limit=1&run_id=${runID}`);
		if (!response.ok) {
			throw new Error(`HTTP error! status: ${response.status}`);
		}
		
		const results = await response.json();
		if (results.length > 0) {
			// Add previous batch to end of window (older batches at bottom)
			currentBatchWindow.push(...results);
			
			// Limit to max window size when going back
			if (currentBatchWindow.length > maxWindowSize * 10) {
				currentBatchWindow = currentBatchWindow.slice(0, maxWindowSize * 10);
			}
			
			// Update display
			updateDisplay();
			updateStatus();
			
			// Update window slider
			updateWindowSlider();
		}
	} catch (error) {
		console.error('Error loading previous batch:', error);
		showError('Failed to load previous batch');
	}
}

// Update the display with current data
function updateDisplay() {
    const grid = document.getElementById('results-grid');
    
    if (currentBatchWindow.length === 0) {
        grid.innerHTML = '<p>No data available. Click "Next" to load the first batch.</p>';
        return;
    }
    
    // Group results by batch
    const batches = groupResultsByBatch(currentBatchWindow);
    
    let html = '';
    
    // Add header row
    html += `
        <div class="result-header">
            <div>Run</div>
            <div>Date/Time</div>
            <div>Batch</div>
            <div>Cluster</div>
            <div>Analysis</div>
        </div>
    `;
    
    	// Add data rows
	batches.forEach((batch, batchIndex) => {
		const batchClass = batchIndex % 2 === 0 ? 'batch-even' : 'batch-odd';
		
		batch.results.forEach(result => {
			const batchTime = new Date(result.batch_time).toLocaleString();
			const analysisText = formatAnalysisText(result.response_text);
			
			html += `
				<div class="result-row ${batchClass}">
					<div class="result-data">
						<div>1</div>
						<div>${batchTime}</div>
						<div>${result.batch_number}</div>
						<div>${result.cluster_number}</div>
						<div class="analysis-text" onclick="showTooltip(event, '${escapeHtml(result.prompt_text)}')">
							${analysisText}
						</div>
					</div>
				</div>
			`;
		});
	});
    
    grid.innerHTML = html;
}

// Group results by batch number
function groupResultsByBatch(results) {
    const batches = {};
    
    results.forEach(result => {
        if (!batches[result.batch_number]) {
            batches[result.batch_number] = {
                batchNumber: result.batch_number,
                batchTime: result.batch_time,
                results: []
            };
        }
        batches[result.batch_number].results.push(result);
    });
    
    	// Convert to array and sort by batch number (newest first)
	return Object.values(batches).sort((a, b) => b.batchNumber - a.batchNumber);
}

// Format analysis text for display
function formatAnalysisText(text) {
	if (!text) return 'No analysis available';
	
	// Clean up the text, remove leading whitespace from each line
	let cleaned = text.trim();
	
	// Split into lines, trim each line, and rejoin
	const lines = cleaned.split('\n');
	const trimmedLines = lines.map(line => line.trim());
	cleaned = trimmedLines.join('\n');
	
	// Also remove any remaining leading whitespace patterns
	cleaned = cleaned.replace(/^\s+/gm, '');
	
	const maxLength = 200;
	
	if (cleaned.length <= maxLength) {
		return cleaned;
	}
	
	return cleaned.substring(0, maxLength) + '...';
}

// Show tooltip with tweet details
function showTooltip(event, promptText) {
    const tooltip = document.getElementById('tooltip');
    tooltip.textContent = promptText;
    tooltip.style.display = 'block';
    tooltip.style.left = event.pageX + 10 + 'px';
    tooltip.style.top = event.pageY + 10 + 'px';
    
    // Hide tooltip after 5 seconds
    setTimeout(() => {
        tooltip.style.display = 'none';
    }, 5000);
}

// Hide tooltip
function hideTooltip() {
    document.getElementById('tooltip').style.display = 'none';
}

// Update status information
function updateStatus() {
    const currentBatchSpan = document.getElementById('current-batch');
    const resultsCountSpan = document.getElementById('results-count');
    
    	if (currentBatchWindow.length > 0) {
		const batches = groupResultsByBatch(currentBatchWindow);
		const latestBatch = batches[0]; // First batch is now the newest
		currentBatchSpan.textContent = `Batch: ${latestBatch.batchNumber}`;
	} else {
		currentBatchSpan.textContent = 'Batch: 0';
	}
    
    resultsCountSpan.textContent = `Results: ${currentBatchWindow.length}`;
}

// Change experiment run
function changeExperimentRun() {
	const select = document.getElementById('experiment-run');
	const selectedOption = select.options[select.selectedIndex];
	const runID = select.value;
	
	// Update run details display
	const runDetails = document.getElementById('run-details');
	if (selectedOption) {
		const windowSize = selectedOption.dataset.windowSize || 'N/A';
		const batchSize = selectedOption.dataset.batchSize || 'N/A';
		const freqClasses = selectedOption.dataset.freqClasses || 'N/A';
		const minJaccard = selectedOption.dataset.minJaccard || 'N/A';
		
		runDetails.innerHTML = `
			<div class="detail-item">
				<span>Window Size:</span>
				<span>${windowSize}</span>
			</div>
			<div class="detail-item">
				<span>Batch Size:</span>
				<span>${batchSize}</span>
			</div>
			<div class="detail-item">
				<span>Freq Classes:</span>
				<span>${freqClasses}</span>
			</div>
			<div class="detail-item">
				<span>Min Jaccard:</span>
				<span>${minJaccard}</span>
			</div>
		`;
	}
	
	// Reset batch window and load data for new experiment run
	currentBatchWindow = [];
	currentBatchIndex = 0;
	
	// Load first batch for new experiment run
	nextBatch();
}

// Update window slider
function updateWindowSlider() {
    const slider = document.getElementById('window-slider');
    const positionSpan = document.getElementById('window-position');
    
    if (currentBatchWindow.length === 0) {
        slider.max = 0;
        slider.value = 0;
        positionSpan.textContent = '0';
        return;
    }
    
    const batches = groupResultsByBatch(currentBatchWindow);
    const maxPosition = Math.max(0, batches.length - 1);
    
    slider.max = maxPosition;
    slider.value = maxPosition;
    positionSpan.textContent = maxPosition;
}

// Update window position based on slider
function updateWindowPosition() {
    const slider = document.getElementById('window-slider');
    const positionSpan = document.getElementById('window-position');
    const position = parseInt(slider.value);
    
    positionSpan.textContent = position;
    
    // TODO: Implement window position navigation
    console.log('Window position changed to:', position);
}

// Utility functions
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function showError(message) {
    // Simple error display - could be enhanced with a proper notification system
    console.error(message);
    alert(message);
}

// Global event listeners
document.addEventListener('click', function(event) {
    if (!event.target.classList.contains('analysis-text')) {
        hideTooltip();
    }
});
