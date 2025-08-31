# SQL Loader for Twitter Pipeline

This program reads the JSON output from the main Twitter pipeline and loads it into a PostgreSQL database for analysis and AI processing.

## Features

- **Experiment Tracking**: Each data load is associated with an experiment run containing all configuration parameters
- **Duplicate Prevention**: Automatically skips batches and clusters that already exist
- **Configurable Limits**: Option to cap the number of tweets per cluster to manage database size
- **Data Validation**: Warnings for anomalous data (clusters with no busy words, etc.)
- **Config Override Support**: Can use experimental configurations with the override system

## Usage

### Basic Usage
```bash
./sql_loader "Run Name" ../../config/database.yaml ../../config/config.yaml
```

### With Override Config
```bash
./sql_loader "High Freq Test" ../../config/database.yaml ../../config/config.yaml ../../config/experiments/high_freq.yaml
```

### With Specific JSON File
```bash
./sql_loader "Test Run" ../../config/database.yaml ../../config/config.yaml ../../data/august_12_clusters.json
```

### Complete Example
```bash
./sql_loader "High Freq Test" ../../config/database.yaml ../../config/config.yaml ../../config/experiments/high_freq.yaml ../../data/august_12_clusters.json
```

## Database Schema

The database includes these main tables:

- **experiment_runs**: Tracks each experimental run with all configuration parameters
- **batches**: Batch metadata linked to experiment runs
- **clusters**: Cluster information within batches
- **tweets**: Individual tweets within clusters (with medoid marking)
- **busy_words**: Busy words identified in each cluster with frequency classes

## Configuration

### Database Configuration (`config/database.yaml`)
```yaml
database:
  host: 192.168.1.76
  port: 5432
  name: x_twitter
  user: petercoates
  password: aardvark1
  ssl_mode: disable

# Processing options
max_tweets_per_cluster: 50  # Limit tweets per cluster (0 = no limit)
validate_data: true         # Enable validation warnings
```

### Pipeline Configuration
Uses the same configuration files as the main pipeline (`config/config.yaml` and override files).

## Database Management Scripts

- **`create_tables.sql`**: Creates all tables with proper constraints
- **`drop_tables.sql`**: Removes all tables and views
- **`clear_database.sql`**: Clears all data while preserving schema
- **`fix_permissions.sql`**: Grants necessary permissions to the database user

## Experiment Tracking

Each data load creates an experiment run record containing:
- Run name (provided by user)
- Timestamp
- All key configuration parameters (window size, Z-scores, thresholds, etc.)
- Arrays stored as strings for easy querying

This enables:
- Comparing results across different parameter sets
- Tracking which configuration produced which results
- Historical analysis of parameter effectiveness

## Building

```bash
go build -o sql_loader main.go
```

## Dependencies

- Go 1.21+
- PostgreSQL database
- `github.com/lib/pq` (PostgreSQL driver)
- `gopkg.in/yaml.v3` (YAML parsing)
- `cursor-twitter/json_parser` (shared JSON parsing)
