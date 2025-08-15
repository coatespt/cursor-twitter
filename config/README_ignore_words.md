# Word Filter Lists

This directory contains comprehensive lists of words that should be filtered out of the anomaly detection system.

Any .txt file in this directory will be read on startup and matching tokens will be filtered out of the stream used to compute busy words.  

These words are not filtered out of the Tweets themselves. As trashy as it is, it is.

More categories can be added or the existing categories broken up. 

If you DON'T want any or all categories excluded, just reneme them .back or some such.

## File Structure

```
filter_lists/
├── README.md                    # This file
├── repetitive_filters.txt       # hahaha, xxxxx, etc.
├── offensive_filters.txt        # Vulgar language, racism, profanity
├── low_entropy_filters.txt      # Any low-entropy word. "at", "the", etc. 

