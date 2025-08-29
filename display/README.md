# Cursor Twitter Display

A simple web-based display for viewing Twitter cluster analysis results from the cursor-twitter project.

## Features

- **File-based input**: Reads JSON output files from cursor-twitter
- **Batch navigation**: Step through batches one by one
- **Auto-play**: Automatically advance through batches
- **JSON display**: Pretty-printed JSON output
- **Batch information**: Summary of each batch's metadata

## Usage

1. **Build the project**:
   ```bash
   go build
   ```

2. **Run with a JSON file**:
   ```bash
   ./cursor-twitter-display ../cursor-twitter/logs/clusters_20250814_125655.txt
   ```

3. **Open in browser**:
   Navigate to `http://localhost:8080`

## Controls

- **Play/Pause**: Auto-advance through batches every 2 seconds
- **Previous/Next**: Manually step through batches
- **Batch counter**: Shows current position (e.g., "Batch 3 of 10")

## Display Modes
### Grid 
The grid mode shows on the left the clusters in the current batch as a set of rows, one for each busy word used in the cluster.  

The columns to the left are the appearance of the busy word's appearance in the n preceding batches.  

This is to give an idea of how the busy words in the current cluster have been appearing in the last n batches.

### Medoid 
The medoid view shows the medoids for clusters for recent batches, grouped by batch, the number of Tweets that made up the cluster, and the busywords for each cluster.  

The grid extending to the left shows how many clusters in the preceding n batches it matches to. The more ephemeral the subject, the fewer 1's will be seen in the previous n batches.

### Bubble
The bubble graphs shows 
- The medoid Tweet for each cluster in the current batch.
- The busywords for the current batch 
   - The right-most bubble column is the current busy words. 
   - Extending to the left are earlier and earlier occurrences of the words that were busy but no longer are. 
   - If a word that is to the left again becomes a busyword, it snaps over to the right, so the words that are moving left are staler and staler the farther left they are
   - The shade of the bubble reflect the background frequency of the word.
   - The size of the bubble reflects how much the current frequency of the word departs from what is typical of it's nominal frequency class.
- The righmost column lists all the busy words that fit, from current to oldest. Often, the older ones won't fit.
- Flying over a bubble reveals hidden information about it: the word, the batch, the frequency class, the divergence.

Thise things let you see what words are currently super active, and the Tweets that are associateed with them.

There are times when the entire grid is bland because there are no new subjects. This is normal behavior.

## File Format

The display expects JSON files with one batch per line, where each line contains a JSON object with this structure:

```json
{
  "batch_number": 1,
  "batch_time": "2025-08-14T12:56:55Z",
  "method": "graph",
  "total_tweets": 25000,
  "total_clusters": 15,
  "clusters_above_min_size": 8,
  "clusters": [...]
}
```

## Development

- **Backend**: Go HTTP server (`main.go`)
- **Frontend**: HTML/CSS/JavaScript (`templates/`, `static/`)
- **Data**: JSON files from cursor-twitter output

## Project Structure

```
cursor-twitter-display/
├── main.go              # Go HTTP server
├── templates/
│   └── index.html       # Main HTML template
├── static/
│   ├── style.css        # CSS styling
│   └── script.js        # JavaScript controls
└── data/                # Sample data files
```
