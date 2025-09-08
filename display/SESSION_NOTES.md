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

# AI Front End
This is a way to view the subjects as interpreted by the Ollama LLM.

## Putting The main Output Data in Postgres 

We have a process that reads the output of the main news extraction and puts it in a Postgres database.

The SQL is Tweets, Clusters, Batches, and a table of run configurations.  
So you can have multiple runs with different runs in the database and a record of what the key parameters of the run were, e.g., batch size, number of frequency classes, z scores per frequency class, etc.

## Sending the data to Ollama

We have another process that reads the clusters from Postgres and sends them to Ollama via HTTP. Ollama figures out what the cluster (subject) is about and returns a result that the process stores in the database.

Ollama running on the Linux laptop can do about a cluster every fifteen seconds. 

The output we're running against has 25,000 Tweet batches, which would be about five seconds of the firehose, or 50 seconds of the decahose.
- Average clusters per batch 2.34
- Minimum clusters per batch 1
- Maximum clusters per batch 10
- Total clusters in the dataset 1,681
- It has 717 batches.

So, it's 16 seconds * 2.34 is an average of 37 seconds per batch, so with the data reduction of the primary part, we are bottlenecked by Ollama to a little less than 1.5 decahoses. 

Note, the iMac is doing very little here--it's formulating SQL queries that it runs on Postgres, which is running on the Linux machine to obtain clusters. It sends the clusters to Ollama, then inserts the results in SQL.

We happen to be reading from pre-prepared output, but the imac can get the subjects from multiple fire-hoses, so if we wanted to do this realtime, we could, on the imac and the Linux machine doing the AI.

## Front End
The front end


# TTD

## We have a variation on Levenshtein to remove duplicates.
Verify it's functionality with some log lines to show what it removes. Document how to
set the value that indicated a duplicate and how the duplication is resolved.
Not obvious becuase resemblence is not transitive.

## Find out how fast the language gizmo works and make sure it runs multi-threaded. Put the results in the config.yaml doc and in the manual.  If it doesn't run multi-threaded, that would be an important improvement.

## The config notes a number of values that should probably be removed from the config and from the code.  Populate this list here from config.yaml

## Big New Functionality 

Dont implement anything yet. Let's hammer out the details.

All the existing functionality stays where it is.  It's important to understand 
these pieces before building.

We built a piece that reads the JSON output of the main program and puts it into 
a Postgres SQL database.  That seems well in hand.
TODO: Be sure this piece is fully documented in the manual.

### AI Action on SQL Clusters

We have AI-generated analysis for clusters, and a way to group clusters into batches.

We need a Web service that uses SQL to get the AI-generated results and display them. 

The Web Service will take it's instructions from a browser page it puts up.
The browser page is described below.

On the top of the left browser panel, which is tall and narrow, some information about the SQL data will be presented when the browser comes up.
- Available processing runs (Drop down? Right now we only have one)
- For the selected run (should default to the newest, but again, we only have one.)
- Display date time for oldest Batch
- Display date time for newest Batch 
- Time span of the batch (It should be available in the properties. It's 25k for this processing run)
- Number of batches and clusters in the dataset.

Below that on the left browser panel The user will have various options for how to look at the AI analysis.
- The processing run is selected above and defaults to the newest.
- Select the start time (in the range of Oldest Tweet to Newest Tweet)
- Select the viewing mode (from a drop down, but initially we only have one which will be simply stepping through the subjects.)
- Select Automatic v manual advance (Manual means you press next/previous each time, Automatic means they scroll past. Automatic is meaningless unless we are processing live data, which we will not be yet.)
- Next and previous buttons.

Each time the user presses next the next available batch of clusters will be read in and displayed in the grid on the right side.

The right panel is as wide as possible so it can show the full analysis.

Each new batch will be appended, so the previous batches will scroll off the screen to the top. There should be a slider to access a window of up to N previous batches.

The previous button will fetch the batch privious to the earliest we have in the window. There might be no earlier batches if we started at the earliest.

- The output is from oldest in the selected range forward.
- It has a column for processing run, date/time, batch, cluster, with most of the space  reserved for the text, which might have to roll over to the next line in the column. 

So the user can click along, looking at the analysis of the latest subjects batch by batch


### More advanced modes to implement in the future

The front end is interesting to play with, but for commercial use, the principles buy which it works would probably need to be adapted to automation.

- The evaluation of subject significance would be enhanced to identify only the most significant subjects.

- More powerful ways to look back. Once a subject is ID'd, looking back in time for the origins and related subjects might make sense.

- A more compact display emphasizing the medioids with the ability to drill in for a deeper look at certain subjects.

### Answers
1. Yes, we're going against the ai_analysis_results table.  More complex modes will join with tweets, etc.

2. Default should be pull in one batch each time they click next

3. Sequential could be the first mode.  That would mean the clusters for each batch.

4. I think the batch time, and cluster id's plus the analysis text
I think we could enhance this to pop up the actual tweet texts if you hover over it.

5. We'd have a window of up to N batches that have been read in by pressing next. You could just use the window bar to move back that far.  When you get to the earliest in the window, privious would get the next earlier, etc.  Unless there are no earlier, in which case it does nothing.

6. I think we only need to worry about the data we've pre-stored now, but if it were processing live data it wouldn't be very different. The only thing is it would probably not need the next button to move forward.

7. I was thinking the next button would do it all, but we could read in the whole window of N batches.  That's a good idea. I'm picturing N being maybe 10? 

### More Answers
1. I think the new directory is a good idea. It's a very different piece--a web server.

2. I think a different config too. For the same reason.

3. How about 8081?  Is there any way to tell what might be taken?

4. Yes, that's a good idea.


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
