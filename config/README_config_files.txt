

Absolute pathnames have been removed. Paths in the configs are now relative.

Note that the main config is in this directory, but the display program
has its own config directory and config file.

However, you may want specific versionf of the config file for testing 
with various parameter settings. Note the following:

# Run with base config only (normal operation)
./main -config config/config.yaml

# Run with base config + override for experiments
./main -config config/config.yaml -override config/experiments/high_freq.yaml

# Run with base config + different override
./main -config config/config.yaml -override config/experiments/low_threshold.yaml

