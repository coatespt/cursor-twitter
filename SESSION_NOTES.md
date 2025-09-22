# Session Notes

The document is about nitty gritty practicalities in development.

Specifically, it contains:

- Loud warnings to Cursor not to mess with any code without asking
- Notes on how it works
- Notes on things learned, enhancements, improvements, bugs fixed, etc.
- TTD for fixes, enhancements, analysis, etc.
- Some miscellaneous details on functionality 
- Some details on data volume and issues

Other documentation:

- See PROJECT_DESCRIPTION.md for an overview and more abstract concerns.
- See USER_MANUAL.md for how to run the software


# Cursor Principles 

Please read #Cursor Principles in SESSION_NOTES.md before doing anything.

After I ask you to read this, do not inform me about understanding it unless something is unclear. 

Absolutely no debug, logging, informationals or other should be written to the main pipeline's standard out. STDOUT is only for the program output. Only standard error should be used for this kind of information.

Never make a code change without explicitly asking me. That means per-change.  Do not so much as add a single whitspace without explicit permission. 

If something seems to require any change beyond the strictest possible interpretation of what you are asked, stop immediately. Do nothing at all until you clarify what you think needs doing and get my buy-in.  

Never, under any circumstances do any git operation that modifies anything in the filesystem for any reason until we have explicitly discussed the operation you contemplate. If you think you should do a git operation, automatically assume **every** time that you have misunderstood me and verify that I know what suicidally insane act you are about to undertake. 

Don't run the program yourself without asking. The program runs forever and I have to kill it to get your attention back. I will handle running the program. 

If you are going to make changes that you cannot reliably rewind without getting further input from me, warn me so that I can commit everything in a working state.

Do not implement anything without explicit approval. Sometimes I just want to talk about an approach. Asking a question or soliciting your opinion doesn't mean that I want to rewrite the codebase on your own. Assume I am just asking a question unless I tell you to write code and/or you ask if you want code written.

For every feature, we need to add unit tests.  The tests should be carefully commented about what is being tested and why the pass/fail conditions are what they are.

For any significant code change, we must run the test suite! 

When a test doesn't pass, we have to look at why before changing anything. No removing tests because they don't pass unless we agree the test is obsolete.

If any changes seem to involve multiple threads in any way, or includes anything remotely related to concurrency protections, for instance, mutexes, be sure to get my agreement before doing anything. If you intend to write or touch code might do anything that can be seen by other threads, ask and ask again, then verify that I fully understand exactly what you propose to do.

Apparently you are under the impression that I want happy sunny output. No! Be as critical of what I say as possible.

# TTD 
## Bubble View
The bubble view computes the size of the bubbles completely wrongly. 
The bubble size is computed as a direct function of the frequency class. Divergence of a word's frequency from the mean isn't even part of it!

The solution requres that we compute the information necessary to show this in the BWP's and pass it into the analytics thread as part of the BusyWordClass struct. Something like this:
- In the BWP's we currently sift the busyword 3pk's out of the cartesian product of the high-z-score counts.
- At that point, we have all we need:
  - The 3pk's 
  - The z-scores for each of their keys (indices) 
  - The counts for each of their keys (indices)
  - The mean of all the counters 
- The 3pk values are the indexes, so we can get the Z-scores.

Note that more than one busyword can land on an index so the three Z-scores for a given token might not be approximately the same. However, it is unlikely that more than one busyword will land on all three. Therefore, we take the lowest of the three z-scores for the three indices.

Add fields to the BusyWordClass for
- Z-Score (lowest of the three)
- Mean count 
- Actual count (lowest of the three)
This can be done without changing anything that now exists--we are just adding three fields to the busyword object we pass to analytics.

In the analytics phase, these three values should be added to the busyword in the JSON output.

This adds data to the JSON, and must be properly parsed and put in the SQL that is stored for the busyword.

In the non-AI display, it must be parsed out made available in the bubbles.

We can take these things one at a time.  
- In the BWP's we can obtain the values and print them out in a debug statment to get it right before modifyinng the object passed to Analytics.
- Test to be sure we didn't break anything
- We modify type BusyWordClass struct to have three more fields: 
  - The lowest z-score or the 3pk 
  - The mean for all counters
  - The lowest counter value for 3pk
- Test to be sure we didn't break anything
- We pass them to Analytics in the busyword object.
- Test to be sure we didn't break anything
- Then we can modify the JSON
- Test to be sure we didn't break anything
- When the JSON passes muster, then we can modify the display to show the correct divergence of the busyword from the typical value for the class.
  - There are multiple possibilites for computing this
  - E.g., make the diameter a function of the Z
  - Make the diameter a function of the ratio of mean to actual
  - Probably we should offer a choice in config
- Test to be sure we didn't break anything
- When that all works, we can modify the SQL Loader
- Test to be sure we didn't break anything
- When it is going into Postgres correctly, we can do anything necessary in the AI_loader part. (this may not need changes)
- Test to be sure we didn't break anything

  

  

## Verify that the sequential screen of the AI display is doing the right stuff.

## There are somethign like eighty functions that are unused or duplicated. 
As of september 21 there are 12,697 lines of Go in the pipeline compilation.  We'll see how many can be removed.

jaccard() is an example. 

## Do we want these log lines gone?
*** FCT REBUILD STARTED at 18:26:01 ***
*** FCT REBUILD COMPLETED at 18:26:02 (duration: 857.015865ms, token files written: 473430) ***

## The Config Override System is a mess.
This has supposedly been fixed and any value can go in the override file. We shall see.

## Rabbit Not Tested With Refactored Code

