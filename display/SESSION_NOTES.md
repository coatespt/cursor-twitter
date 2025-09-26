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

## The AI cluster history display seems to work now.
- It works, but the history seems really weak. The clusters seem pretty unrelated.  This is probably a matter of parameterizing the run differently.
- I think there are too many busy words. Try raising all the minimum Z values.
- We may need a utility to compute stats on this


## Bubble View

### Testing What is There
There are two different bases for scaling the bubbles.  The following aspects need testing
- The choice is in config 
- The parameters for each that tune the display. 
- The ability to override the values int he override file needs to be tested

### Bubble scaling choice and params should be on the screen 

### When you get to the end of the input file, the bubble view can't back up.

### Find out what the divergence value in the bubble display means exactly.
It seems to be in proportion to the size of the bubble.

### The bubbles are too big and they size range is to narrow.

## Barrier Issues 

###Should I remove the barrier timing logs since they're not revealing a real problem?

### Examine this log line. 
time=2025-09-10T08:57:11.370-04:00 level=WARN msg="Large time span in cluster" cluster_id=1 time_span_seconds=384 earliest="2012-01-30 08:23:53 UTC" latest="2012-01-30 08:30:17 UTC" cluster_size=98

Six minutes seems like a very long time.

## Is This Log Line Obsolte?
time=2025-09-10T08:57:10.201-04:00 level=INFO msg="BusyWordProcessor released from barrier" class_index=15 batch=376 barrier_wait_time_ms=848

The barrier_wait_time seems very long. 848ms. They all seem to be the same, so that must be cluster processing time.

## Override File
Need a test for every single value in the override file.

## Empty Batches--Is this still an issue?
We have lots of batches that have tweets but no clusters.  If a batch had no clusters but it had not tweets, that would be find--nothing says there have to be any busy words.  But if it has tweets it should have at least one cluster.

If the clustering algorithm produces no clusters, and there are tweets, we need to put in a cluster with all those tweets.

The clustering algorithm seems to sometimes find no clusters even though there are tweets. Can you look at the docs and the API and see if this is a thin? And maybe there's a flag to require that it always produce at least one cluster if there are tweets.

## A Metric for Subjects
This metric now exists and is displayed, but it is a first cut. Various enhancements are possible. Details to be decided.

It currently is a function of the number of Tweets in the cluster, how long its busy words have persisted over previous batches, the LD of its medioid from the Tweets in earlier batches, etc.

The score is computed thusly:
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


# Some Lessons Learned