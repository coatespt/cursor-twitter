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
