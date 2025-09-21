# Twitter Pipeline User Manual

## Overview

This manual describes how to use all the programs and utilities in the Twitter subject detection pipeline. The project is organized into several categories:

### Extraction of Subjects From the Firehose with Z-Filters
- **Preprocessor**: Converts original Twitter JSON files to CSV format
- **Main Application**: The core Twitter processing pipeline to extract subjects from the firehose with Z-Filters
- **Main Application Display**: Explore the subjects extracted from the firehose

### Semantic Interpretation of Subjects Via Ollama AI
- **AI Application**: Organizes the output of the main application in Posgres and sends it to Ollama for analysis of the semantics of the subjects 
- **AI Application Display**: Explore the subjects as interpretd by Ollama AI

### Utilities and Ancilary Programs
- **Go Utilities** (`util_go/`): Production utilities for data analysis and processing
- **Test Utilities** (`util_test/`): Testing and debugging tools
- **Shell Scripts** (`util_shell/`): Automation and convenience scripts

### Not Bumping Into Others With Z-Filters
When running the main subject extraction project, you have two options for configuration:

1. **Copy and modify the main config**: Copy `config/config.yaml` and adjust it to suit, naming it something like `config/config.my-computer.yaml`
2. **Use config overrides (recommended)**: Keep the main `config/config.yaml` unchanged and use the `-override` flag to apply custom settings from separate files

The override approach is preferred as it keeps the main config as the authoritative source and allows easy experimentation without file divergence. If you are changing the config code, make sure you ask Cursor to propagate your changes to the other config files so they don't diverge.

There is no adequate writeup of the meaning of the many config parameters other than the comments in the config files.  Hopefully one is coming.

### Not Bumping Into Others With AI  
The AI portion uses a Postgres SQL Database and connects to a locally running Ollama.
If you want to put your output in a commona database, the extraction program below allows you to name it, so that your batches, clusters, and Tweets don't collide with those of others.

The process that takes your main program JSON output and puts it into Postgres will take an identifier on the command line and assoicate it with the parameters you ran with (they have to still be current!)
If you use the same identifier, it will notice and append a number for uniquenes.

## Table of Contents

