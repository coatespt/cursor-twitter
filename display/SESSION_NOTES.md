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

## The Front End is Exploratory
The front end is interesting to play with, but for commercial use, the principles buy which it works would probably need to be adapted to automation.
- The evaluation of subject significance would be enhanced to identify only the most significant subjects.
- More powerful ways to look back. Once a subject is ID'd, looking back in time for the origins and related subjects might make sense.
- A more compact display emphasizing the medioids with the ability to drill in for a deeper look at certain subjects.
- Semantic analysis. The subject identification is really a form of data reduction.  It should be possible to apply NLP and/or AI to this greatly reduced data set to find:
    - Human understandable description of what's up
    - ID related subjects 
    - Flag subjects for further attention
    - etc

## Testing is Incomplete

## Put a sample input data set in Git and indicate it in the manual.

## Put the config.yaml in a config directory

## Make the config file to be used a command line option

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
