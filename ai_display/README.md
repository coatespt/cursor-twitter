# AI Display Server

A web interface for viewing AI-generated analysis of Twitter clusters.

## Features

- **Left Panel**: Dataset overview and navigation controls
- **Right Panel**: Grid display of AI analysis results
- **Batch Navigation**: Next/Previous buttons to navigate through batches
- **Window Management**: Slider to access previous batches in memory
- **Hover Details**: Click on analysis text to see the original prompt
- **Responsive Design**: Works on desktop and mobile devices

## Quick Start

1. **Build the server**:
   ```bash
   cd ai_display
   go build -o ai_display main.go
   ```

2. **Run the server**:
   ```bash
   ./ai_display ../../config/ai_display.yaml
   ```

3. **Open in browser**:
   ```
   http://localhost:8081
   ```

## Configuration

The server uses `config/ai_display.yaml` for configuration:

- **Server**: Port and host settings
- **Database**: PostgreSQL connection details
- **Display**: Batch window size and default settings

## API Endpoints

- `GET /` - Main web interface
- `GET /api/batches?start_batch=X&limit=Y` - Get analysis results

## Navigation

- **Next Button**: Load the next chronological batch
- **Previous Button**: Load the batch before the earliest in current window
- **Window Slider**: Navigate within the current batch window (up to 10 batches)
- **Auto-Advance**: Toggle automatic progression (for future live data)

## Data Display

Each row shows:
- **Run**: Experiment run ID (currently 1)
- **Date/Time**: Batch timestamp
- **Batch**: Batch number
- **Cluster**: Cluster ID within the batch
- **Analysis**: AI-generated analysis text (clickable for prompt details)

## Architecture

- **Go Server**: HTTP server with PostgreSQL backend
- **HTML Templates**: Server-side rendering with Go templates
- **JavaScript**: Client-side navigation and data loading
- **CSS**: Responsive grid layout with modern styling