## Refactor the insanely large main into a reasonable structure.

This is ongoing. It has been shrunk by about 1/3 already.

## Global configs on the graphical output. 
It would be nice to see global parameters on the output screen so you could see things like:
- How big a batch is.
- How much clock time is represented by each batch
- Some values we aren't yet computing or displaying like, the quality of a cluster.

## Clustering Improvement By Weighting Frequency Classes
Would clustering be improved by weighting the frequency classes?

- The rarer the word class, the more it is worth?
- Or possibly the opposite?

Not too hard to do if it seems worth it.

- Make the frequency filters accessible to the analytic thread
- Assign a weight to each busyword based on its F class
- Weighted edges are amost certainly part of the graph algorithm. We probably use them now, but just defaulted to 1.

Danger is, it's another configuration matter to try to figure out how to tune.
There are a lot of them!

## Meta-Cluster Quality Not Great
See clustering improvement above.
- We now have a choice between clustering on the busy words or clustering on all tokens. 
- Investigate exactly how this is done. 
- Does "all teh words" mean just the medioids or does it mean all the Tweets?

## Dynamic Adjustment of Z Minima 
Sometimes you see a gross inflation of the number of busy words. It is not clear why.  A facility to dynamically adjust the Z values to keep them at some optimim number might be useful.

- Take the Z minima from config as a starting point 
- Bump them up and down to seek a level that results in an average of B busywords for each frequency pipeline.

### Considerations and Risks
- There is considerable variance across batches 
- There is considerable variance among batches
- We don't actually know which frequency classes characterize clusters the best

We save the busy words sets, so we could do some statistical analysis of them.  We might need to add to that to correlate them with the clusters.

## Possible logic error
We sometimes get notification in the logs of a burst of 3pk's not mapping to tokens. 
This should be almost impossible (it says so right in the warning message.) 'Sup with that?

Suspect this is a phenomenon of startup from token counts on disks.
This needs to be confirmed.
 
## Ongoing Items

- Comments Key areas need to be commented to keep out, don't touch, etc.

- Testing has been neglected. Make tests around everything.

## Major: Improving Busy Word Detection Quality by Computing Redundantly

This would be a significant effort.  Interesting idea, but it's not 100% clear that it's worth doing. We need some investigation.

Consider that dual sets of pipelines, or even three sets, each with different hash functions, could do a much more accurate job of filtering out the true busy words for a given set of parameters.  
 
It is not clear how big an impact this would have on performance. All the BW processors are doing is counting and periodically computing Z on a thousand values in each of the three counter arrays. 

You'd have to 
- Maintain 3 copies of the 3pk mappings
- Three sets of queues
- Three sets of busyword processors 
- Add a piece to the analysis thread to take all three sets of sets of busywords and and decide on what the final set is. 
- After that, it's the same.

Risk. If multiplying the work in the busyword processors made them in aggregate slower than the combined main pipeline and the clustering, it would cause the queues to grow without bound and crash the program. You'd need to detect the problem and throttle the reads if this is a problem. Actually, this should probably be done anyway! Who knows if some combination of config parameters could cause this to happen.

## Major: Clustering Across Batches

Needs to be investigated for quality.

Jacquard similarity for the clustering seems to be applied to all the tokens in the Tweets. 
  - Is that true, and if so, is it correct? This may have been done.
  - Maybe it should be only on the busywords. 
  - Jacquard similarity might not be important. Maybe just the raw occurrences?

## Consider Stripping K-Means Out Entirely
It doesn't do any harm, but we're never going to use it and it could be confusing.
   
# Input and Output

The input available for development purposes is two weeks of the decahose (about 500 Tweets/second.) It consists of JSON-formatted Tweets in files that have the file order encoded in the filenames as Unix start and end times. The files are about five minutes of Tweets at about 500 Tweets/second. The original files are unpacked into uncompressed CSV files.  

Calendar and clock time are encoded into the Tweet data at the decahose rate, but the Tweets can be consumed at any desired rate--the application consumes them as fast as they are supplied. 

In production the input could be either files (for historical processing) or the actual live Tweet stream (for real time processing).  

Using files is not a "cheat" because any system would typicaly receive Tweets and feed them somehow. The actual development feed is from a process running on the same meachine and sending the Tweets via RabbitMQ. In a real deployment, a separate machine would probably handle receiving the data, storing it and parsing it for consumption by the application.

The non-graphical output is a series of clustered sets of Tweets that are about new subjects. There is not yet a graphical front end.

# Lessons Learned and Etc.
This section covers issues encountered and resolved, and lessons learned.

## Batch Size
Was getting good results with batch size 25000. Shifted to batch size 12500 and got terrible results. 

The problem was several things. For one, the interval between FCT recomputations of the partitioning data structure had accidentally been set to 0, so it was furiously recomputing as fast as it could.

One symptom of this was excessive growth of the token queue. It was getting logged as something like 40m tokens.  It normally should be 0 to 400, give or take.

At first it seemed that below a certain size batch, the time spent finding busywords and clustering lets the ingestion outrun the z-filtering, and throttling is doing its job.

More investigation showed that this was not the problem.

One problem was that the allowed length of the busyword processor queues was too short. Even a slight slowness could cause the queue lengths to get too big resulting in throttling. The second problem was that the throttling was set to 20,000 ms. Crazy amount. That meant every one of the 25 busyword queues would fire off a 83ms sleep. 

Adjusting that value way down eliminated the problem.

## 3pk's not mapped
We were getting occasional messages that a 3pk could not be mapped to a value.

The problem is, a token can qualify as a busy word and have it's count go to zero again and get removed from the mapping in the space of one batch if everthing lines up right. 