1. [JSON to CSV Preprocessor](#json-to-csv-preprocessor)
2. [Language Detector](#language-detector)
3. [Starting RabbitMQ](#starting-rabbitmq)
4. [Sender Scripts](#sender-scripts)
5. [Main Application](#main-application)
6. [Display Component](#display-component)
7. [Artificial Intelligence Component](#artificial-intelligence-component)
8. [SQL Loader](#sql-loader)
9. [AI Feeder](#ai-feeder)
10. [AI Display Server](#ai-display-server)
11. [Other Software](#other-software)
12. [Additional Scripts](#additional-scripts)
13. [Shell Scripts, Utilities, Tests, etc](#shell-scripts-utilities-tests-etc)
14. [Configuration](#configuration)
15. [Common Workflows](#common-workflows)
16. [Parser Scripts](#parser-scripts)
17. [Go Utilities](#go-utilities)
18. [Test Utilities](#test-utilities)
19. [Troubleshooting](#troubleshooting)
20. [Performance Tips](#performance-tips)
21. [Support](#support)

---

## JSON to CSV Preprocessor

### JSON to CSV Preprocessor (`parser/parser.py`)

**What it does:** This is the **FIRST STEP** in the entire pipeline. It reads the ORIGINAL Twitter JSON files (compressed .json.gz format) and converts them to CSV format for further processing. This preprocessor handles the raw Twitter data format and extracts the essential fields needed for analysis.

**Dependencies:**
```bash
cd parser
pip install -r requirements.txt
```

**Usage:**
```bash
python parser/parser.py <input_directory> <output_directory> [options]
```

**Required Parameters:**
- `input_directory`: Directory containing .json.gz Twitter files
- `output_directory`: Directory where CSV files will be created

**Optional Parameters:**
- `--num-workers <N>`: Number of worker processes (default: CPU cores)
- `--no-language-detect`: Disable language detection, use original lang field

**Example:**
```bash
# Convert all JSON files in /raw/tweets to CSV in /processed/tweets
python parser/parser.py /raw/tweets /processed/tweets

# Use 8 worker processes for faster processing
python parser/parser.py /raw/tweets /processed/tweets --num-workers 8

# Skip language detection (faster processing)
python parser/parser.py /raw/tweets /processed/tweets --no-language-detect
```

**Output Format:**
The CSV files contain these columns:
- `id_str`: Tweet ID
- `created_at`: Tweet creation timestamp
- `user_id_str`: User ID
- `retweet_count`: Number of retweets
- `text`: Tweet text content
- `retweeted`: Whether this is a retweet
- `at`: Number of @ mentions
- `http`: Number of URLs
- `hashtag`: Number of hashtags
- `words`: Tokenized words from tweet text
- `lang`: Language code (detected or from original JSON)

**Features:**
- **Multi-threaded processing** for high performance
- **Automatic encoding detection** (UTF-8, Latin-1, CP1252, ISO-8859-1)
- **Language detection** using langid library (can be disabled)
- **Progress reporting** with tweets per second metrics
- **Error handling** for malformed JSON and encoding issues
- **Skip processing** if output file already exists
- **Handles Twitter's JSON format** including delete events and scrub_geo events

**Input Requirements:**
- Twitter JSON files in .json.gz format
- Files should contain one JSON object per line
- Standard Twitter API response format

**Output:**
- One CSV file per input JSON file
- Same filename with .csv extension
- UTF-8 encoded CSV with proper escaping

**Performance:**
- Processes thousands of tweets per second
- Multi-threaded for optimal CPU utilization
- Automatic resumption if interrupted

**Example Workflow:**
```bash
# 1. Install dependencies
cd parser
pip install -r requirements.txt

# 2. Convert JSON to CSV
python parser.py /path/to/raw/tweets /path/to/processed/tweets

# 3. Verify output
ls -la /path/to/processed/tweets/*.csv

# 4. Continue with language detection or main pipeline
```

---

## Language Detector

### Language Detector (`util_go/language_detector.go`)

**Optional but recommended component** that processes CSV tweet files and fixes the language field. The `lang` value supplied by GNIP in the original Twitter data is worthless, so this component uses proper language detection to set the correct language for each tweet.

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
- **Multi-threaded processing** for high performance
- **Language detection** using Lingua library
- **Progress reporting** and error handling
- **CSV processing** with language field enrichment
- **Performance**: 50-100x faster than Python equivalent

**Architecture Role:**
The Language Detector is the **second step** in the data processing pipeline:
1. **Raw CSV files** → input data (with worthless GNIP language field)
2. **Language Detector** → fixes language field using proper detection (data enrichment)
3. **Main Pipeline** → processes enriched data with accurate language filtering
4. **SQL Loader** → database storage
5. **AI Analysis** → cluster analysis and display

**Example:**
```bash
# Process all CSV files with 4 workers
./language_detector -input /raw/tweets -output /processed/tweets -workers 4
```

**Dependencies:**
- Go 1.21+
- Lingua library for language detection
- Multi-threading support

---

## Starting RabbitMQ

You only need to start RabbitMQ if you intend to run the main in that mode. (It can also get the data directly from files.) 

There are numerous ways to start RabbitMQ. You can run it as a daemon that will always be available when you start your computer. You can also ask Cursor to start rabbit in Docker 

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

---

## Sender Scripts

### CSV to RabbitMQ Sender (`sender/send_csv_to_mq.py`)

The sender reads the CSV Tweet files and sends them one by one as messages via RabbitMQ. The main pipeline program picks them up from Rabbit. There is no real point in doing this for testing, as MQ slows down processing by five to ten X.  

You definitely would want to use this if, for instance, you were getting the JSON live on another machine, parsing it, and sending it on to the main.  

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

## Main Application

### Twitter Pipeline (`src/main.go`)

The core application reads the pre-processed CSV.  It can be fed by RabbitMQ a line at a time or it can be set to read the CSV directly from the files. The latter is vastly faster. 

Note that there are extensive options in config/config.yaml that are covered there, but not here. 

**Build:**
```bash
cd src
go build -o main .
```

**Run:**
```bash
# Basic run with main config
./main -config ../config/config.yaml

# Run with config override for experiments
./main -config ../config/config.yaml -override ../config/experiments/high_freq.yaml

# Run with custom override file
./main -config ../config/config.yaml -override ../config/my_custom_settings.yaml
```

**Key Features:**
- Identification of subjects at single-digit second latency
- Clustering of similar tweets
- Real-time tweet processing from RabbitMQ
- Deduplication and filtering
- Configurable output suppression for economy of display

**Configuration:**
- **Main config**: `config/config.yaml` contains all default settings
- **Config overrides**: Use `-override` flag to apply custom settings without modifying the main config
- **Override files**: Only specify the parameters you want to change - the system merges them with the base config
- Key parameters: `batch_size`, `window_size`, `freq_classes`
- Clustering: `min_cluster_size`, `min_jaccard_similarity`
- Deduplication: `deduplicate_by_user`, `use_levenshtein_deduplication`

**Config Override System:**
The pipeline supports a flexible config override system that allows you to:
- Keep the main `config/config.yaml` as the authoritative source
- Create experiment-specific override files (e.g., `config/experiments/high_freq.yaml`)
- Override only specific parameters without duplicating the entire config
- Easily switch between different parameter sets for testing

**Example Override Usage:**
```bash
# Run with high frequency experiment settings
./main -config ../config/config.yaml -override ../config/experiments/high_freq.yaml

# Run with low threshold experiment settings  
./main -config ../config/config.yaml -override ../config/experiments/low_threshold.yaml

# Run with your custom settings
./main -config ../config/config.yaml -override ../config/my_computer.yaml
```

**Caveats**

The -load-state flag is handy in development as it reads in the state saved on disk to save waiting for millions of tokens. However, the statistics will be thrown off any you'll get poor results until the entire token window has been replaced, which may be a logical hour (a real time fifteen minutes on a slow machine.)

When you are filtering for language, e.g. set lang: en in the config.yaml, the log line, for example, "Pipeline stats" tweets=1690148 tokens=6247611 distinct=65190 inbound_queue_size=45 processing_rate_tweets_per_sec=1109.4297100838241 prints the count of Tweets in the specified language.  With "en", this is less than half the number of Tweets read.  So in the case above, you'd be ingesting nearly 3000 Tweets/second.

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
go build -o cursor-twitter-display
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

# Artificial Intelligence Component
The foregoing sections have all been related to using the Z-filters heuristic to extract the subjects from the firehose. The subjects are represented in JSON as collections of Tweets with some associated metadata and a medoid Tweet that is the Tweet most typical of the cluster.

All of that functionality is free-standing and can be used without the AI portion which begins here. This additional functionality:
- Parses the JSON output from the foregoing steps and inserts it into a Postgres relational database.
- The database represents it a hierarchical association of Tweets, Clusters, and Batches. Batches are associated with a run, which holds the parameters that the subject analysis was performed with. This allows multiple versions of the dataset to be stored and compared.
- A process runs against the database, reading each cluster and sending it off via HTTP to an Ollama AI for analysis of what the cluster (subject) is about. The HTTP response is then written back to Postgres. The response is associated with the cluster, batch, and run.
- A Web service with a browser allows a user to explore the results data. 
  - The browser allows the user to choose which run, where to start, etc.
  - The user can use next and previous buttons to see the subjects.

## SQL Loader

### SQL Loader (`src/sql_loader/main.go`)

Loads the JSON output from the main pipeline into a PostgreSQL database for analysis and AI processing. Includes experiment tracking to compare different parameter configurations.

**Build:**
```bash
cd src/sql_loader
go build -o sql_loader .
```

**Basic Usage:**
```bash
./sql_loader "Run Name" ../../config/database.yaml ../../config/config.yaml
```

**Real-time Processing (Recommended):**

The SQL loader can run simultaneously with the main Z-filters pipeline, reading data directly from the JSON file as it's being written. This enables real-time database population without waiting for the main pipeline to complete.

**Simultaneous Operation (Main + SQL Loader):**
```bash
# Terminal 1: Start the main Z-filters pipeline
./main -config config/config.yaml > clusters.json

# Terminal 2: Start SQL loader to read the same file in real-time
cd src/sql_loader && go build -o sql_loader . && ./sql_loader "Run Name" ../../config/database.yaml ../../config/config.yaml ../../clusters.json
```

**Key Benefits:**
- **Real-time database population**: Data appears in PostgreSQL as soon as batches are processed
- **No waiting**: Don't need to wait for the main pipeline to complete before loading data
- **Live monitoring**: Can monitor database growth and cluster analysis in real-time
- **Efficient processing**: SQL loader automatically skips existing batches on restart

**Build and Run in One Command:**
```bash
# Build and run in one command for real-time database population
cd src/sql_loader && go build -o sql_loader . && ./sql_loader "Run Name" ../../config/database.yaml ../../config/config.yaml ../../clusters.json

# Or build once, then run multiple times
cd src/sql_loader
go build -o sql_loader .
./sql_loader "Run Name" ../../config/database.yaml ../../config/config.yaml ../../clusters.json
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
- **Real-time Processing**: Can read JSON files while they're being written by the main pipeline
- **Incremental Loading**: Efficiently processes only new data, skips existing batches
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

## AI Feeder

### AI Feeder (`src/ai_feeder/main.go`)

The AI Feeder is the bridge between the processed Twitter clusters and AI analysis. It reads cluster data from the PostgreSQL database, sends it to AI services (like Ollama) for analysis, and stores the results back in the database.

**Build:**
```bash
cd src/ai_feeder
go build -o ai_feeder main.go
```

**Run:**
```bash
./ai_feeder "run-name" ../../config/ai_feeder.yaml
```

**Key Features:**
- **Database Integration**: Reads cluster data from the pipeline database
- **AI Service Support**: Interfaces with Ollama and other AI services
- **Template-Based Prompts**: Uses customizable prompt templates
- **Session Tracking**: Tracks analysis sessions with progress monitoring
- **Retry Logic**: Handles AI service failures with configurable retries
- **Batch Processing**: Configurable batch sizes and processing limits

**Prerequisites:**
1. **Ollama Running**: Start Ollama with your desired model
   ```bash
   ollama serve
   ollama pull llama3.1:8b
   ```

2. **Database Setup**: Ensure the AI analysis tables are created
   ```bash
   psql -d x_twitter -f src/ai_feeder/schema.sql
   ```

3. **Data Available**: Ensure you have cluster data in the database from the SQL loader

**Configuration:**
```yaml
# config/ai_feeder.yaml
ai:
  model: "llama3.1:8b"                    # AI model to use
  endpoint: "http://localhost:11434/api/generate"  # Ollama API endpoint
  timeout: 60                              # Request timeout in seconds

processing:
  batch_size: 1                           # Clusters to process in parallel
  max_retries: 3                          # Max retries per request
  prompt_template: "prompts/cluster_analysis.txt"  # Prompt template file
  analysis_type: "cluster_summary"        # Type of analysis
  session_name: "Cluster Analysis Run 1"  # Session name
  run_id: 1                               # Experiment run ID to analyze
  max_clusters: 100                       # Max clusters to process (0 = all)
```

**Data Flow:**
1. **Read**: Extract cluster data from PostgreSQL
2. **Process**: Generate prompts using templates
3. **Analyze**: Send to AI service (Ollama)
4. **Store**: Save results back to database
5. **Track**: Monitor progress and session status

**Prompt Templates:**
The AI feeder uses Go templates to generate prompts. Available template variables:
- `{{.ClusterID}}`: Cluster identifier
- `{{.BatchNumber}}`: Batch number
- `{{.Size}}`: Number of tweets in cluster
- `{{.QualityScore}}`: Cluster quality score
- `{{.BusyWords}}`: Array of busy words
- `{{.MedoidTweet}}`: Representative tweet

**Example Template:**
```text
Analyze this Twitter cluster:

Cluster ID: {{.ClusterID}}
Size: {{.Size}} tweets
Busy Words: {{range .BusyWords}}{{.}} {{end}}

Representative Tweet:
{{.MedoidTweet}}

Provide analysis of the main topic and sentiment.
```

**Monitoring:**
Check analysis session status:
```sql
SELECT session_id, session_name, status, processed_clusters, total_clusters 
FROM ai_analysis_sessions 
WHERE run_id = 1;
```

View AI analysis results:
```sql
SELECT c.cluster_id, ar.response_text, ar.processing_time_ms
FROM ai_analysis_results ar
JOIN clusters c ON ar.cluster_id = c.id
WHERE ar.session_id = 1
ORDER BY c.cluster_id;
```

**Use Cases:**
- **Subject Analysis**: AI-generated summaries of what each cluster is about
- **Sentiment Analysis**: Understanding the tone and sentiment of discussions
- **Topic Classification**: Categorizing clusters by subject matter
- **Insight Generation**: Extracting key insights from large volumes of tweets

---

## AI Display Server

### AI Display (`ai_display/main.go`)

A web-based interface for viewing AI-generated analysis of Twitter clusters, with experiment run selection and batch navigation.  This is a Web server that can be reached as localhost:8081 to view the AI generated data.

**Build:**
```bash
make build-ai-display
# or manually:
cd ai_display && go build -o ai_display main.go
```

**Run:**
```bash
./ai_display ../config/ai_display.yaml
```

**Build and Run in One Command:**
```bash
cd ai_display && go build -o ai_display main.go && ./ai_display ../config/ai_display.yaml
```

**Access:**
- Open browser to `http://localhost:8081`
- Server runs on port 8081 by default

**Key Features:**

#### **Left Panel - Controls & Overview**
- **Dataset Overview**: Total batches, clusters, time range, batch size
- **Experiment Run Selector**: Dropdown to choose which experimental configuration to view
- **Run Details**: Shows key parameters (window size, batch size, frequency classes, Jaccard threshold)
- **Navigation Controls**: Next/Previous buttons, batch window slider, auto-advance toggle

#### **Right Panel - AI Analysis Display**
- **Grid Layout**: Shows analysis results in organized rows
- **Batch Grouping**: Results grouped by batch with alternating colors
- **Analysis Text**: Clickable AI-generated analysis with hover tooltips
- **Push-Down Behavior**: New batches appear at top, older ones scroll down

#### **Smart Navigation**
- **Batch Window**: Configurable window size (default: 10 batches)
- **Dynamic Expansion**: Window expands when going backward beyond normal limit
- **Auto-Reset**: Window returns to normal size when advancing forward
- **Status Display**: Current batch number and result count

#### **Experiment Run Management**
- **Automatic Detection**: Shows all available experiment runs from database
- **Parameter Display**: Shows key configuration parameters for each run
- **Run Switching**: Seamlessly switch between different experimental configurations
- **Data Filtering**: Results automatically filtered by selected experiment run

**Configuration:**
```yaml
# config/ai_display.yaml
server:
  port: 8081
  host: "localhost"

database:
  host: "192.168.1.76"
  port: 5432
  name: "x_twitter"
  user: "petercoates"
  password: "aardvark1"
  sslmode: "disable"

display:
  batch_window_size: 10
  default_viewing_mode: "sequential"
  auto_advance: false
```

**API Endpoints:**
- `GET /` - Main web interface
- `GET /api/batches?start_batch=X&limit=Y&run_id=Z` - Get analysis results for specific experiment run
- `GET /api/experiment-runs` - Get all available experiment runs

**Use Cases:**
- **Research Analysis**: Compare AI insights across different pipeline configurations
- **Parameter Tuning**: See how different settings affect cluster quality and AI analysis
- **Historical Review**: Browse through past experimental runs and their results
- **Quality Assessment**: Evaluate AI analysis quality across different parameter sets
- **Collaboration**: Share results with team members through web interface

**Navigation Tips:**
- **Next Button**: Loads next chronological batch (appears at top)
- **Previous Button**: Loads earlier batch (appears at bottom)
- **Window Slider**: Navigate within current batch window
- **Experiment Dropdown**: Switch between different experimental runs
- **Hover on Analysis**: Click analysis text to see original AI prompt

**Data Requirements:**
- Requires `experiment_runs` table with pipeline configuration parameters
- Needs `ai_analysis_results` table with AI-generated analysis
- Batches must be properly linked to experiment runs via foreign keys
- AI feeder must have processed clusters for the selected experiment run

---

# Other Software

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



# Shell Scripts, Utilities, Tests, etc

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

### 4. JSON Output Analysis (`util_shell/analyze_json_output.sh`)

Comprehensive analysis tool for Twitter pipeline JSON output files. Provides detailed statistics about batches, clusters, tweets, and file structure.

**Usage:**
```bash
./util_shell/analyze_json_output.sh <json_file>
```

**Example:**
```bash
./util_shell/analyze_json_output.sh september_18.json
```

**Features:**
- **File Statistics**: File size, total batches, time range analysis
- **Cluster Analysis**: Min/max/average clusters per batch with standard deviation
- **Tweet Analysis**: Min/max/average tweets per batch with standard deviation
- **Performance Metrics**: Total tweets processed, clusters created, processing rates
- **Data Validation**: File structure validation, error detection
- **Summary Statistics**: Clusters above minimum size, batch time ranges

**Output Example:**
```
=== JSON Output Analysis for: september_18.json ===

📁 File size: 2.3M
📊 Total batches: 45
📈 Cluster statistics per batch:
   Min clusters: 2
   Max clusters: 18
   Average clusters: 8.3
   Standard deviation: 3.2
📈 Tweet statistics per batch:
   Min tweets: 25000
   Max tweets: 25000
   Average tweets: 25000
   Standard deviation: 0
📊 Summary statistics:
   Total tweets processed: 1125000
   Total clusters created: 375
   Average clusters per tweet: 0.0003
⏰ Time analysis:
   First batch: 2012-01-28T15:56:35Z
   Last batch: 2012-01-28T18:23:45Z
📏 Clusters above minimum size analysis:
   Average clusters above min size per batch: 6.1
   Total clusters above min size: 275
🔍 File structure validation:
   Data types found: ["batch"]
   ✅ No error entries found
```

**Use Cases:**
- **Performance Analysis**: Understand processing efficiency and throughput
- **Data Quality Assessment**: Verify cluster generation and tweet processing
- **Experiment Comparison**: Compare results across different parameter sets
- **Debugging**: Identify anomalies in output files
- **Reporting**: Generate statistics for research or documentation

**Dependencies:**
- `jq` (JSON processor) - install with `brew install jq` or `apt-get install jq`
- `bc` (basic calculator) - usually pre-installed on Unix systems

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
cd src && go build -o main . && ./main -config ../config/config.yaml
```

### 2. Real-time Pipeline with Database Loading
```bash
# Terminal 1: Start main Z-filters pipeline (outputs to clusters.json)
cd src && go build -o main . && ./main -config ../config/config.yaml > ../clusters.json

# Terminal 2: Start SQL loader to read the same file in real-time
cd src/sql_loader && go build -o sql_loader . && ./sql_loader "Live Run" ../../config/database.yaml ../../config/config.yaml ../../clusters.json

# Terminal 3: Start AI feeder to analyze clusters as they're loaded
cd src/ai_feeder && go build -o ai_feeder main.go && ./ai_feeder ../../config/ai_feeder.yaml

# Terminal 4: Start AI display to view results in real-time
cd ai_display && go build -o ai_display main.go && ./ai_display ../config/ai_display.yaml
```

**Benefits of this approach:**
- **Real-time data flow**: Data flows from Z-filters → Database → AI Analysis → Web Display
- **Live monitoring**: Watch clusters and AI analysis appear in real-time
- **No waiting**: Start analysis before the main pipeline completes
- **Efficient restart**: Each component can be restarted independently

### 3. Data Analysis
```bash
# Analyze token frequencies
./token_frequency_analyzer -input data/ -interval 10000

# Find specific CSV files
./find_csv_file -dir data/ -datetime "2023-01-15 14:30:00" -n 2

# Add language detection
./language_detector -input raw/ -output processed/ -workers 4
```

### 4. Monitoring and Debugging
```bash
# Tail the latest log
./util_shell/tail-the-log.sh

# Run comprehensive tests
./util_shell/run_tests.sh

# Clean up old files
./util_shell/clean_up_old_run.sh
```

### 5. Performance Tuning
```yaml
# In config.yaml, adjust these for performance:
window: 3000000             # Larger = more memory, better accuracy
batch: 25000                # Larger = faster processing
freq_classes: 24            # More = finer granularity
cleanup_trigger_batch_size: 500  # More frequent = less memory
```



## Parser Scripts

### JSON Parser (`parser/parser.py`) - **DEPRECATED**

> **⚠️ This Python parser is obsolete and has been replaced by the Go-based JSON parser.**
> **Use the Go parser (`src/json_parser/parser.go`) for all new development.**

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

### 3. CSV Filenames to Time (`util_go/csv_file_mapping.go`)

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

### 4. Token Frequency Analyzer (`util_go/token_frequency_analyzer.go`)

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

### 5. Token Examiner (`util_go/examine_tokens.go`)

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