# Persistence Functions Log

This document tracks the persistence (save/load) functions we're building for the new scalable frequency counting mechanism. These functions are currently only used by test code and will be integrated into the main pipeline later.

## Completed Functions

### 1. TokenCounter Persistence ✅

**File**: `src/pipeline/tokencounter.go`

**Functions**:
- `SaveToFile(filename string) error`
- `LoadFromFile(filename string) error`

**Purpose**: Save/load the current token count map (map[string]int)

**Usage**:
```go
tc := pipeline.NewTokenCounter()
tc.IncrementTokens([]string{"hello", "world", "hello"})

// Save
err := tc.SaveToFile("/path/to/token_counts.gob")

// Load
tc2 := pipeline.NewTokenCounter()
err := tc2.LoadFromFile("/path/to/token_counts.gob")
```

**File Format**: Binary gob encoding
**Thread Safety**: Uses existing TokenCounter mutex
**Tests**: `src/pipeline/tokencounter_persistence_test.go` (6 test cases)

---

## Planned Functions (Not Yet Implemented)

### 2. Frequency Filter Persistence 🔄

**File**: `src/pipeline/frequency_classes.go`

**Functions Needed**:
- `SaveFiltersToFile(filename string) error` (on FreqClassResult or similar)
- `LoadFiltersFromFile(filename string) error`

**Purpose**: Save/load the frequency class filters (SetFilter and BloomFilterWrapper)

**Usage** (planned):
```go
result := BuildFrequencyClassHashSets(tokenCounts, F, bloomSizes, hashCounts)
err := result.SaveFiltersToFile("/path/to/frequency_filters.gob")

// Load
var loadedResult FreqClassResult
err := loadedResult.LoadFiltersFromFile("/path/to/frequency_filters.gob")
```

### 3. Three-Part Key Mapping Persistence 🔄

**File**: `src/main.go` (or new file)

**Functions Needed**:
- `SaveThreePKMappings(filename string) error`
- `LoadThreePKMappings(filename string) error`

**Purpose**: Save/load the global token-to-3PK and 3PK-to-token mappings

**Usage** (planned):
```go
// Save global mappings
err := SaveThreePKMappings("/path/to/threepk_mappings.gob")

// Load global mappings
err := LoadThreePKMappings("/path/to/threepk_mappings.gob")
```

### 4. Token Log File Management 🔄

**File**: New file (e.g., `src/pipeline/token_log_manager.go`)

**Functions Needed**:
- `WriteTokenLog(tokens []string, filename string) error`
- `ReadTokenLog(filename string) ([]string, error)`
- `ManageTokenLogWindow(logDir string, maxFiles int) error`

**Purpose**: Manage the rolling window of token log files

**Usage** (planned):
```go
// Write tokens to log file
err := WriteTokenLog(tokens, "/path/to/tokenlog_00001.dat")

// Read tokens from log file
tokens, err := ReadTokenLog("/path/to/tokenlog_00001.dat")

// Manage rolling window
err := ManageTokenLogWindow("/path/to/logs", 10) // Keep 10 files
```

---

## Configuration

**File**: `config/config.yaml`

**Added**:
```yaml
persistence:
  state_dir: "../data/state"
```

**Planned Additions**:
```yaml
persistence:
  state_dir: "../data/state"
  token_log_dir: "../data/token_logs"
  token_log_chunk_size: 50000  # tokens per file
  max_token_log_files: 20      # rolling window size
  save_interval_seconds: 300   # how often to save state
```

---

## File Naming Convention

**Current**:
- Token counts: `token_counts.gob`

**Planned**:
- Frequency filters: `frequency_filters.gob`
- 3PK mappings: `threepk_mappings.gob`
- Token logs: `tokenlog_00001.dat`, `tokenlog_00002.dat`, etc.

---

## Integration Plan

1. ✅ **TokenCounter persistence** - COMPLETE
2. 🔄 **Frequency filter persistence** - NEXT
3. 🔄 **3PK mapping persistence** - AFTER
4. 🔄 **Token log management** - AFTER
5. 🔄 **Integration into FCT** - FINAL STEP

---

## Notes

- All functions use Go's `encoding/gob` for binary serialization
- All functions are thread-safe where needed
- All functions have comprehensive test coverage
- Functions are designed to be non-intrusive to existing code
- File paths are configurable via `config.yaml` 