Every time that FCT computes a new partitioning datastructure, it tracks the number of tokens that have zero counts and removes them from the counter map it maintains. So it knows how many tokens become obsolete each time. It takes that number, multiplies by  




## Time v Time
Note that reading the decahose faster isn't equivalent to reading the firehose because the relationship to wall clock time changes.  A batch size of 25,000 is fifty seconds of the decahose, but only five seconds of the firehose.  Need to quantify how this should affect the settings. 


### To Try
- Raising the batch size seems to be a good thing. Try raising it even more.
  - 25,000 is 50 seconds of the decahose, 5 seconds of the firehose. So, one could probably go considerably higher with the firehose.
- Smaller batch size means higher variance so a bigger spread from the mean would be normal. That would mean you need to lower the Z threshold, right? 
- We're doing this with the lang=en. Try lang=all.

### Heavy string operations affect throughput
When running Cursor simultaneously witht he main, throughput went way down. Interesting question why. Could be disk contention or it could be CPU. Monitoring the backlog of the busyword queues would be revealing.

If it is CPU contention, you won't should see the queues backing up, while if it's disk contention, you won't because the busyword processors are starved.

Not sure what happens if the analysis thread is overwhelmed.


# The Heuristic
This section explains the core heuristic for finding the busywords at a minimal level to make fixit items comprehensible.

## Word Frequency Computations

Global relative frequency calculations occur, but they are relatively infrequent and not in the in the main processing path.  

This is done to create a set of filters that are used to partition the flow of words into F frequency classes, with the most frequently used words in the first class and the least frequent words in the last class.  
The recalculations are on the order of minutes apart and are invisible to the main processing pipeline.

Each class represents the same amount of word usage (the filters are just hashed sets.) F is a configuration item. A value between 7 and 28 is reasonable. 


This frequency partitioning filters are computed over a sliding window of Tweets. The size is configurable, but it is typically a few million. This is necessary to lower the variance in frequency. 

Every so often, again configurable but typically every few minutes, as set of filters is computed and transparently swapped into the function used by the main processing pipeline to partition the token stream. 

Note that the size of the window and the frequency of the re-computation of the filters are two different things. The frequency filters might be recomputed many times over the duration of the window of Tweets. 

Note also that the periods of the cycles of computing busywords and clustering are independent of both. The big window of tokens might cover 500k Tweets, but the filters might be recomputed every 100k Tweets, while the busyword/clustering might be computed every 15k Tweets. All values configurable, of course.
 
New words come along all the time, even after weeks of the firehose and a novel word won't match to any frequency class the first time it is seen. This is not a problem, however. The window is large, so any incoming word that doesn't match to a frequency class is by definition among the rarest class. When the next update to the frequency class filters is occurs, the novel word will no longer be an unknown.

## Receiving Tweets
The main routine reads inbound Tweets from RabbitMQ in CSV format. This detail is a convenience because the Tweet data is canned. Any medium could be used including directly reading data from disk.

The main pipeline maintains an in-memory sliding window of the latest Tweets. This is not to be confused with the token window used for frequency analysis. The Tweet window is much shorter (a few minutes at most) and holds the entire Tweet struct for every Tweet that contributed to the last several cycles of busy-word processing. The Tweets are continually aged out and discarded to limit the size in memory.

The pipeline extracts the words from each incoming Tweet and normalizes them. This include unifying the case, dealing with diacritics and such, discarding junk words, etc. The result is a stream of meaningful tokens.

It puts the cleaned-up tokens on a queue for the off-line frequency calculations to use.

The frequency class filters are used to frequency class for each token. 

A "three part key" (3pk) is looked up for each token. If a 3pk doesn't yet exist, one is computed and inserted it into the lookup table. More on 3pk's later.

Each  3pk is put on a busyword queue for the token's frequency class.

When one "batch" (perhaps 10 or 20k) of incoming Tweets has been processed, the main processor puts a special 3pk that cannot correspond to any real token, on each of the F queues to signal to the busyword processors that the latest batch is complete. This keeps them synchonized.

With the handing off of the 3pk's for the tokens in the current Tweet to the busy-word processor queues, the main processing loop is at its end, and it starts over with the next Tweet. 

The queues for the frequency counting thread and the F busy-word processing threads provide elasticity, so no synchronization with the main loop is necessary other than what is implicit in the thread-safe queues. 

One key design element here is a high degree of encapsulation. Shared data structures are held to the strictest minimum.
 
 
## The 3pK's
A 3pk is just an ordered triple of hashes modulo C for some token or word.  
The hashes are each parameterized with a different value, which yields three almost always different values for each token. C is a configurable global parameter, but it is usually set to somewhere around 1000 to 1500, which gives a key space in the range of one or two billion. More on this later.

As mentioned above, a global two-way mapping of 3pk's to tokens is maintained. 
If a 3pk corresponds to an existing token, it will always exist in this mapping.

The finite key-space means that there is always a possibility of a collision for any given token. 
However, as we will see later, collisions are neither frequent nor very harmful.  

The range can be increased to reduce this probablility farther, but there are 
computational costs to a larger range, so the choice of C is a trade-off. 
How to compute the optimal range size is an open question.

## The Busyword Processor Threads
 
Each of the F busyword processors works the same way, as follows. 

The purpose of the frequency partitioning is to ensure that the background frequencies of the tokens received by a busyword processor are appoximately equal. 

The range of frequencies in a given class is higher for the higher frequency classes than for lower frequency classes, so a set of configuration values allow for compensatory adjustments. More on this elsewhere.

