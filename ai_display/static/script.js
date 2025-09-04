// Global state
let currentBatchWindow = [];
let currentBatchIndex = 0;
let totalBatches = 0;
let batchWindowSize = 10; // Normal window size
let maxWindowSize = 50;    // Maximum window size when going back

// Global variables
let currentMode = 'standard';
let currentRunId = 1;
let currentBatch = 0;
let currentClusters = [];
let selectedClusterId = null;

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

// Mode switching
function switchMode(mode) {
    currentMode = mode;
    
    // Update mode buttons
    document.querySelectorAll('.mode-btn').forEach(btn => btn.classList.remove('active'));
    document.getElementById(`mode-${mode}`).classList.add('active');
    
    // Show/hide controls
    document.getElementById('standard-controls').style.display = mode === 'standard' ? 'block' : 'none';
    document.getElementById('evolution-controls').style.display = mode === 'evolution' ? 'block' : 'none';
    
    // Show/hide displays
    document.getElementById('standard-display').style.display = mode === 'standard' ? 'block' : 'none';
    document.getElementById('evolution-display').style.display = mode === 'evolution' ? 'block' : 'none';
    
    // Update panel title
    const title = mode === 'standard' ? 'AI Analysis Results' : 'Cluster Evolution Analysis';
    document.getElementById('panel-title').textContent = title;
    
    // Clear evolution results when switching
    if (mode === 'standard') {
        clearEvolutionResults();
        clearError();
    } else if (mode === 'evolution') {
        // Auto-load batches when switching to evolution mode
        loadBatchesWithClusters();
    }
}

// Load batches that have clusters for the current run
async function loadBatchesWithClusters() {
    try {
        const response = await fetch(`/api/batches-with-clusters?run_id=${currentRunId}`);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        const batches = await response.json();
        populateBatchSelector(batches);
        
    } catch (error) {
        console.error('Error loading batches:', error);
        showError('Error loading batches. Please try again.');
    }
}

// Populate the batch selector dropdown
function populateBatchSelector(batches) {
    const selector = document.getElementById('evolution-batch');
    selector.innerHTML = '<option value="">Select a batch</option>';
    
    batches.forEach(batch => {
        const option = document.createElement('option');
        option.value = batch.batch_number;
        option.textContent = `Batch ${batch.batch_number} (${batch.cluster_count} clusters) - ${new Date(batch.batch_time).toLocaleString()}`;
        selector.appendChild(option);
    });
}

// Load clusters for a specific batch
async function loadClustersForBatch() {
    const batchNumber = document.getElementById('evolution-batch').value;
    if (!batchNumber) {
        return; // No batch selected, nothing to do
    }
    
    try {
        const response = await fetch(`/api/clusters?batch_number=${batchNumber}&run_id=${currentRunId}`);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        currentClusters = await response.json();
        populateClusterSelector();
        
    } catch (error) {
        console.error('Error loading clusters:', error);
        showError('Error loading clusters for batch. Please try again.');
    }
}

// Populate the cluster selector dropdown
function populateClusterSelector() {
    const selector = document.getElementById('evolution-cluster');
    selector.innerHTML = '<option value="">Select a cluster</option>';
    
    if (currentClusters.length === 0) {
        selector.innerHTML = '<option value="">No clusters found in this batch</option>';
        return;
    }
    
    currentClusters.forEach(cluster => {
        const option = document.createElement('option');
        option.value = cluster.cluster_id;
        option.textContent = `Cluster ${cluster.cluster_number} (${cluster.size} tweets) - ${cluster.busy_words.join(', ')}`;
        selector.appendChild(option);
    });
}

// Handle cluster selection
function selectStartingCluster() {
    const selector = document.getElementById('evolution-cluster');
    selectedClusterId = selector.value;
}

// Run cluster evolution analysis
async function runEvolutionAnalysis() {
    if (!selectedClusterId) {
        showError('Please select a starting cluster first.');
        return;
    }
    
    const batchesBack = document.getElementById('batches-back').value;
    const minMatchingWords = document.getElementById('min-matching-words').value;
    
    // Show loading
    document.getElementById('evolution-loading').style.display = 'block';
    document.getElementById('evolution-results-grid').innerHTML = '';
    
    try {
        const response = await fetch(`/api/cluster-evolution?cluster_id=${selectedClusterId}&batches_back=${batchesBack}&min_matching_words=${minMatchingWords}`);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        const results = await response.json();
        displayEvolutionResults(results);
        
    } catch (error) {
        console.error('Error running evolution analysis:', error);
        showError('Error running cluster evolution analysis. Please try again.');
    } finally {
        document.getElementById('evolution-loading').style.display = 'none';
    }
}

// Display evolution analysis results
function displayEvolutionResults(results) {
    const container = document.getElementById('evolution-results-grid');
    container.innerHTML = '';
    
    if (results.length === 0) {
        container.innerHTML = '<p>No results found for the given parameters.</p>';
        return;
    }
    
    results.forEach(result => {
        const resultDiv = document.createElement('div');
        resultDiv.className = 'evolution-result';
        resultDiv.classList.add(result.type.toLowerCase().replace(' ', '-'));
        
        const title = result.type === 'TARGET CLUSTER' ? 'Starting Cluster' : `Matching Cluster (${result.batches_back} batches back)`;
        
        resultDiv.innerHTML = `
            <div class="result-header">
                <h3>${title}</h3>
                <div class="result-meta">
                    <span>Batch: ${result.batch_number}</span>
                    <span>Cluster: ${result.cluster_number}</span>
                    <span>Size: ${result.size}</span>
                    ${result.batches_back > 0 ? `<span>Batches Back: ${result.batches_back}</span>` : ''}
                </div>
            </div>
            <div class="result-content">
                <div class="busy-words">
                    <strong>Busy Words:</strong> ${result.busy_words.join(', ')}
                </div>
                <div class="ai-summary">
                    <strong>AI Analysis:</strong>
                    <div class="summary-text">${formatAnalysisText(result.ai_summary)}</div>
                </div>
            </div>
        `;
        
        container.appendChild(resultDiv);
    });
    
    // Update results count
    document.getElementById('results-count').textContent = `Results: ${results.length}`;
}

// Clear evolution results
function clearEvolutionResults() {
    document.getElementById('evolution-results-grid').innerHTML = '';
    document.getElementById('evolution-loading').style.display = 'none';
    document.getElementById('evolution-cluster').innerHTML = '<option value="">Select a cluster first</option>';
    selectedClusterId = null;
}

// Utility functions
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Show error message without modal dialog
function showError(message) {
    // Create or update error display
    let errorDiv = document.getElementById('error-message');
    if (!errorDiv) {
        errorDiv = document.createElement('div');
        errorDiv.id = 'error-message';
        errorDiv.className = 'error-message';
        document.querySelector('.right-panel').insertBefore(errorDiv, document.querySelector('.right-panel').firstChild);
    }
    
    errorDiv.textContent = message;
    errorDiv.style.display = 'block';
    
    // Auto-hide after 5 seconds
    setTimeout(() => {
        errorDiv.style.display = 'none';
    }, 5000);
}

// Clear error message
function clearError() {
    const errorDiv = document.getElementById('error-message');
    if (errorDiv) {
        errorDiv.style.display = 'none';
    }
}

// Global event listeners
document.addEventListener('click', function(event) {
    if (!event.target.classList.contains('analysis-text')) {
        hideTooltip();
    }
});
