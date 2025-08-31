# AI Feeder for Twitter Pipeline

This program reads cluster data from the PostgreSQL database, sends it to AI services (like Ollama) for analysis, and stores the results back in the database.

## Features

- **Database Integration**: Reads cluster data from the pipeline database
- **AI Service Support**: Interfaces with Ollama and other AI services
- **Template-Based Prompts**: Uses customizable prompt templates
- **Session Tracking**: Tracks analysis sessions with progress monitoring
- **Retry Logic**: Handles AI service failures with configurable retries
- **Batch Processing**: Configurable batch sizes and processing limits
- **Progress Monitoring**: Real-time progress updates and session status

## Architecture

### Database Schema
The AI feeder extends the pipeline database with these tables:

- **`ai_analysis_sessions`**: Tracks analysis sessions for experiment runs
- **`ai_analysis_results`**: Individual AI requests and responses for clusters
- **`ai_insights`**: Structured insights extracted from AI responses

### Data Flow
1. **Read**: Extract cluster data from PostgreSQL
2. **Process**: Generate prompts using templates
3. **Analyze**: Send to AI service (Ollama)
4. **Store**: Save results back to database
5. **Track**: Monitor progress and session status

## Configuration

### AI Service Configuration
```yaml
ai:
  model: "llama3.1:8b"                    # AI model to use
  endpoint: "http://localhost:11434/api/generate"  # Ollama API endpoint
  timeout: 60                              # Request timeout in seconds
```

### Processing Configuration
```yaml
processing:
  batch_size: 1                           # Clusters to process in parallel
  max_retries: 3                          # Max retries per request
  retry_delay: 5                          # Seconds between retries
  prompt_template: "prompts/cluster_analysis.txt"  # Prompt template file
  analysis_type: "cluster_summary"        # Type of analysis
  session_name: "Cluster Analysis Run 1"  # Session name
  run_id: 1                               # Experiment run ID to analyze
  max_clusters: 100                       # Max clusters to process (0 = all)
  start_from_cluster: 1                   # Start from specific cluster ID
```

## Usage

### Basic Usage
```bash
cd src/ai_feeder
go build -o ai_feeder main.go
./ai_feeder ../../config/ai_feeder.yaml
```

### Prerequisites
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

## Prompt Templates

The AI feeder uses Go templates to generate prompts. Available template variables:

- `{{.ClusterID}}`: Cluster identifier
- `{{.BatchNumber}}`: Batch number
- `{{.BatchTime}}`: Batch timestamp
- `{{.Size}}`: Number of tweets in cluster
- `{{.QualityScore}}`: Cluster quality score
- `{{.FrequencyClass}}`: Frequency class of busy words
- `{{.BusyWords}}`: Array of busy words
- `{{.MedoidTweet}}`: Representative tweet
- `{{.Tweets}}`: Array of all tweets in cluster

### Example Template
```text
Analyze this Twitter cluster:

Cluster ID: {{.ClusterID}}
Size: {{.Size}} tweets
Busy Words: {{range .BusyWords}}{{.}} {{end}}

Representative Tweet:
{{.MedoidTweet}}

Provide analysis of the main topic and sentiment.
```

## Monitoring

### Session Status
Check analysis session status:
```sql
SELECT session_id, session_name, status, processed_clusters, total_clusters 
FROM ai_analysis_sessions 
WHERE run_id = 1;
```

### Results
View AI analysis results:
```sql
SELECT c.cluster_id, ar.response_text, ar.processing_time_ms
FROM ai_analysis_results ar
JOIN clusters c ON ar.cluster_id = c.id
WHERE ar.session_id = 1
ORDER BY c.cluster_id;
```

## Error Handling

- **Retry Logic**: Failed requests are retried with exponential backoff
- **Session Recovery**: Sessions can be resumed from where they left off
- **Progress Tracking**: Real-time progress updates prevent data loss
- **Error Logging**: Detailed error messages for debugging

## Performance

- **Batch Processing**: Configurable parallel processing
- **Connection Pooling**: Efficient database connections
- **Timeout Handling**: Prevents hanging requests
- **Memory Management**: Streams large responses efficiently

## Building

```bash
cd src/ai_feeder
go build -o ai_feeder main.go
```

## Dependencies

- Go 1.21+
- PostgreSQL database with AI analysis schema
- Ollama or compatible AI service
- `github.com/lib/pq` (PostgreSQL driver)
- `gopkg.in/yaml.v3` (YAML parsing)