The three hashes in a 3pk are used to index into three corresponding arrays of counters of size C. (The array size is the same as the 3pk range.) One counter in each of the three arrays gets bumped for each incoming 3pk.

### The Z Scores
Because the 3pk values are pseudo random, if every word had the same probability (which the frequency classes attempt to approximate) the C counter values would have an approximately Gaussian distribution, i.e., the familiar bell curve.  

However, we are assuming that some of the words assigned to a given frequecy class are anomalously busy (after all, that is the point of the exercise.) Therefore, some of the counters will have exceptionally high counts. For instance, if you have 10,000 Tweets in a batch, with an average of 10 tokens in each Tweet text, that's 100,000 tokens spread over, say, 24 busyword processors. This means each of the F processors would get about 4,166 tokens, and the counter values would have a mean of 4.166 when the signal 3pk is detected. (We use {-1, -1, -1}).

For low-frequency words, it doesn't take a great number of usages to show up against such a background distribution. If a frequency class is for one-or-two-in-a-million words, half a dozen usages in a batch would usually raise the three counters its 3pk lands on far above the expected value.

One way to look at this is that the anomalously busy words impose a non-Gaussian distribution on top of the underlying background distribution. 

Technically, our counters are not Gaussian
	- Firstly because the values are discrete, not continuous
	- Secondly, because while the frequency class is constructed so that all the words have the same frequency, the busywords are by definition non-conforming. 
  - However, Z scores are fairly robust and this is regarded as an acceptable technique for isolating anomalies.  

 The average Z is 0, by definition (Z can be positive or negative), and the higher the Z, the less likely that a large value is just random variation. This is useful for detecting anomalies because
	- A Z score beyond 4.0 would occur less that once per cycle if all words in the frequency class were arriving at their normal background rate. 
  - A Z of 5.0 or greater would almost never occur.

 By choosing our Z cut-off, we can adjust how freakishly un-random a count has to be for us to treat it as "busy." The busy words we are looking for often bump the Z scores for the corresponding counters as high as double digit numbers, a range far beyond what would ever be seen in the background bell curve.

### Where the magic happens:

The sets of indexes of counters with freakishly high Z scores are collected for each of the three counter arrays. Note that we chose Z to keep cardinality of the set quite small relative to the number of counters. 

The Cartesian product of the three index sets is computed. This means that a triple is computed for every combinations of anomalous values in all three sets. 

This gives you a large set of three-part keys, some of which should correspond to actual busy words, while the others (the vast majority) are just spurious junk. 

The lambs are separated from the goats by selecting only the 3pk's that exist in the global mapping. Any token that is not in the mapping is necessarily a goat, because it's a token we've never seen.

The 3pk's that exist correspond to the busy words for the current window. Note that this is a somewhat leaky filter. 
	- Some randomly generated 3pk's will correspond to real tokens just by random chance because with a range of 1000, there are only a billion possible keys. 
	- Collisons happen. Occasionally, more than one token could map to a 3pk.

The leakiness isn't as bad as one might suppose. The probability of a key collision for a given token is the the cardinality of the 3pk map divided by the cardinality of the key space, which is no more one in several thousand. As this is at most low hundreds of thousands divided by low billions. Something in the 1 in 10,000 range. 

While the number of distinct tokens used in a day is in the millions, the number of distinct tokens seen over the briefer token window, is much smaller. For multiple reasons, you don't want to store unnecessary mappings. Therefore, an offline mechanism culls the tokens that have counts that have gone to zero from the token counter, and also puts them on a queue for the main pipeline to use in culling them from the 3pk mappings. This greatly reduces both the required memory and the probability of token collisions. 

## Processing the Batch of Busywords.

Each of the F frequency classes puts the set of busyword it has computed on a queue to an analysis thread. The results are kept in synch by a concurrency "barrier" to keep the F batches of busywords together.

When the analysis thread gets all F batches of busywords, it does the clustering as follows.

- It filters a configured range of the set of recent Tweets for Tweets that contain at least M busywords. M is configurable and might typically be from two to five.

- With proper configuration, this set of Tweets is quite small compared to the total number of Tweets upon which the batch of busywords was computed. This makes sense because most Tweet subjects at any given moment are not new.

- A graph clustering algorithm finds groups of Tweets among the reduced set that have the most joint similarity in the sets of busywords they contain. (Whether clustering is based on busywords or all words in the Tweet is configurable.)

- Clusters often overlap. For instance, one group might be characterized by five busywords, and another by four, but they happen to share two busywords. However, despite that overlap, they the two groups are identifiably different.  

## The Output
The output is clusters of Tweets around a particular subject. 
These represented in JSON and presented with some metadata for the cluster and for the individual Tweets.  

A second level of clustering can be applied to cluster clusters.

# Speed of Computation and Significant Events

Ideally, we want to be able to process at a multiple of the firehose rate. 

Running on a mid-quality c. 2020 Linux four-core laptop, we process about 8,000 Tweets/second, i.e. 1.6x the nominal firehose rate. 
This could be expected increas with more powerful hardware.

For processing historial data, almost any rate could be achieved by processors working in parallel on large time slices, say, 12 machines each working on a month of data could expect to process a year of the firehose in a week or two.
 
### Starting and Stopping in Development
It takes several minutes for the processing to make a cold start because you need to read enough Tweets to populate the token window. 

If you are starting and stopping frequently, the system can optionally read in the existing token window from disk, for an instant start.  Beware that if you are starting at some random point, the saved frequency data will be wrong to an unpredictable degree. This means you need to either start fresh or disregard the results until you have run for long enough to completely update the background statistics.

