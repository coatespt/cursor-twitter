# Cursor Principles

Paste this into Cursor when starting a new chat.

Before doing anything that changes code, please ask me any questions you have about this before you begin.

Never, under any circumstances do any git operation for any reason until we have explicitly discussed the operation you contemplate. Assume you misunderstood me.

Don't run the program yourself without asking. The program runs forever and I have to kill it to get your attention back. I will handle running the program.

If you are going to make changes that you cannot reliably rewind without getting further input from me, warn me so that I can commit everything in a working state.

Do not implement anything without explicit approval. Sometimes I just want to talk about an approach. Asking a question or soliciting your opinion doesn't mean that I want to rewrite the codebase!

For every feature, we need to add unit tests.  The tests should be carefully commented about what is being tested and why the pass/fail conditions are what they are.

For any significant code change, we must run the test suite!

When a test doesn't pass, we have to look at why before changing anything. No removing tests because they don't pass unless we agree the test is obsolete.

If any changes seem to involve multiple threads, be sure to get my agreement before doing anything. Anytime there thread safety constructs like mutex, etc., always check. We keep getting into trouble with unnecessary and incorrect thread complexity.


# TTD

## A Metric for Subjects
Playing with the grid display suggests that real subjects in a batch share a lot of busy words with earlier batches if they are going to amount to anything.

Accordingly, is there a metric we could computed based on some combination of the number of Tweets in the cluster and how many busy words it uses that appeared how far back.

What you often see is clear subject that don't seem to persist. But you also see clear subjects that have the same busywords trailing all the way to the left side of the grid. 

Come up with a metric that reflects the average persistence of the busywords over time and the number of Tweets involved.

### An LD-based Metric
We see lots of clusters that keep recurring e.g. "Taylor our queen" but don't show up graphically in the grid.

Search backwards for mediods within some LD or normalized LD of the current cluster's mediod.

For each older batch that matches, highlight the cell. 

### If we have such a metric, reflect it in the display somehow. 
- Backgrond color of the Tweets and/or busywords
- Bold

## Darken the background coloration of every other cluster
It is a little hard to see the cluster demarkations

## Quality Score Implementation

### Function: `calculateQualityScore`
**Location**: `display/display_main.go` (lines ~95-130)

**Purpose**: Computes a composite quality score (0-1) for each cluster based on multiple factors indicating its significance and persistence.

**Formula**: Weighted composite of 5 components:
```
Quality Score = (0.35 × Persistence) + (0.25 × Recurrence Strength) + (0.15 × Size Weight) + (0.20 × Tweet Weight) + (0.05 × Consistency)
```

**Components**:

1. **Persistence** (35%): `appearances / total_historical_batches`
   - Percentage of historical batches where the cluster appears
   - Higher score = more consistent presence over time

2. **Recurrence Strength** (25%): `average_similarity_score`
   - Average similarity of detected recurrences (default 0.7 for detected matches)
   - Based on Levenshtein Distance comparisons with historical tweets/medoids

3. **Size Weight** (15%): `cluster_size / max_cluster_size_in_batch`
   - Normalized cluster size relative to largest cluster in current batch
   - Linear scaling

4. **Tweet Weight** (20%): `log(tweet_count) / log(max_tweet_count)`
   - Logarithmic normalization of tweet count
   - Provides diminishing returns for very large tweet counts
   - More realistic scaling than linear

5. **Consistency** (5%): `recurrence_count / total_historical_batches`
   - How evenly distributed the recurrences are across batches
   - Higher score if recurrences are spread out rather than clustered

**Usage**: 
- Displayed in "Quality" column in grid view
- Shows as decimal (e.g., "0.63")
- Monospace font for easy reading
- Color coding ready (green=high, yellow=medium, red=low)

**Example**: A cluster with score 0.63 indicates strong persistence, good recurrence detection, substantial size, and moderate tweet volume with diminishing returns applied.

