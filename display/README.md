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