You can use the utilities mentioned below to find the date/time you want to start at a specific place in the two-week interval.

## Persistence of Subjects

In addition to computing the new subjects, the system gives information about how long those new subjects have existed in terms of the number of batches. This is given in the form of a list of the previous N batches that also match the new subject.

Persistence of subjects is a major consumer of CPU, bigger than the cost as the clustering itself.  It can be turned on and off, but the effectiveness of turning it off is still to be determined. It's not clear that it affects throughput, which may be bottlenecked elsewhere.

# Significant Events in the Data Set

It is useful to know of some of the major events that occur in the two weeks of data available so that one can watch subjects develop.

## News of Whitney Houston's Death

Her death was discovered at 3:30 PM PST February 11, 2012
She was pronounced dead at 3:55 PM PST

So that is 11:30/11:55 GMT aka Zulu i.e. 23:30/23:55 GMT

### Super Bowl
Note the superbowl is also a great place to see real conversations starting up. It occurs on Feb 5 2012.  
There is much Tweet activity during as well as preceding the game.

## Beyonce Has a Baby
The singer Beyonce has a baby named Blue Ivy.

## The Lion King 
There is a showing of Lion King that gets a lot of activity (Just before Whitney)

## Orgulo Herbeldes Performance

## One Direction Italy Tour Hype


# Data Set Start and End Time

We have a little more than two weeks of almost unbroken data. The feed was restarted briefly once or twice over that time span--probably not enough to matter. 

- Starts around 2012-01-28 16:16:46, i.e. quarter after five on New Years Day
- Ends at 2012-02-15 03:22:36, which is 3:22 AM the day after Valentine's day.

The very first hour or so may or may not be fully trustworthy as there may have been some starts and stops. This can be investigated using the file/time utility. It doesn't seem to matter.

# Data Set Time Fields
The timestamps in the original data are Unix time, which is Zulu, aka GMT, aka UTC.

We have a per-tweet created_at field in UTC.
The formatted date-time fields in individual Tweets are rendered in UTC for the batch and as both UTC and local time for the Tweeter, e.g., in the case of California, i.e., PST

# Tests
We have fallen behind in unit tests!  This is a little obsolete now and needs to be upgraded.

cd /Users/petercoates/python-work/cursor-twitter

### See all available commands
make help

### Run tests with detailed output (this is what you were asking about)
cd cursor-twitter
make test-verbose

### Run comprehensive test suite
make test-full

### Just check for race conditions
make test-race

### This will run them all
cd to cursor-twitter
./run_tests

# Notes On Things That Have Been Checked/Fixed/Explored


## Batch Size Confusion
We had an error where Cursor had the clustering routine working on a multiple of the batch size as set in clustering_window_batches (which is for something else.)  

globalTweetQueue.GetRecentTweets(cfg.Analysis.ClusteringWindowBatches * cfg.BatchSize)

This was wrong, and we fixed it, but it's actually not a bad idea. Why should the busywords at the moment only be tested against the batch? Perhaps clusters go back farther than this? Worth investigating.

Note, it will cause an error were the time spanned in a cluster is excessive (which was what we were fixing when we noticed this.)


## Detecting Backup on the Busy Word Processor Queues
The main pipeline puts tokens on each of F queues for the F busyword processors. 

Accordingly, we periodically check the queue lengths and sleep the main thread for a short time if the queue length ever exceeds a certain value. 

An analogous check is made for the analytics queue. This is checked by comparing the global batch count as maintained by the main thread with the number of batches processed by the analytics thread.

We were seeing these slowdowns a lot. Analysis of how long clustering took revealed the problem. Occasionally, clustering takes a long time because it is quadratic in the number of Tweets to cluster. However, the Tweet count seems quite random, and only a small percentage are out of the 10ms range. However, occasionally the seemed to run as high as 400ms.

The simple solution was to simply allow the threads to get 10x as long before they started sleeping the main thread.  This allowed the occasional very long batch to run overtime, a debt that the average short computation times could easily make up for.
Simply multiplying the allowed queue length by ten almost completely stopped the slowdows.

## 3pk's missing.
We were still getting occasions on which the analystics phase would encounter a 3pk that had already gone to a zero count and been removed.  This was solved by having the FCT count the number of tokens it puts on the zero-count queue and then multiply that number by the value of 
window_batches and atomically updating a globally accessible value that is used to know how much it is safe to remove from the obsolete keys queue.

Every cleanup_trigger_batch_size Tweets, the main checks the queue and deletes up to cleanup_max_items items from the queue, but the computed value is the lower limit to the queue length that the cleanup will go to.  This ensures that any key the analysis thread uses will still be in the lookup table.

## How To Tune Meta-Clusters

After running for 18 hours, we see 3170 meta clusters and 23976 individual cluster, which is about seven individual clusters for each meta cluster. 

Meta-clustering is doesn't seem to be as good at primary clustering. 
Some of them seem ok and some of the seem to be unrelated.  
We need a program for systematically seeing config parameters work and what do not.

Ther are a number of parameters that affect meta clustering. See the config files for details. At the very least, the effect of the parameters on meta-cluster quality needs to be explored.

## Processing Rate

We now have two data input modes: RabbitMQ and reading CSV files directly from disk.
Reading directly from disk is an order of magnitude faster than reading from RabbitMQ.

A large set of enhancements have greatly improved throughput for both modes.

The feeder is running on the same box feeding CSV via RabbitMQ which undoubtedly has some effect.

However, profiling reveals that even with the extreme processing rates obtained with directly reading from disk, the program is still I/O bound. The FCT doing the background computing is the biggest CPU user. After that, it's parsing and cleaning up the tokens, followed by the meta-clustering. The busyword computation costs almost nothing and the primary clustering is only one or two percent.  

