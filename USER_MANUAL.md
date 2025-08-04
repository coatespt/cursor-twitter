# Twitter Pipeline User Manual

## Overview

This manual describes how to use all the programs and utilities in the Twitter subject detection pipeline. The project is organized into several categories:

- **Main Application**: The core Twitter processing pipeline
- **Go Utilities** (`util_go/`): Production utilities for data analysis and processing
- **Test Utilities** (`util_test/`): Testing and debugging tools
- **Shell Scripts** (`util_shell/`): Automation and convenience scripts

## Table of Contents

1. [Main Application](#main-application)
2. [Go Utilities](#go-utilities)
3. [Test Utilities](#test-utilities)
4. [Shell Scripts](#shell-scripts)
5. [Configuration](#configuration)
6. [Build System](#build-system)

---

## Main Application

### Twitter Pipeline (`src/main.go`)

The core application that processes Twitter data in real-time.

**Build:**
```bash
go build -o main src/main.go
```

**Run:**
```bash
./main -config config/config.yaml
```

**Key Features:**
- Identification of subjects at single-digit second latency
- Clustering of similar tweets
- Real-time tweet processing from RabbitMQ
- Deduplication and filtering

**Configuration:**
- See `config/config.yaml` for all settings
- Key parameters: `batch_size`, `window_size`, `freq_classes`
- Clustering: `min_cluster_size`, `min_jaccard_similarity`
- Deduplication: `deduplicate_by_user`, `use_levenshtein_deduplication`

---

## Go Utilities

### 1. Token Analyzer (`util_go/analyze_tokens.go`)

Processes the full dataset to compute unique tokens against total Tweets 

**Build:**
```bash
make build-token-frequency
```

**Usage:**
```bash
./token_frequency_analyzer -input data/ -interval 10000 -filter-tokens=true
```

**Parameters:**
- `-input`: Directory containing CSV files (default: "data")
- `-interval`: Report stats every N tweets (default: 10000)
- `-filter-tokens`: Filter URLs, mentions, hashtags (default: true)

**Output:**
- Token frequency statistics
- Distinct token counts over time
- ASCII graph of token growth

**Example:**
```bash
# Analyze all CSV files in data directory
./token_frequency_analyzer -input /path/to/tweet/data -interval 5000
```

### 2. CSV File Finder (`util_go/find_csv_file.go`)

Finds CSV files based on timestamp ranges. Give it a date/time and get back the CSV file that contains that date.  Optional parameter to return the name of the file N filese earlier. This allows for a startup period before the data of interest.

**Build:**
```bash
make build-find-csv
```

**Usage:**
```bash
./find_csv_file -dir /path/to/csv -datetime "2012-02-14 19:35:55" -n 3
```

**Parameters:**
- `-dir`: Directory containing CSV files
- `-datetime`: Target datetime (format: "2012-02-14 19:35:55")
- `-n`: Number of files to go back (default: 1)

**Output:**
- Prints the filename of the Nth file before the target datetime

**Example:**
```bash
# Find the file 3 positions before the file containing 2012-02-14 19:35:55
./find_csv_file -dir /data/tweets -datetime "2012-02-14 19:35:55" -n 3
```

### 3. Language Detector (`util_go/language_detector.go`)

Identifies the language of a Tweet and puts it int he lang: field.
The contents of that field in the original Tweets are useless.
The Python parser optionally does this, but go is about fifty to a hundred times faster.

**Build:**
```bash
make build-language-detector
```

**Usage:**
```bash
./language_detector -input /path/to/input -output /path/to/output -workers 8
```

**Parameters:**
- `-input`: Input directory containing CSV files
- `-output`: Output directory for processed files
- `-workers`: Number of worker goroutines (default: CPU cores)
- `-progress`: Show progress updates (default: true)

**Features:**
- Multi-threaded processing
- Language detection using Lingua library
- Progress reporting
- Error handling and recovery

**Example:**
```bash
# Process all CSV files with 4 workers
./language_detector -input /raw/tweets -output /processed/tweets -workers 4
```

### 4. CSV Filenames to Time (`util_go/csv_file_mapping.go`)

Creates mappings between CSV files and their time ranges.
Files are five minutes. This gives the start times.

**Build:**
```bash
make build-csv-mapping
```

**Usage:**
```bash
./csv_file_mapping [options]
```

**Features:**
- Maps CSV filenames to time ranges
- Helps with data organization and retrieval
- Supports timestamp-based file lookup

### 5. Token Frequency Analyzer (`util_go/token_frequency_analyzer.go`)

Advanced token frequency analysis with detailed statistics.
It's amazingly Zipf. 250 words comprise half of all word usage in English.

**Build:**
```bash
make build-token-frequency
```

**Usage:**
```bash
./token_frequency_analyzer [options]
```

**Features:**
- Detailed token frequency analysis
- Statistical reporting
- Performance metrics

### 6. Token Examiner (`util_go/examine_tokens.go`)

Interactive tool for examining individual tokens and their properties.

**Build:**
```bash
go build -o examine_tokens util_go/examine_tokens.go
```

**Usage:**
```bash
./examine_tokens [options]
```

**Features:**
- Token-by-token analysis
- Pattern recognition
- Debugging tool for token processing

---

## Test Utilities

### 1. RabbitMQ Test (`util_test/test_rabbitmq.go`)

Tests RabbitMQ connection and message consumption.

**Build:**
```bash
go build -o test_rabbitmq util_test/test_rabbitmq.go
```

**Usage:**
```bash
./test_rabbitmq
```

**Features:**
- Tests RabbitMQ connection
- Verifies queue declaration
- Tests message consumption
- Receives up to 5 test messages

**Use Case:**
- Verify RabbitMQ is running and accessible
- Test message queue configuration
- Debug connection issues

### 2. K-Means Test (`util_test/test_kmeans.go`)

Tests the K-means clustering algorithm.
K-means is in there for subject clustering but is of dubious value. Stick with graph clustering unless you have a good reason to use K-Means.

**Build:**
```bash
go build -o test_kmeans util_test/test_kmeans.go
```

**Usage:**
```bash
./test_kmeans
```

**Features:**
- Tests K-means clustering implementation
- Validates clustering algorithms
- Performance testing

### 3. Debug Timestamps (`util_test/debug_timestamps.go`)

Debugging tool for timestamp parsing and conversion.

**Build:**
```bash
go build -o debug_timestamps util_test/debug_timestamps.go
```

**Usage:**
```bash
./debug_timestamps [options]
```

**Features:**
- Timestamp format validation
- Time conversion debugging
- Date/time parsing assistance

---

## Shell Scripts

### 1. Tail Log (`util_shell/tail-the-log.sh`)

Automatically finds and tails the latest pipeline log file.
Very handy.

**Usage:**
```bash
./util_shell/tail-the-log.sh
```

**Features:**
- Automatically finds the most recent `pipeline_*.log` file
- Tails the log with follow option (`tail -f`)
- Error handling for missing logs directory
- Clear output formatting

**Example:**
```bash
# Start tailing the latest log
./util_shell/tail-the-log.sh
# Press Ctrl+C to stop
```

### 2. Run Tests (`util_shell/run_tests.sh`)

Comprehensive test runner for the entire project.

**Usage:**
```bash
./util_shell/run_tests.sh [options]
```

**Features:**
- Runs all Go tests
- Generates coverage reports
- Performance benchmarks
- Multiple test modes

### 3. Clean Up Old Run (`util_shell/clean_up_old_run.sh`)

Cleans up old build artifacts and temporary files.

**Usage:**
```bash
./util_shell/clean_up_old_run.sh
```

**Features:**
- Removes old build artifacts
- Cleans temporary files
- Frees disk space

---

## Configuration

### Main Configuration (`config/config.yaml`)

The main configuration file controls all aspects of the pipeline.

**Key Sections:**

#### Core Settings
```yaml
mode: mqj                    # Processing mode
window: 3000000             # Token window size
batch: 25000                # Batch size for processing
freq_classes: 24            # Number of frequency classes
```

#### Analysis Settings
```yaml
analysis:
  clustering_method: graph   # "graph" or "kmeans"
  min_cluster_size: 9        # Minimum cluster size
  min_jaccard_similarity: 0.4 # Similarity threshold
  deduplicate_by_user: true  # Enable deduplication
  cluster_sort_descending: false # Sort order
```

#### Performance Settings
```yaml
analysis:
  cleanup_trigger_batch_size: 500  # Cleanup frequency
  cleanup_max_items: 4000          # Max items per cleanup
```

### Building Configuration

Use the Makefile for building all utilities:

```bash
# Build all utilities
make build-language-detector
make build-find-csv
make build-csv-mapping
make build-token-frequency

# Clean build artifacts
make clean

# Run tests
make test
make test-verbose
make test-coverage
```

---

## Common Workflows

### 1. Initial Setup
```bash
# Build all utilities
make build-language-detector
make build-find-csv
make build-csv-mapping
make build-token-frequency

# Test RabbitMQ connection
./util_test/test_rabbitmq

# Start the main pipeline
./main -config config/config.yaml
```

### 2. Data Analysis
```bash
# Analyze token frequencies
./token_frequency_analyzer -input data/ -interval 10000

# Find specific CSV files
./find_csv_file -dir data/ -datetime "2023-01-15 14:30:00" -n 2

# Add language detection
./language_detector -input raw/ -output processed/ -workers 4
```

### 3. Monitoring and Debugging
```bash
# Tail the latest log
./util_shell/tail-the-log.sh

# Run comprehensive tests
./util_shell/run_tests.sh

# Clean up old files
./util_shell/clean_up_old_run.sh
```

### 4. Performance Tuning
```yaml
# In config.yaml, adjust these for performance:
window: 3000000             # Larger = more memory, better accuracy
batch: 25000                # Larger = faster processing
freq_classes: 24            # More = finer granularity
cleanup_trigger_batch_size: 500  # More frequent = less memory
```

---

## Troubleshooting

### Common Issues

1. **RabbitMQ Connection Failed**
   - Run `./util_test/test_rabbitmq` to test connection
   - Verify RabbitMQ is running: `sudo systemctl status rabbitmq-server`

2. **No CSV Files Found**
   - Check file paths in configuration
   - Verify CSV files exist in specified directories
   - Use `./find_csv_file` to locate files

3. **Low Performance**
   - Increase `batch_size` in config
   - Adjust `cleanup_trigger_batch_size`
   - Monitor memory usage

4. **No Clusters Generated**
   - Lower `min_cluster_size`
   - Adjust `min_jaccard_similarity`
   - Check `min_busy_words_per_tweet`

### Debugging Tools

- **Tail Log**: `./util_shell/tail-the-log.sh`
- **RabbitMQ Test**: `./util_test/test_rabbitmq`
- **Token Analysis**: `./token_frequency_analyzer`
- **Timestamp Debug**: `./util_test/debug_timestamps`

---

## Performance Tips

1. **Memory Management**
   - Monitor cleanup queue performance
   - Adjust `cleanup_trigger_batch_size` based on memory usage
   - Use `window_size` appropriate for your data volume

2. **Processing Speed**
   - Increase `batch_size` for faster processing
   - Use appropriate number of frequency classes
   - Monitor CPU usage and adjust accordingly

3. **Data Quality**
   - Use language filtering for focused analysis
   - Enable deduplication to reduce noise
   - Adjust similarity thresholds for better clustering

---

## Support

For issues and questions:
1. Check the logs using `./util_shell/tail-the-log.sh`
2. Run test utilities to verify components
3. Review configuration settings
4. Monitor system resources (CPU, memory, disk)

The pipeline is designed to be robust and self-monitoring, with extensive logging and error handling built in. 