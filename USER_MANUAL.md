# Twitter Pipeline User Manual

## Overview

This manual describes how to use all the programs and utilities in the Twitter subject detection pipeline. The project is organized into several categories:

- **Main Application**: The core Twitter processing pipeline
- **Go Utilities** (`util_go/`): Production utilities for data analysis and processing
- **Test Utilities** (`util_test/`): Testing and debugging tools
- **Shell Scripts** (`util_shell/`): Automation and convenience scripts

When running the project, copy an existing config/config.yaml file and ajust it to suit, naming it something like config/config.my-computer.yaml.  This way you won't step on other configs.  If you are changing the config code, make sure you ask Cursor to propagate you changes to the other config files so they don't diverge.

There is no adequate writeup of the meaning of the many config parameters other than the comments in the config files.  Hopefully one is coming.

## Table of Contents

1. [Main Application](#main-application)
2. [Go Utilities](#go-utilities)
3. [Sender Scripts](#sender-scripts)
4. [Parser Scripts](#parser-scripts)
5. [Test Utilities](#test-utilities)
6. [Shell Scripts](#shell-scripts)
7. [SQL Loader](#sql-loader)
8. [Additional Scripts](#additional-scripts)
9. [Configuration](#configuration)
10. [Build System](#build-system)

---

## Main Application

### Twitter Pipeline (`src/main.go`)

The core application that processes Twitter data in real-time.

Note that there are extensive options in config/config.yaml that are covered there, but not here. 

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
- Configurable output suppression for economy of display

**Configuration:**
- See `config/config.yaml` for all settings
- Key parameters: `batch_size`, `window_size`, `freq_classes`
- Clustering: `min_cluster_size`, `min_jaccard_similarity`
- Deduplication: `deduplicate_by_user`, `use_levenshtein_deduplication`

**Caveats**

The -load-state flag is handy in development as it reads in the state saved on disk to save waiting for millions of tokens. However, the statistics will be thrown off any you'll get poor results until the entire token window has been replaced, which may be a logical hour (a real time fifteen minutes on a slow machine.)

When you are filtering for language, e.g. set lang: en in the config.yaml, the log line, for example, "Pipeline stats" tweets=1690148 tokens=6247611 distinct=65190 inbound_queue_size=45 processing_rate_tweets_per_sec=1109.4297100838241 prints the count of Tweets in the specified language.  With "en", this is less than half the number of Tweets read.  So in the case above, you'd be ingesting nearly 3000 Tweets/second.

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

## Starting RabbitMQ

There are numerous ways to start RabbitMQ. You can run it as a daemon that will always be available when you start your computer.

You can also ask Cursor to start rabbit in Docker 

It is easy to start and manage it yourself with the following commands.
 
- docker start rabbitmq
- docker stop rabbitmq
- docker restart rabbitmq
- docker ps | grep rabbit
- docker logs rabbitmq

It can easily be monitored on their Web app. It tells you how much data it is moving and is very useful for diagnosing problems.

- http://localhost:15672/#/

The username and passwords are guest,guest.

Note that the sending and acking rates are accurate. If you are using a language filter as specified in config.yaml, you will see a smaller number under "Pipeline statistics" in the log file. This is because Pipeline statitics isn't counting  the ones that are filtered out because they are not mareked as being of your desired language.
 

## Sender Scripts

### CSV to RabbitMQ Sender (`sender/send_csv_to_mq.py`)

Sends CSV files as messages to RabbitMQ for processing by the main pipeline.

**Usage:**
```bash
python ./sender/send_csv_to_mq.py <directory> [options]
```

**Required Parameters:**
- `directory`: Path to directory containing CSV files to send

**Optional Parameters:**
- `--config <path>`: Path to config file (default: `../config/config.yaml`)
- `--max-queue-depth <number>`: Maximum queue depth before pausing (overrides config)
- `--pause-duration <seconds>`: Duration to pause when queue is full (overrides config)

**Examples:**
```bash
# Send CSV files with default config
python ./sender/send_csv_to_mq.py ../twits/test_language_detect_out/

# Send CSV files with custom config path
python ./sender/send_csv_to_mq.py ../twits/test_language_detect_out/ --config config/config.yaml

# Send with custom queue depth limits
python ./sender/send_csv_to_mq.py ../twits/test_language_detect_out/ --max-queue-depth 5000 --pause-duration 2.0
```

**Features:**
- Sends CSV rows as individual messages to RabbitMQ
- Automatic flow control to prevent queue overflow
- Status tracking to resume from where it left off
- Configurable queue depth limits and pause durations
- Processes files in sorted order

**Configuration:**
The sender reads settings from the config file:
```yaml
sender:
  status_file: "/path/to/sender_status.txt"  # Track last processed file
  max_queue_depth: 10000                     # Pause when queue gets this full
  pause_duration: 1.0                        # Seconds to pause when queue is full
```

**Status Tracking:**
- Creates a status file to track the last processed CSV file
- Enables resuming from where it left off if interrupted
- Useful for processing large datasets that may take hours

### Test Queue (`sender/test_queue.py`)

Tests RabbitMQ queue functionality and message handling.

**Usage:**
```bash
python ./sender/test_queue.py
```

**Features:**
- Tests RabbitMQ connection and queue operations
- Verifies message publishing and consumption
- Useful for debugging queue issues

### Test Status Tracking (`sender/test_status_tracking.py`)

Tests the status tracking functionality of the sender.

**Usage:**
```bash
python ./sender/test_status_tracking.py
```

**Features:**
- Tests atomic file operations for status tracking
- Verifies sender can resume from interrupted runs
- Validates status file creation and reading

### Send Test Tweets (`sender/send_test_tweets.py`)

Sends test tweet data to RabbitMQ for testing purposes.

**Usage:**
```bash
python ./sender/send_test_tweets.py [options]
```

**Features:**
- Generates and sends test tweet messages
- Useful for testing the pipeline without real data
- Configurable message count and content

---

## Parser Scripts

### JSON Parser (`parser/parser.py`)

Parses JSON tweet data and converts to CSV format for processing.

**Usage:**
```bash
python ./parser/parser.py [options]
```

**Features:**
- Converts JSON tweet data to CSV format
- Handles various JSON structures and formats
- Outputs data suitable for the main pipeline
- Configurable parsing options

**Dependencies:**
```bash
pip install -r parser/requirements.txt
```

---

## SQL Loader

### SQL Loader (`src/sql_loader/main.go`)

Loads the JSON output from the main pipeline into a PostgreSQL database for analysis and AI processing. Includes experiment tracking to compare different parameter configurations.

**Build:**
```bash
cd src/sql_loader
go build -o sql_loader main.go
```

**Basic Usage:**
```bash
./sql_loader "Run Name" ../../config/database.yaml ../../config/config.yaml
```

**With Experimental Config:**
```bash
./sql_loader "High Freq Test" ../../config/database.yaml ../../config/config.yaml ../../config/experiments/high_freq.yaml
```

**With Specific JSON File:**
```bash
./sql_loader "Test Run" ../../config/database.yaml ../../config/config.yaml ../../data/august_12_clusters.json
```

**Complete Example:**
```bash
./sql_loader "High Freq Test" ../../config/database.yaml ../../config/config.yaml ../../config/experiments/high_freq.yaml ../../data/august_12_clusters.json
```

**Key Features:**
- **Experiment Tracking**: Each run creates a record with all configuration parameters
- **Duplicate Prevention**: Automatically skips existing batches/clusters
- **Configurable Limits**: Option to cap tweets per cluster to manage database size
- **Data Validation**: Warnings for anomalous data (clusters with no busy words)
- **Config Override Support**: Works with experimental configurations

**Database Management Scripts:**

The SQL loader includes several scripts for database management:

**1. Create Tables** (`src/sql_loader/create_tables.sql`)
```bash
# Creates all tables with proper constraints and indexes
psql -d x_twitter -f src/sql_loader/create_tables.sql
```
- Creates `experiment_runs`, `batches`, `clusters`, `tweets`, `busy_words` tables
- Sets up foreign key relationships and unique constraints
- Creates performance indexes
- Uses `IF NOT EXISTS` to avoid errors if tables already exist

**2. Clear Database** (`src/sql_loader/clear_database.sql`)
```bash
# Removes all data while preserving schema structure
psql -d x_twitter -f src/sql_loader/clear_database.sql
```
- Uses `TRUNCATE CASCADE` to efficiently clear all tables
- Resets sequences to start from 1
- Verifies tables are empty after cleanup
- Much faster than deleting individual records
- **Use this between experiments to get a clean slate**

**3. Drop Tables** (`src/sql_loader/drop_tables.sql`)
```bash
# Completely removes all tables and views
psql -d x_twitter -f src/sql_loader/drop_tables.sql
```
- Drops all pipeline tables and views
- Removes all data and schema
- **Use this for a complete fresh start**

**4. Fix Permissions** (`src/sql_loader/fix_permissions.sql`)
```bash
# Grant necessary permissions (run as superuser)
psql -d x_twitter -f src/sql_loader/fix_permissions.sql
```
- Grants `ALL PRIVILEGES` on all pipeline tables to `petercoates`
- Grants `USAGE, SELECT` on all sequences
- Fixes permission issues if tables were created by different user
- **Run this if you get permission errors**

**Typical Workflow:**
```bash
# First time setup
psql -d x_twitter -f src/sql_loader/create_tables.sql
psql -d x_twitter -f src/sql_loader/fix_permissions.sql

# Between experiments (recommended)
psql -d x_twitter -f src/sql_loader/clear_database.sql

# Complete reset (if needed)
psql -d x_twitter -f src/sql_loader/drop_tables.sql
psql -d x_twitter -f src/sql_loader/create_tables.sql
```

**Configuration:**
- Database settings in `config/database.yaml`
- Uses same pipeline config as main application
- Supports config override files for experiments

**Database Schema:**
- `experiment_runs`: Tracks each experimental run with all parameters
- `batches`: Batch metadata linked to experiment runs
- `clusters`: Cluster information within batches
- `tweets`: Individual tweets within clusters (with medoid marking)
- `busy_words`: Busy words with frequency classes

**Use Cases:**
- Compare results across different parameter sets
- Track which configuration produced which results
- Enable AI analysis of pipeline output
- Historical analysis of parameter effectiveness

---

## Additional Scripts

### Test Startup Scenarios (`test_startup_scenarios.sh`)

Tests various startup scenarios for the pipeline.

**Usage:**
```bash
./test_startup_scenarios.sh
```

**Features:**
- Tests different configuration scenarios
- Validates startup behavior with various settings
- Useful for ensuring robust startup behavior

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
  suppress_individual_tweets: false # Suppress individual tweets in output
```

#### Performance Settings
```yaml
analysis:
  cleanup_trigger_batch_size: 500  # Cleanup frequency
  cleanup_max_items: 4000          # Max items per cleanup
```

### Configuration Overrides

The pipeline supports config overrides for easy experimentation. You can create override files that contain only the parameters you want to change, and the system will merge them with the base config.

**Usage:**
```bash
# Run with base config only
./main -config config/config.yaml

# Run with base config + override for experiments
./main -config config/config.yaml -override config/experiments/high_freq.yaml

# Run with different override
./main -config config/config.yaml -override config/experiments/low_threshold.yaml
```

**Example Override Files:**

**`config/experiments/high_freq.yaml`**:
```yaml
# Only specify what you're changing
freq_classes: 32
z_scores: [7.0, 7.0, 8.0, 7.5, 7.5, 6.5, 6.5, 6.5, 6.5, 6.5, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0, 6.0]
busyword_classes: [2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32]
analysis:
  min_busy_words_per_tweet: 3
```

**`config/experiments/low_threshold.yaml`**:
```yaml
# Lower thresholds for more sensitive detection
z_scores: [4.0, 4.0, 5.0, 4.5, 4.5, 3.5, 3.5, 3.5, 3.5, 3.5, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0]
analysis:
  min_busy_words_per_tweet: 1
  min_jaccard_similarity: 0.2
  min_cluster_size: 3
```

**Benefits:**
- **Minimal files**: Only specify what you're changing
- **Easy tracking**: Each override file focuses on specific parameters
- **No duplication**: Don't repeat all the common settings
- **Clear intent**: Obvious what each experiment is testing

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

## Display Component

The project includes a web-based display component for viewing Twitter cluster analysis results in real-time.

### Overview

The display component provides a web interface for:
- Viewing cluster analysis results from JSON output files
- Navigating through batches of processed data
- Auto-playing through results with configurable timing
- Pretty-printed JSON display of cluster data
- Batch information and metadata display

### Building the Display

```bash
# Navigate to display directory
cd display

# Build the display component
./build.sh
```

### Running the Display

```bash
# Navigate to display directory
cd display

# Run with default config (uses sample data)
./cursor-twitter-display

# Or run with a specific JSON clusters file by editing config.json
# Edit display/config.json to point to your cluster file:
# {
#   "input_file": "../logs/clusters_20250814_125655.txt",
#   "batch_size": 10,
#   "historical_batches": 5,
#   "min_cluster_size": 3
# }

# Open in browser
# Navigate to http://localhost:8080
```

### Display Controls

- **Play/Pause**: Auto-advance through batches every 2 seconds
- **Previous/Next**: Manually step through batches
- **Batch Counter**: Shows current position (e.g., "Batch 3 of 10")
- **Grid View**: Alternative view for cluster visualization

### Configuration

The display component uses `display/config.yaml` for configuration:

```yaml
input_file: "/path/to/clusters.json"
batch_size: 10
historical_batches: 5
min_cluster_size: 3
recurrence_threshold: 0.4
recurrence_strategy: "all_tweets"  # Options: "medoid_only" or "all_tweets"
```

**Recurrence Detection Settings:**
- `recurrence_threshold`: Similarity threshold (0.0 = identical, 1.0 = completely different)
  - `0.3` = Very strict (70% similarity required)
  - `0.4` = Moderate (60% similarity required) 
  - `0.6` = Relaxed (40% similarity required)
- `recurrence_strategy`: Comparison method
  - `"medoid_only"`: Compare current medoid to historical medoids only
  - `"all_tweets"`: Compare current medoid to all historical tweets (more comprehensive)

### File Format

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

### Display Project Structure

```
display/
├── display_main.go        # Go HTTP server
├── build.sh              # Build script
├── go.mod                # Go module file
├── README.md             # Display-specific documentation
├── USER_MANUAL.md        # Display user manual
├── SESSION_NOTES.md      # Display session notes
├── templates/
│   ├── index.html        # Main HTML template
│   └── grid.html         # Grid view template
├── static/
│   ├── style.css         # CSS styling
│   └── script.js         # JavaScript controls
└── data/                 # Sample data files
```

### Integration with Main Pipeline

The display component reads output files generated by the main Twitter processing pipeline. To use them together:

1. **Run the main pipeline** to generate cluster data:
   ```bash
   ./main -config config/config.yaml
   ```

2. **Start the display** to view results:
   ```bash
   cd display
   ./cursor-twitter-display ../logs/clusters_YYYYMMDD_HHMMSS.txt
   ```

3. **Monitor in browser** at `http://localhost:8080`

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