## Speedups
Optimizations have tremendously increased throughput.

Many small changes increased:
- Removing contention caused by unnecessary concurrency protections
- Tightening up encapsulation. Everything is now independent, connected by queues that provide elasticity.
- Better memory management, especially getting rid of 3pK mappings for zero-count tokens
- Deduplication of clusters, i.e. scrapping nearly identical Tweets. (We use an algorithm based on Levenshtein distance that works on the word level rather than the character level. Edits apply to entire words, not to characters.)
- Greatly reduced logging and use of a framework that controls how much gets written

### Lessons from Profiling
The profiler reveals that the FCT and the initial processing of inbound Tweets are the big CPU/Time consumers when reading from files.

- In the main thread, processCSVFile is 42% of the cycles. Within that, parseCSVToTweet, simpleTokenize are the big ones.

- The FCT thread is about 20% of the total, with processTokens being the big item.

- The BusyWordProcessors use less than 1%.

- The graph clustering for batch and for meta clusters use about 4.3% and 3.6% respectively.

In other words, when running from pre-processed CSV, processing is dominated by the offline FCT thread.

Next biggest is getting the CSV files off the disk, parsing them into Tweet objects, and cleaning up the tokens, which are about 20% of the total.

Much of the 20%  could be moved out of the main pipeline in an industrial scale application. 

The graph calculations are about 8% in total. 

The core identification of busy words are less than 1% of CPU.

### Still More Efficiency

Going from JSON to CSV would competely dominate processing by a large factor for real-time processing.  Even on a laptop, we can do 8 firehoses once it's converted to CSV.
 
For historical processing all of the JSON -> CSV, the language ID, and the token normalization and cleanup could be moved to other processorsm and nade essentially as fast as you want to pay for CPU's to bulk process it.

Moving the token cleanup to the backend ingestion would significantly speed up the main pipeline activities of identification of busywords, subjects, clusters, and meta-clusters.

The main work in the core pipeline would be bottlenecked by reading and unpacking the CSV.

## Confirmation of the Z-score Principle
The data technically violates the assumptions of Z because a Gaussian assumes continuous data, not integer counts. (There is an argument to be made for Poisson based scoring.)

Also, we pre-suppose that only the underlying frequencies are approximately Gaussian. We explicitly assume that the busywords impose an explicitly non-Gaussian distribution on top of this underlying normal distribution of pseudo random values.

Nevertheless, use of Z in this way is fairly common as we are only identifying anomalies and don't really need to know exactly how anomalous they are.

The following are excerpted from the logs. Note that the low-end Z scores are all small negative numbers, as you'd expect.  
The high z scores are up in the double digits. 
The asymmetry indicates that it is spotting anomalies effectively.

"Z-score stats" class_index=20 min_z=-0.209818386948746 max_z=18.370296042703444

"Z-score stats" class_index=22 min_z=-0.26988490658743347 max_z=12.244939390813714

"Z-score stats" class_index=23 min_z=-1.66058739959313 max_z=7.069655295369607

"Z-score stats" class_index=0 min_z=-0.13493075129053422 max_z=19.828340714364074

"Z-score stats" class_index=22 min_z=-0.27382076781547243 max_z=12.423513223627806

"Z-score stats" class_index=11 min_z=-0.3258363554161604 max_z=11.41178451692391

"Z-score stats" class_index=22 min_z=-0.2729404421546748 max_z=12.93385514597475

"Z-score stats" class_index=11 min_z=-0.3311012931553238 max_z=11.297996719324528

"Z-score stats" class_index=17 min_z=-0.2367758067288478 max_z=15.911842496557954

"Z-score stats" class_index=12 min_z=-0.241437616624771 max_z=19.08450355887032


## 3pk Collisions
The majority of tokens are unique on the time scale of a day and many more occur only once or twice. The majority, in fact, don't appear more than once in two weeks. Permanently storing the 3pk values for super-rare tokens incurs two very significant costs:
- It wastes a ton of memory 
- It reduces quality because of high token collision rates

We solve this bloat problem by having the FCT put tokens on a queue when their counts in the FCT's counter map go down to zero. 

## Seemingly Excessive Token Rejection
We were getting huge percentages of rejection of tokens because of too-short token length and/or tokens being on the ingore list. 
More than 30% and 40% respectively. 
The list of rejected tokens is in config/token_filters.txt.
 
Those numbers were not supurious. 

Turning that functionality off gave the expected result and it is consistent with the Ziph distribution. 

It means both are very effective and doing their job. 
You definitely want min length = 3.  

The two things together strip out a huge amount of processing at the very beginning of the pipeline.

## Effect if Computing Cluster Persistence

Going from checking over six batches to checking over 1 batch did not result in a huge speedup. Maybe 100/second faster.

### Checking all words v checking busy words when doing cluster persistence checking
This aspect of the problem has NOT been reviewed

## Is token splitting on periods, commas, and dashes happening?

No, it was not happening.  The token cleanup in Go now does this. Note that we can optionally split or not split on apostrophes. (See config.yaml) The default is to not split so that O'Brian and M'kele M'beme can continue to be tokens.

## Overhaul of ASCII Output and Reduction of Logs

Logging has been overhauled with logging almost all going to the designated log file and almost none to the screen except some startup info going to stderr.

The program output is now all in JSON format sent to stdout. It can be configured to be more complete, for machines, or more readable, for humans. 

The rest of the logging now has the customary DEBUG/INFO/WARNING/ERROR levels in config.yaml

