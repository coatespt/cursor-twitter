# Cursor Twitter Data Processor

This is a Go implementation of the Twitter data processing script that processes gzipped CSV files and generates three-part keys for words in the text content.

## Features

- Processes gzipped CSV files containing Twitter data
- Generates three-part MD5 hash keys for words in text content
- Supports both interactive and non-interactive modes
- Handles file compression/decompression
- Provides detailed logging and progress reporting
- Command-line flag support for configuration
- Processing statistics and timing information
- Verbose logging mode for debugging

## Requirements

- Go 1.21 or later

## Installation

1. Navigate to the project directory:
   ```bash
   cd cursor-twitter
   ```

2. Build the executable:
   ```bash
   go build -o process src/process.go
   ```

## Usage

### Basic Usage
```bash
./process <input_directory> <output_directory>
```

### Command Line Flags
```bash
./process [flags] <input_directory> <output_directory>

Flags:
  -auto-clean    Automatically clean output directory without prompting
  -help          Show help message
  -verbose       Enable verbose logging
```

### Examples
```bash
# Interactive mode
./process ../twits/msg_input ../twits/msg_output

# Auto-clean mode (no prompts)
./process -auto-clean ../twits/msg_input ../twits/msg_output

# Verbose logging mode
./process -verbose ../twits/msg_input ../twits/msg_output

# Combined flags
./process -auto-clean -verbose ../twits/msg_input ../twits/msg_output
```

## Input Format

The script expects gzipped CSV files with the following columns:
- id_str
- created_at
- user_id_str
- retweet_count
- text
- retweeted
- at
- http
- hashtag
- words

## Output Format

The processed files will have an additional column:
- threepk (three-part key for words)

## Algorithm

The script generates three-part keys for each word using MD5 hashing with three different unique strings:
- `__0NE__`
- `__TW0__`
- `__THR33__`

Each key component is calculated as: `MD5(word + unique_string) % 1000`

## Error Handling

- Graceful handling of malformed CSV lines
- Automatic cleanup of temporary files
- Detailed error logging
- Skip files that already exist in output directory
- Comprehensive error messages with context

## Performance

The Go version is designed for high performance with:
- Efficient file I/O operations
- Minimal memory allocations
- Concurrent processing capabilities (can be extended)
- Proper resource cleanup
- Processing statistics and timing information

## New Features

### Command Line Interface
- Proper flag parsing with `flag` package
- Help system with examples
- Verbose logging mode for debugging
- Auto-clean mode for non-interactive operation

### Enhanced Logging
- Processing time tracking
- Row count reporting
- Verbose mode for detailed operation logging
- Better error context and reporting

### Statistics
- Files processed count
- Files skipped count
- Total processing time
- Row-level statistics

## Comparison with Python Version

Both versions provide the same functionality, but the Go version offers:
- Better performance for large files
- Single binary deployment
- Stronger type safety
- Lower memory footprint
- Enhanced command-line interface
- Better error handling and logging
- Processing statistics and timing

## Future Development

This Go implementation serves as the foundation for the real-time Twitter anomaly detection pipeline. The next phase will include:
- Real-time tweet processing
- Counter array management
- Bloom filter implementation
- Anomaly detection algorithms
- Distributed processing capabilities

# Cursor Twitter Pipeline

## RabbitMQ Setup

This project uses RabbitMQ as the message queue for decoupling ingestion and processing.

### Option 1: Install RabbitMQ with Homebrew (macOS)

```sh
brew install rabbitmq
brew services start rabbitmq
```
- Management UI: http://localhost:15672 (user: guest, pass: guest)
- AMQP port: 5672

### Option 2: Run RabbitMQ with Docker

```sh
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management
```
- Management UI: http://localhost:15672 (user: guest, pass: guest)
- AMQP port: 5672

### Option 3: Download and Run Standalone

- Download from: https://www.rabbitmq.com/download.html
- Extract and run: `rabbitmq-server`

### Option 4: Free Cloud RabbitMQ

- Sign up for a free [CloudAMQP](https://www.cloudamqp.com/) instance ("Little Lemur" plan)
- Use the provided connection string in your config

---

## Project Overview

This pipeline processes tweets for anomaly detection and clustering using a modular, message-queue-based architecture. See the rest of this README for usage and architecture details. 