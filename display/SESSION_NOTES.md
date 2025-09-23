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

## Cursor has duplicated definitions of structs everywhere.

We have removed all structs from display and put them in types.

Next, remove all structs from SQL Loader that are used outside that program and have it refer to types.



## The bubbles need scaling. They are too close in size. 
But they are different in size--this is cosmetic.  What is the right function to apply?

I think cursor did something wacky. The Z's will rarely be less than four and can go to double digits. Need to know what range of sizes the bubbles need.

The count ratios are more extreme. They might need logs.

## Grid View Doesn't Work


## Medoid View Doesn't Work

## Instructions
- The JSON is correctly parsed into a struct that is batch with a set of clusters and some metadata fields.  The cluster is composed of a set of tweets, some metadata fields, and a set of busyword objects which each has five fields.
- The bubble handler uses these structs and we have verifed that they are corrrect.


## Remove barrier time logs.
Should I remove the barrier timing logs since they're not revealing a real problem?

## Examine this log line. 
time=2025-09-10T08:57:11.370-04:00 level=WARN msg="Large time span in cluster" cluster_id=1 time_span_seconds=384 earliest="2012-01-30 08:23:53 UTC" latest="2012-01-30 08:30:17 UTC" cluster_size=98

Six minutes seems like a very long time.

## Look at this log line
time=2025-09-10T08:57:10.201-04:00 level=INFO msg="BusyWordProcessor released from barrier" class_index=15 batch=376 barrier_wait_time_ms=848

The barrier_wait_time seems very long. 848ms. They all seem to be the same, so that must be cluster processing time.

## Override File
Need a test for every single value in the override file.


## Empty Batches
We have lots of batches that have tweets but no clusters.  If a batch had no clusters but it had not tweets, that would be find--nothing says there have to be any busy words.  But if it has tweets it should have at least one cluster.

If the clustering algorithm produces no clusters, and there are tweets, we need to put in a cluster with all those tweets.

The clustering algorithm seems to sometimes find no clusters even though there are tweets. Can you look at the docs and the API and see if this is a thin? And maybe there's a flag to require that it always produce at least one cluster if there are tweets.

##  Fixing the database.

### The basic insertion of zfilters data
We don't have to use these exact names
- runs have a name like "DB_CLEAR_TEST" or "PTC_window_size_test" 
   - This is just a name, but it should refer to a set of key parameters for the run. This could even be one of the config override files. Detail to be decided
   - A run has a run_id that is a globally unique key
- batches have a meaningless key primary key. 
   - They also have a run_id foreign key that must exist to link to a run.
- A cluster has a globally unique primary key cluster_id 
   - It also must have a FK of the cluster_id it is part of 
- A tweet has a globally unique meaningless tweet_id.
   - A tweet in threory could be part of multiple clusters.
   - This means it should be linked to a cluster by a linkage table something like {}

When we start putting AI tables in for the Ollama output the rows should also have globally unique meaningless keys, but they must have FK's that are the cluster they are associated with.

Do we need busy words? I think not.

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

## The Barrier
There was no proper barrier construct holding the busyword processors and the analytics thread in lock step. 

Both are quite fast. The BWP's usually run in under a millisecond, occasionally as long as two or three.  The clustering is slower 