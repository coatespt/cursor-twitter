# Configuration Path Resolution

## Overview

The application now supports **relative paths** in configuration files. All relative paths are resolved relative to the **config file's location**, not the current working directory.

## How It Works

### Before (Absolute Paths)
```yaml
# config.yaml
log_dir: /home/petercoates/cursor-twitter/logs
file_src_dir: /home/petercoates/cursor-twitter/data
```

### After (Relative Paths)
```yaml
# config.yaml
log_dir: ../logs
file_src_dir: ../data
```

## Benefits

1. **Portable Configs**: Config files work on any machine without modification
2. **Multiple Environments**: Easy to have different configs for different setups
3. **No Hardcoded Paths**: No need to update absolute paths when moving the project
4. **Cleaner Configs**: Shorter, more readable path specifications

## Usage

### Main Pipeline
```bash
# Run from project root with base config
./twitter-pipeline -config config/config.yaml

# Run with base config + override for experiments
./twitter-pipeline -config config/config.yaml -override config/experiments/high_freq.yaml

# Run from any directory
./twitter-pipeline -config /path/to/config.yaml -override /path/to/override.yaml
```

### Display Server
```bash
# Run from display directory
./cursor-twitter-display

# Run with custom config
./cursor-twitter-display /path/to/config.yaml
```

## Path Resolution Examples

| Config Location | Relative Path | Resolved Path |
|----------------|---------------|---------------|
| `/project/config.yaml` | `../logs` | `/project/logs` |
| `/project/config.yaml` | `data` | `/project/data` |
| `/project/config.yaml` | `/absolute/path` | `/absolute/path` (unchanged) |
| `/project/subdir/config.yaml` | `../../logs` | `/project/logs` |

## Supported Path Fields

### Main Pipeline (`config/config.yaml`)
- `log_dir`: Log directory
- `file_src_dir`: Input file directory
- `filter.offensive_filters_file`: Filter file paths
- `filter.repetitive_filters_file`: Filter file paths
- `filter.low_entropy_filters_file`: Filter file paths
- `filter.banned_phrases_file`: Filter file paths
- `filter.useless_busywords_file`: Filter file paths

### Display Server (`display/config.yaml`)
- `input_file`: Input JSON file path

## Config Overrides

The pipeline supports config overrides for easy experimentation. You can create override files that contain only the parameters you want to change, and the system will merge them with the base config.

### How Overrides Work

1. **Base config**: Contains all common/default settings
2. **Override config**: Contains only the parameters you want to change
3. **Result**: The program uses all settings from base config but with override values applied

### Example Override Files

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

### Benefits of Overrides

- **Minimal files**: Only specify what you're changing
- **Easy tracking**: Each override file focuses on specific parameters
- **No duplication**: Don't repeat all the common settings
- **Clear intent**: Obvious what each experiment is testing

## Migration Guide

1. **Update existing configs** to use relative paths
2. **Test the changes** to ensure paths resolve correctly
3. **Update any scripts** that reference config files
4. **Create override files** for experimental parameters

## Example Config Files

- `config/config.yaml`: Main config with relative paths
- `display/config.yaml`: Display server config with relative paths
- `config/experiments/high_freq.yaml`: Example override for high frequency classes experiment
- `config/experiments/low_threshold.yaml`: Example override for low threshold experiment