The new policy is to put only critical messages on the screen and to write them to stderr so we can preserver stdout for proper output.
    
## Annoying Nonsense

We now have a file called banned_phrases.txt that contains a number of phrases that occur in a disproportionate number of essentially meaningless Tweets. No doubt the contents of such a file would need to be adjusted from time to time.

- "The awkward moment"
- "That awkward moment"
- "Do you want more Followers" 
- "Start and internet business"

# Miscelaneous Details

## Why a Big Token Window?
Even relatively common words are rare. Most words show up at most a few times a day, and something like half show up less than once a day. This means you need a lot of words (millions) to get even a reasonably accurate estimate of a word's background frequency unless it is in one of the most frequent classes. 

On the other hand, you  need the set of words to be bounded because if it is too big, the surging tweets won't age out in a reasonable amount of time.

Five million is a reasonable window size because it is big enough to to get an idea of the frequencies of the top few 10's of thousands of words, yet fine enough to keep up with the time of day and age-out surges in frequency promptly. The actual size limit is controled from the config.yaml file 

Millions of words take significant space in memory. A typical hash table uses a minimum of about a hundred bytes regardless of the size of the word, so five million words would be half a gigabyte of main memory.

Accordingly, the tokens underlying the token counters are written to disk in batches. 
After the number of token batch files reaches a defined limit, each time a new batch is written out, the oldest batch is read in (and deleted from disk), and the contents of the old batch are then used to decrement the global token counts.

This keeps the token counts relatively up to date with the flow of Tweets.  

Note that frequency calculations would typically happen multiple times in the span of time represented by an entire token window. Again, it's a matter of configuration, but three times would be reasonable.

Say you have a five million word window, that would be 16.6 minutes of the full firehose. You might recompute the frequency filters five times per window span, which would be every  
3.33 minutes of nominal Tweet time.

There is also a parameter that controls how many chunks the window is broken into on disk. The larger the number, the more up to date the program is with the changing frequencies.

###  Clock Driven Changes in Background Frequency 

People don't Tweet the same things at breakfast on Monday that they do at 1:00 AM on Saturday.  Also, the world turns, and while New Yorkers are getting up in the morning, people in Bejing are out for the evening.

### New and Evolving Subjects Change the Background Frequencies
The surges in Tweet subjects (i.e., we are looking for) continually change the current prevailing word frequencies. Therefore, it is necessary to recompute the background frequencies fairly frequently to cause no longer new subjects that people keep Tweeting about to be forgotten. 

One aspect of this that may not be obvious is that a word in frequency class f that has surged in usage will probably be in a higher frequency class the next time relative frequencies are calculated. This is one way the words signifying major Tweet subjects can stay alive. A word can surge enough to get bumped into a different class, and if it's usage has plateaued, it may not be a busy word the next time frequencies are computed. But if it continues to surge, it may be an anomaly in its next class as well.

## The Small Window of Tweets

The big token window is only of concern to the off-line frequency calculating thread.  
The main thread keeps short window of Tweets in memory for use in the clustering algorithm, which is part of the main processing pipe line (although it runs in its own thread.)

This is a conventional queue that keeps a configured number of Tweets, dropping the old ones as new ones are added. This is configured to hold a few processing batches of Tweets. 
As a batch is typically in the range of a few thousands (10k to 30k), it would be a small multiple of that size. The multiple is a matter of configuration. 

Clustering happens with respect to the latest batch. However, the persistence of clusters is tracked across previous batches. You can see in this way whether a subject just popped up, or has it been around a while.

# Running the Profiler

The profiler is very useful for finding where the CPU goes and whether you are suffering from contention due to concurrency.

The graphical display of profiler output won't work on the iMac because it's too old to load the graphviz libraries and other necessary stuff.

The graphical display works fine on the System 76 Linux box.

 You can run the program for a while with the following command line to get profile data. 

 ./main -config ./config/config.yaml -print-tweets=false -profile

 Let it run for a while.  

 This gets crude output on either platform.

  go tool pprof -text cpu.prof

  The following gets graphical output on the Linux box.

  go tool pprof -http=:8080 cpu.prof


  You can get a quick summary with
  go tool pprof -top cpu.prof
 
   
# The analyze_tokens Program Output

The analyze_tokens program reads CSV files and accumulates the number of distinct tokens that it has encountered.

This is the result of running on about 1200 CSV files which is at least a couple of days of data.  Notice it rises quickly at first, and then settles down to a pretty stead rate of about one new token for every 3.15 Tweets, or roughly every 31.5 tokens.  That rate looks like it holds pretty steady, as this is two days of the Decahose.

It is unknown if the full Firehose would behave differently. It could be that most of the diversity at any given moment is contained in a fraction of the Tweets.

ASCII Graph: Distinct Tokens vs. Tweets Read
44298119 |                                                           *
         |                                                      ***** 
         |                                                  *****     
         |                                               ****         
         |                                           *****            
         |                                       *****                
         |                                   *****                    
         |                                ****                        
         |                            *****                           
         |                         ****                               
22162570 |                      ****                                  
         |                   ****                                     
         |                ****                                        
         |             ****                                           
         |          ****                                              
         |       ****                                                 
         |     ***                                                    
         |   ***                                                      
         | ***                                                        
   27021 |**                                                          
         +------------------------------------------------------------
         10000                     69810038                 139610077
Y: 27021 to 44298119

 

# Volume Issues

## The Data Set
The full dataset is the original JSON. It has 5399 five minute files, comprising 598,725,870 Tweets. We preprocess this to 105 Gigabytes in uncompressed CSV format (Which is significantly smaller than the compressed JSON!)

