let currentBatch = 0;
let totalBatches = 0;
let isPlaying = false;
let playInterval = null;

// Initialize the display
document.addEventListener('DOMContentLoaded', function() {
    loadCurrentBatch();
});

function loadCurrentBatch() {
    fetch(`/api/batch?batch=${currentBatch}`)
        .then(response => response.json())
        .then(data => {
            displayBatchInfo(data);
            displayJSON(data);
            updateBatchInfo();
        })
        .catch(error => {
            console.error('Error loading batch:', error);
            document.getElementById('batchDetails').innerHTML = 'Error loading batch data';
            document.getElementById('jsonOutput').innerHTML = 'Error loading batch data';
        });
}

function displayBatchInfo(batch) {
    const details = document.getElementById('batchDetails');
    details.innerHTML = `
        <strong>Batch Number:</strong> ${batch.batch_number}<br>
        <strong>Time:</strong> ${batch.batch_time}<br>
        <strong>Method:</strong> ${batch.method}<br>
        <strong>Total Tweets:</strong> ${batch.total_tweets}<br>
        <strong>Total Clusters:</strong> ${batch.total_clusters}<br>
        <strong>Clusters Above Min Size:</strong> ${batch.clusters_above_min_size}
    `;
}

function displayJSON(data) {
    const output = document.getElementById('jsonOutput');
    output.textContent = JSON.stringify(data, null, 2);
}

function updateBatchInfo() {
    const batchInfo = document.getElementById('batchInfo');
    batchInfo.textContent = `Batch ${currentBatch} of ${totalBatches}`;
}

function togglePlay() {
    if (isPlaying) {
        pause();
    } else {
        play();
    }
}

function play() {
    isPlaying = true;
    document.getElementById('playBtn').style.display = 'none';
    document.getElementById('pauseBtn').style.display = 'inline-block';
    
    // Auto-advance every 2 seconds
    playInterval = setInterval(() => {
        if (currentBatch < totalBatches - 1) {
            step('next');
        } else {
            pause(); // Stop when we reach the end
        }
    }, 2000);
}

function pause() {
    isPlaying = false;
    document.getElementById('playBtn').style.display = 'inline-block';
    document.getElementById('pauseBtn').style.display = 'none';
    
    if (playInterval) {
        clearInterval(playInterval);
        playInterval = null;
    }
}

function step(direction) {
    fetch(`/api/step?direction=${direction}`)
        .then(response => response.json())
        .then(data => {
            currentBatch = data.current_batch;
            totalBatches = data.total_batches;
            loadCurrentBatch();
        })
        .catch(error => {
            console.error('Error stepping:', error);
        });
}

// Get total batches from the page template
totalBatches = parseInt(document.querySelector('#batchInfo').textContent.match(/of (\d+)/)[1]);
