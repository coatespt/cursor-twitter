# Database Schema Documentation

## Current Schema (NEW)

The current database uses the `new_*` table names. Use `create_new_tables.sql` to recreate the database from scratch.

### Tables:
- `new_experiment_runs` - Experimental run configuration and metadata
- `new_batches` - Metadata for each batch processed by the pipeline  
- `new_clusters` - Cluster information extracted from each batch
- `new_tweets` - Individual tweets within each cluster
- `new_tweet_clusters` - Many-to-many relationship between tweets and clusters
- `new_busy_words` - Busy words identified in each cluster with their frequency classes

### AI Analysis Tables:
- `ai_analysis_sessions` - AI analysis sessions for experiment runs
- `ai_analysis_results` - AI analysis results for individual clusters
- `ai_insights` - AI-generated insights from analysis sessions

## Legacy Schema (DEPRECATED)

The old schema used table names without the `new_` prefix. These are deprecated and should be removed.

### Files:
- `create_tables.sql` - **DEPRECATED** - Creates old table names (batches, clusters, tweets, busy_words)
- `create_new_tables.sql` - **CURRENT** - Creates new table names (new_*, ai_*)
- `drop_old_tables.sql` - Script to remove old tables from the database

## Usage

### To recreate the database from scratch:
```bash
# Drop and recreate the database
PGPASSWORD=aardvark1 psql -h 192.168.1.76 -U petercoates -d postgres -c "DROP DATABASE IF EXISTS x_twitter;"
PGPASSWORD=aardvark1 psql -h 192.168.1.76 -U petercoates -d postgres -c "CREATE DATABASE x_twitter;"

# Create the new schema
PGPASSWORD=aardvark1 psql -h 192.168.1.76 -U petercoates -d x_twitter -f create_new_tables.sql
```

### To clean up old tables from existing database:
```bash
PGPASSWORD=aardvark1 psql -h 192.168.1.76 -U petercoates -d x_twitter -f drop_old_tables.sql
```

## Configuration

All applications should use the `x_twitter` database with the new table names. Configuration files are already updated to point to the correct database and table names.