The dataset has 599 million Tweets, which contain approximately six billion tokens.  
 
The dataset is two weeks of the decahose, at about 500/sec. You can process it at any speed, but the time stamps in the data will correspond to the real clock time when they were recorded. 

The volume of real firehose is typically about 5000/sec but has been known to surge up to 20k/sec. 

The decahose nominal Tweet time is about
- 500/sec 
- 30k Tweets/min 
- 450,000 Tweets/15min  
- 1.8m Tweets/hour 
- There are an average of 10 words/Tweet = 4,400,000 words/15 min or 18m words/hour

## Our Processing Speed

Running over RabbitMQ, the iMac can do about 
1100 Tweets/second and the System 76 running Linux can do about 8,000 Tweets/second.
So, about 2.5 decahoses and 16 decahoses respectively.

With direct reading of the Tweets from disk, the System76 laptop and the iMac do about 55k and 45k Tweets/second respecively. 
They both appear to be bottlenecked on reading the Tweets in, not on processing.
This is very fast--about five or six firehoses, i.e. fifty or sixty decahoses.

So the system as a whole is overwhelmingly bottlnecked on parsing the JSON.  This is not a problem for historical data as the task is easy to share over as many CPU's as you want. 

It's a little trickier for real time because without significant complexity, you r are limted to the speed of the processor receiving the feed and parsing the JSON. 

  
## Language 

If you care about a particular language, e.g., "en", you need a pre-processed version of the data because the lang: field in the original data is essentially worthless, being evidently based only on the environmetal settings of the person issuing Tweet.
There is a multi-threaded utility to identify the actual language and set the field accordingly, (see USER_MANUAL.md) but it should be noted that the NLP processing to 
if by far the most expensive part of the processing, outweighing everything else put together buy a multiple.

We have a set of the CSV files that have been post-processed to have this more reliable lang: field. 

# Candidate Functions That May Be Duplicated. 91 possibles! Insane.

These functions are all possible duplicates. Apparently cursor was not deleting
them as they were moved during refactoring.  (Despite having been cautioned to do
so repeatedly.)

### Main Pipeline Functions (src/main.go)
addBatchToWindow
getBatchWindow
TweetQueue.Len
setupCPUProfiling
createBatchFromClusters
getContinuationInfo
clustersAreRelated
clustersAreRelatedByBusyWords
clustersAreRelatedByFullText
deduplicateAndSort
processBatchPersistence
removePunctuation
shouldFilterToken
manageSlidingWindow
setupBloomFilterParams
jaccard ⚠️ DUPLICATE (also in output.go)
findMostTypicalTweets
removeNearDuplicates
levenshteinDistance
wordDistance
normalizedWordDistance

### RabbitMQ Functions (src/rabbitmq.go)
NewRabbitMQ
RabbitMQ.Close
RabbitMQ.GetQueueInfo

### Config Functions (src/config/config.go)
LoadAndValidateConfig
resolvePathRelativeToConfig
resolvePathsInConfig
LoadAndValidateConfigWithPathResolution
LoadConfigWithoutValidation

### Filter Functions (src/filter/word_filter.go)
WordFilter.AddWord
WordFilter.RemoveWord


### Logging Functions (src/logging/stats.go)
GetCurrentWorkingDir

### Output Functions (src/output/output.go)
ShouldFilterRepetitiveCluster
OutputCluster
OutputStats
OutputError
OutputInfo
OutputRaw
ConvertToHumanReadable
ConvertIndividualClusterToHumanReadable
removeNearDuplicates ⚠️ DUPLICATE (also in main.go)
findMostTypicalTweets ⚠️ DUPLICATE (also in main.go)
abs
max
levenshteinDistance ⚠️ DUPLICATE (also in main.go)
wordDistance ⚠️ DUPLICATE (also in main.go)
normalizedWordDistance ⚠️ DUPLICATE (also in main.go)
min
jaccard ⚠️ DUPLICATE (also in main.go)
clusterSimilarity
PerformMetaClustering
PerformUnionMetaClustering
ConvertToIndividualCluster
createMetaCluster
calculateMedoidSimilarity
calculateBusyWordSimilarity
generateThemeFromMedoid
generateIDFromTheme
ConvertBatchToHumanReadable
CreateBatchFromClusters
GetContinuationInfo
ClustersAreRelated
ClustersAreRelatedByBusyWords
ClustersAreRelatedByFullText
deduplicateAndSort
ProcessBatchPersistence
getBatchWindow
convertToHumanReadable

### Pipeline Functions (src/pipeline/)
NewFrequencyClassProcessor
FrequencyClassProcessor.writeToCSV
FrequencyClassProcessor.printBatchSummary
FrequencyClassProcessor.convert3PKsToWords
FrequencyClassProcessor.getWordFrom3PK
BusyWordProcessor.performZComputation
CleanupQueue.Enqueue
CleanupQueue.Dequeue
CleanupQueue.Size
CleanupQueue.IsEmpty
NewSetFilter
GetTokenFrequencyClass
AddWordTo3PKMapping
AddToCleanupQueue
GetCleanupQueueSize
GetAndResetTokensAddedToCleanupQueue
GetDynamicCleanupLeaveAtLeast

### Regex Functions (src/regex/)
LoadBannedPhrasesFromFile
LoadBannedPhrasesFromDirectory
CompileBannedPhrasePattern

### Utils Functions (src/utils/math.go)
Abs
Max
Min
Note: I found 91 functions, not 80. The functions marked with ⚠️ DUPLICATE are confirmed duplicates that exist in both files.