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

# Input and Output

The input available for development purposes is two weeks of the decahose (about 500 Tweets/second.) It consists of JSON-formatted Tweets in files that have the file order encoded in the filenames as Unix start and end times. The files are about five minutes of Tweets at about 500 Tweets/second. The original files are unpacked into uncompressed CSV files.  

Calendar and clock time are encoded into the Tweet data at the decahose rate, but the Tweets can be consumed at any desired rate--the application consumes them as fast as they are supplied. 

In production the input could be either files (for historical processing) or the actual live Tweet stream (for real time processing).  

Using files is not a "cheat" because any system would typicaly receive Tweets and feed them somehow. The actual development feed is from a process running on the same meachine and sending the Tweets via RabbitMQ. In a real deployment, a separate machine would probably handle receiving the data, storing it and parsing it for consumption by the application.

The non-graphical output is a series of clustered sets of Tweets that are about new subjects. There is not yet a graphical front end.

# The Heuristic
This section explains the core heuristic for finding the busywords at a minimal level to make TTD items comprehensible.

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

## Detecting Backup on the Busy Word Processor Queues
The main pipeline puts tokens on each of F queues for the F busyword processors.  These should be fast, but if they ever began to run more slowly than the main thread, the queues would grow without bound and eventually result in an OOM error.

Accordingly, we periodically check the queue lengths and log warnings if their length ever exceed the batch side multiplied by the  bw_queue_max. A quite small value of 0.1 for bw_queue_max was used without log messages showing up. A higher value would probably be appropriate for general use.

An analogous check is made for the analytics queue. This is checked by comparing the global batch count as maintained by the main thread with the number of batches processed by the analytics thread.

Neither type of slowdown has actually been seen, but both situations cause log lines to be printed both to the log files and to STDERR.

## How To Tune Meta-Clusters

After running for 18 hours, we see 3170 meta clusters and 23976 individual cluster, which is about seven individual clusters for each meta cluster. 

Meta-clustering is doesn't seem to be as good at primary clustering. 
Some of them seem ok and some of the seem to be unrelated.  
We need a program for systematically seeing config parameters work and what do not.

Ther are a number of parameters that affect meta clustering. See the config files for details. At the very least, the effect of the parameters on meta-cluster quality needs to be explored.

## Processing Rate
A large set of enhancements have greatly improved throughput.

The feeder is running on the same box feeding CSV via RabbitMQ.

It is not clear what limits processing. It may be RabbitMQ, which shares the compute platform with the main app. It is also possible that the limit is imposed by the ability of the main thread to take items off of RabbitMQ. The sender program could also be a bottleneck, as could the main-thread processing that takes place after a Tweet is removed from RabbitMQ. 

Throughput might be higher for historical data if RabbitMQ were eliminated entirely:
- The feeder were writing to STDOUT and the main reading fro STDIN
- The main were reading the files directly

It is also not clear whether the feeder running on another machine with Rabbit writing over the LAN would net slower of faster.

## Speedups
Optimizations have tremendously increased throughput.

Many small changes increased:
- Removing contention caused by unnecessary concurrency protections
- Tightening up encapsulation. Everything is now independent, connected by queues that provide elasticity.
- Better memory management, especially getting rid of 3pK mappings for zero-count tokens
- Deduplication of clusters, i.e. scrapping nearly identical Tweets. (We use an algorithm based on Levenshtein distance that works on the word level rather than the character level. Edits apply to entire words, not to characters.)
- Greatly reduced logging and use of a framework that controls how much gets written

## Confirmation of the Z-score Principle
The data technically violates the assumptions of Z because a Gaussian assumes continuous data, not integer counts. (There is an argument to be made for Poission based scoring.)

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

# TTD and Direction

## Clustering Improvement By Weighting Frequency Classes
Would clustering be improved by weighting the frequency classes?

- The rarer the word class, the more it is worth?
- Or possibly the opposite.

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
We sometimes get notification in the logs of burst of 3pk's not mapping to tokens. 
This should be almost impossible (it says so right in the warning message.) 'Sup with that?

Suspect this is a phenomenon of startup from token counts on disks.
This needs to be confirmed.
 
## New Input Mode(s)
The main is now fed through RabbitMQ. 
This is a realistic model of consuming the live firehose.

It would be interesting to have a "history" mode specifically for mass consumption as fast as possible.
The following are possibilities

- Create a mode that reads from STDIN.
  - Cat the lines in the CSV to standard out. D
  - Does it need to be speed limited or will it somehow self-throttle?
- Add a mode for actively the CSV 
  - Throttling may not be an issue 
  - If it is, it's easy enought to start inserting on-second sleeps in the main pipeline when either the busyword processors or the analytics thread get overwhelmed.

 
## Ongoing  Items

- Comments Key areas need to be commented to keep out don't touch, etc.

- Testing has been neglected. Make tests around everything.

## The Token Filter File

This is currently populated by whimsy. Basically, I look at the logged busywords per frequency class output and put the obvious crap in the toke_filters.txt file

This filters out a ton of words--something like 60% of usages right at the entry to the pipeline.

A more mathematically sound way to do this might include entropy.

But you don't necessarily want to blindly eliminate the super common words. 
Some very common words matter. If "Trump" is normally in F-class 3, and suddenly jumps to F-class 1, it might actually mean something.

See my Evernote notes on entropy for this.  It's a long file, but most of it doesn't apply. Basically, all you need to do is compute the Shannon entropy on the word counts and exclude those with insufficient entropy.

The right approach seems to be the one clipped at the bottome. Slotted entropy based on the token files used to age tokens out of the counts.

The FCT thread could compute entropy every time it makes the filters. We'd have to time this. It might take a minute!  But does it matter?


## Major: Improving Busy Word Detection Quality by Computing Redundantly

This would be a significant effort.  Interesting idea, but it's not 100% clear that it's worth doing. We need some investigation.

The most important thing would be to run the profiler and see how much time is actually spent on busyword processing. It might not be much.

Consider that dual sets of pipelines, or even three sets, each with different hash functions, could do a much more accurate job of filtering out the true busy words for a given set of parameters.  
 
It is not clear how big an impact this would have on performance. All the BW processors are doing is counting and periodically computing Z on a thousand values in each of the three counter arrays. There is a tiny bit more work in the analysis to take only the words that appear in the required number of sets. It doesn't really affect the analysis phase that follows, and it's just a little more work for the main to put the tokens on more queues than before.

Risk. If multiplying the work in the busyword processors made them in aggregate slower than the combined main pipeline and the clustering, it would cause the queues to grow without bound and crash the program. You'd need to detect the problem and throttle the reads if this is a problem. Actually, this should probably be done anyway! Who knows if some combination of config parameters could cause this to happen.

## Possible Blow-up From Overload
The busyword processor queues are now monitored for length an throw out warnings
if they grow beyond bw_queue_max * batch in length.

It is not clear whether backup from the analytics processor would be caught here. The bw processors put their data on queues with a barrier to ensure that the analytics queues works with all F busyword sets. However, it must be verified that the bw processors can't continue even when the analytics processor can't keep up!  

Not clear how to ensure programmatically that the analytics thread is keeping up.

It could count batches and compare them to the number of batches by the front end. It would normally be at least one behind, but if the global batch number exceeded the analytics batch number by some fixed amount, e.g. 3, it would warn and print out both batch counts..
   

## Major: Clustering Across Batches

Needs to be investigated for quality.

Jacquard similarity for the clustering seems to be applied to all the tokens in the Tweets. 
  - Is that true, and if so, is it correct? This may have been done.
  - Maybe it should be only on the busywords. 
  - Jacquard similarity might not be important. Maybe just the raw occurrences?

## Major: A Graphical Front End
This is a big one!  Not sure how to go about it with Cursor. I wrote the graphics by hand in Java last time!

A busy-word fade-out like the bubbles would be great.
- The size proportional to the number of Tweets the busy word is in. Or perhaps logarithmically proportional.
- When the BW was last seen, fading off the left
- Vertical axis is the frequency class

## Consider Stripping K-Means Out Entirely
It doesn't do any harm, but we're never going to use it and it could be confusing.
   
## Document What is Known About Tuning Performance and Accuracy

# Miscelaneous Details

## Why a Big Token Window?

###  Clock Driven Changes in Background Frequency 

People don't Tweet the same things at breakfast on Monday that they do at 1:00 AM on Saturday.  Also, the world turns, and while New Yorkers are getting up in the morning, people in Bejing are out for the evening.

### New and Evolving Subjects Change the Background Frequencies
Also, surges in usage continually change the current frequencies.

Even relatively common words are rare. This means you 
need a lot of words (millions) to get even a reasonably accurate estimate 
of a word's background frequency. 

Five million is a reasonable window size because it is big enough to catch the top few 10's of thousands of words, and fine enough to keep up with the time of day while aging surges in frequency out with reasonable latency.

You have to age tokens out if you want a stable size window, which gets to be a lot to keep in memory. 

Accordingly, the tokens underlying the token counters are written to disk in batches. 
After the number of token batch files reaches a defined limit, each time a new batch is written out, the oldest batch is read in (and deleted from disk). 
The contents of the old batch are then used to decrement the global token counts.

This keeps the token counts relatively up to date with the flow of Tweets.  

Note that frequency calculations would typically happen multiple times in the span of time represented by an entire token window.  
These things are controlled by separate parameters. 
Say you have a five million word window, i.e., 16 or 17 minutes, you might recompute the frequency filters every couple or three minutes.

## The Small Window of Tweets

The big token window is only of concern to the off-line frequency calculating thread.  
The main thread keeps short window of Tweets in memory for use in the clustering algorithm, which is in the main processing line.

This is a conventional queue that keeps a configured number of Tweets, dropping the old ones as new ones are added.  
This is configured to hold a few processing batches of Tweets. 
As a batch is typically in the range of a few thousands (3k to 20k), it would be a small multiple of that size.  

Clustering happens with respect to the latest batch.  
However, the persistence of clusters is tracked across previous batches. 
You can see in this way whether a subject just popped up, or has it been around a while.

# Running the Profiler

The profiler is very useful for finding where the CPU goes and whether you are suffering from contention due to concurrency. You can run the program for a while with the following command line to get profile data.  Run it for a good while so that the output is not dominated by startup processing, which is more or less irrelevant to steady state performance.
 
 ./main -config ./config/config.yaml -print-tweets=false -profile

 Let it run for a while. 
 Depending on what you are investigating, it may take some time, as the processing pipeline has to wait a few minutes for frequency filters to become available.

  go tool pprof -text cpu.prof

  or 

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

## Test Data Set
You need a pre-processed version of the data if you care about a particular language, e.g., "en".

- The same data set post-processed for language identification is in ../twits/test_language_detect_out

- The full dataset of 5399 files has 598,725,870 Tweets and is about 105 Gigabytes in uncompressed CSV format (Which is significantly smaller than the compressed JSON!)

- The analysis program reports that there are 
  - 44,298,119 distinct tokens in 137 million Tweets, 
  - 135,315,247 distinct tokens in the full data set 

The analyis program is pretty fast, at 52,683 Tweets/sec    

- 599 million Tweets equals approximately six billion tokens.  
 
- Volume of decahose = 500/sec

- Volume of firehose = 5000/sec

- 500/sec = 30k Tweets/min = 450,000 Tweets/15min  = 1.8m Tweets/hour in nominal Tweet time

- There are an average of 10 words/Tweet = 4,400,000 words/15 min or 18m words/hour

## Our Processing Speed

The System76 laptop does about 8k Tweets/second. The iMac does about 3k.

- The Tweets get ACKed, so RabbitMQ manages to never let the queue get very long--it seems to never get up to 50.
  
- Note that for historical data it will scale in direct proportion to the number of machines so you can do bulk processing essentially as fast as you are willing to pay for.

- We have about 350 hours of decahose, which the 76 can process in about a day. 

# Profiling on an Extended Run
The following are the results from running for several hours with profiling. Any expenditures for startup should be negligible here.

```
(base) Peters-iMac:cursor-twitter petercoates$ go tool pprof -top cpu.prof
File: main
Type: cpu
Time: 2025-08-12 11:19:17 EDT
Duration: 5.11hrs, Total samples = 9325.52s (50.68%)
Showing nodes accounting for 8105.68s, 86.92% of 9325.52s total
Dropped 1066 nodes (cum <= 46.63s)
      flat  flat%   sum%        cum   cum%
  2429.33s 26.05% 26.05%   2441.58s 26.18%  syscall.syscall
  1944.39s 20.85% 46.90%   1944.84s 20.86%  runtime.kevent
   992.84s 10.65% 57.55%    992.94s 10.65%  runtime.pthread_cond_wait
   613.05s  6.57% 64.12%    613.19s  6.58%  runtime.pthread_cond_signal
   362.42s  3.89% 68.01%    362.52s  3.89%  runtime.usleep
   348.22s  3.73% 71.74%    348.22s  3.73%  aeshashbody
   144.71s  1.55% 73.29%    161.57s  1.73%  runtime.findObject
   105.53s  1.13% 74.42%    366.27s  3.93%  runtime.scanobject
   104.08s  1.12% 75.54%    594.21s  6.37%  main.jaccard
    87.74s  0.94% 76.48%    621.06s  6.66%  runtime.mapassign_faststr
    86.21s  0.92% 77.41%   2005.05s 21.50%  runtime.netpoll
    77.49s  0.83% 78.24%     77.49s  0.83%  internal/runtime/maps.ctrlGroup.matchH2 (inline)
    74.89s   0.8% 79.04%     99.78s  1.07%  internal/runtime/maps.(*Iter).Next
    69.59s  0.75% 79.79%    663.95s  7.12%  main.findMostTypicalTweets
    63.07s  0.68% 80.46%     63.07s  0.68%  runtime.memmove
    59.07s  0.63% 81.10%    123.61s  1.33%  internal/runtime/maps.(*Map).getWithoutKeySmallFastStr
    55.75s   0.6% 81.69%    484.68s  5.20%  cursor-twitter/src/pipeline.(*OptimizedTweetClusterer).calculateJaccardSimilarity (inline)
    52.94s  0.57% 82.26%     52.94s  0.57%  runtime.memclrNoHeapPointers
    41.28s  0.44% 82.70%     86.71s  0.93%  internal/runtime/maps.(*Map).putSlotSmallFastStr
    32.55s  0.35% 83.05%       163s  1.75%  runtime.mapaccess2_faststr
    23.58s  0.25% 83.31%     60.79s  0.65%  runtime.selectgo
    23.02s  0.25% 83.55%     65.18s   0.7%  regexp.(*Regexp).tryBacktrack
    19.04s   0.2% 83.76%    101.51s  1.09%  runtime.mapaccess1_faststr
    18.71s   0.2% 83.96%   3584.63s 38.44%  runtime.park_m
    16.88s  0.18% 84.14%    137.71s  1.48%  runtime.mallocgcSmallScanNoHeader
    16.78s  0.18% 84.32%    255.80s  2.74%  runtime.mallocgc
    12.48s  0.13% 84.45%     68.86s  0.74%  runtime.greyobject
    12.27s  0.13% 84.58%    383.48s  4.11%  runtime.stealWork
    11.79s  0.13% 84.71%     46.66s   0.5%  cursor-twitter/src/pipeline.(*BusyWordProcessor).run
    11.10s  0.12% 84.83%     98.04s  1.05%  regexp.(*Regexp).backtrack
    10.63s  0.11% 84.94%    149.27s  1.60%  main.simpleTokenize
    10.44s  0.11% 85.06%     86.82s  0.93%  runtime.mapIterNext
     9.49s   0.1% 85.16%   3187.51s 34.18%  runtime.findRunnable
     9.05s 0.097% 85.25%    494.49s  5.30%  cursor-twitter/src/pipeline.(*OptimizedTweetClusterer).buildSparseGraph
     8.70s 0.093% 85.35%    133.62s  1.43%  internal/runtime/maps.(*Map).growToTable
     8.19s 0.088% 85.44%     66.17s  0.71%  runtime.mallocgcSmallNoscan
     8.09s 0.087% 85.52%     76.06s  0.82%  regexp.(*Regexp).allMatches
     6.99s 0.075% 85.60%     65.95s  0.71%  runtime.growslice
     6.68s 0.072% 85.67%     76.86s  0.82%  runtime.newobject
     6.28s 0.067% 85.74%    293.02s  3.14%  bufio.(*Reader).Read
     6.10s 0.065% 85.80%     85.89s  0.92%  runtime.makeslice
     5.36s 0.057% 85.86%   3537.82s 37.94%  runtime.schedule
     5.35s 0.057% 85.92%     46.99s   0.5%  encoding/csv.(*Reader).readRecord
     5.33s 0.057% 85.97%    298.35s  3.20%  io.ReadAtLeast
     5.24s 0.056% 86.03%    703.04s  7.54%  runtime.gcDrain
     4.44s 0.048% 86.08%   1227.50s 13.16%  main.runClusteringForBatch
     4.40s 0.047% 86.12%     62.63s  0.67%  runtime.(*timers).check
     3.57s 0.038% 86.16%     62.12s  0.67%  runtime.mapIterStart
     3.47s 0.037% 86.20%    339.47s  3.64%  runtime.runqgrab
     3.34s 0.036% 86.24%   2204.95s 23.64%  main.main
     3.19s 0.034% 86.27%     47.67s  0.51%  runtime.makemap
     3.17s 0.034% 86.30%   2160.94s 23.17%  internal/poll.(*FD).Write
     2.78s  0.03% 86.33%   1093.13s 11.72%  runtime.systemstack
     2.58s 0.028% 86.36%    361.55s  3.88%  main.parseCSVToTweet
     2.30s 0.025% 86.39%    304.10s  3.26%  runtime.ready
     2.12s 0.023% 86.41%     58.82s  0.63%  runtime.(*mcache).refill
     1.97s 0.021% 86.43%      3587s 38.46%  runtime.mcall
     1.95s 0.021% 86.45%     49.78s  0.53%  github.com/streadway/amqp.(*reader).parseMethodFrame
     1.87s  0.02% 86.47%   1717.11s 18.41%  github.com/streadway/amqp.(*Channel).sendOpen
     1.82s  0.02% 86.49%     49.60s  0.53%  internal/runtime/maps.(*table).reset
     1.82s  0.02% 86.51%     53.09s  0.57%  runtime.(*mheap).allocSpan
     1.81s 0.019% 86.53%        88s  0.94%  regexp.(*Regexp).Split
     1.69s 0.018% 86.55%     46.89s   0.5%  github.com/streadway/amqp.(*Connection).dispatchN
     1.68s 0.018% 86.57%    373.32s  4.00%  github.com/streadway/amqp.(*reader).ReadFrame
     1.65s 0.018% 86.58%   1721.87s 18.46%  github.com/streadway/amqp.(*Channel).Ack
     1.63s 0.017% 86.60%     64.36s  0.69%  runtime.(*mcache).nextFree
     1.58s 0.017% 86.62%     71.10s  0.76%  internal/runtime/maps.newTable
     1.53s 0.016% 86.63%   1654.69s 17.74%  bufio.(*Writer).Flush
     1.42s 0.015% 86.65%     99.46s  1.07%  regexp.(*Regexp).doExecute
     1.38s 0.015% 86.66%   1007.83s 10.81%  runtime.semasleep
     1.37s 0.015% 86.68%   1690.95s 18.13%  github.com/streadway/amqp.(*writer).WriteFrame
     1.37s 0.015% 86.69%    340.84s  3.65%  runtime.runqsteal
     1.33s 0.014% 86.71%   2153.26s 23.09%  syscall.write
     1.31s 0.014% 86.72%    635.61s  6.82%  runtime.wakep
     1.21s 0.013% 86.73%     79.37s  0.85%  cursor-twitter/src/pipeline.GenerateThreePartKey
     1.12s 0.012% 86.75%    640.41s  6.87%  runtime.semawakeup
     1.09s 0.012% 86.76%     48.46s  0.52%  github.com/streadway/amqp.(*Connection).demux
     1.03s 0.011% 86.77%    437.68s  4.69%  github.com/streadway/amqp.(*Connection).reader
     1.02s 0.011% 86.78%   1651.37s 17.71%  net.(*netFD).Write
     0.97s  0.01% 86.79%    614.14s  6.59%  cursor-twitter/src/pipeline.(*FrequencyComputationThread).run
     0.89s 0.0095% 86.80%   1710.27s 18.34%  github.com/streadway/amqp.(*Connection).send
     0.74s 0.0079% 86.81%   1007.07s 10.80%  runtime.notesleep
     0.66s 0.0071% 86.82%    577.45s  6.19%  cursor-twitter/src/pipeline.(*FrequencyComputationThread).processTokens
     0.66s 0.0071% 86.82%    639.42s  6.86%  runtime.notewakeup
     0.64s 0.0069% 86.83%    625.26s  6.70%  runtime.startm
     0.62s 0.0066% 86.84%   1011.56s 10.85%  runtime.stopm
     0.60s 0.0064% 86.84%    289.94s  3.11%  internal/poll.(*FD).Read
     0.56s 0.006% 86.85%   1651.98s 17.71%  net.(*conn).Write
     0.54s 0.0058% 86.85%   1722.41s 18.47%  github.com/streadway/amqp.Delivery.Ack (inline)
     0.52s 0.0056% 86.86%   1717.63s 18.42%  github.com/streadway/amqp.(*Channel).send
     0.52s 0.0056% 86.87%     76.58s  0.82%  regexp.(*Regexp).FindAllStringIndex
     0.45s 0.0048% 86.87%    461.08s  4.94%  runtime.gcBgMarkWorker
     0.44s 0.0047% 86.87%    333.49s  3.58%  runtime.resetspinning
     0.40s 0.0043% 86.88%     47.39s  0.51%  encoding/csv.(*Reader).Read
     0.35s 0.0038% 86.88%   1216.60s 13.05%  main.runGraphClustering
     0.33s 0.0035% 86.89%    544.98s  5.84%  cursor-twitter/src/pipeline.(*FrequencyComputationThread).writeTokenFile
     0.33s 0.0035% 86.89%   2153.59s 23.09%  syscall.Write (inline)
     0.32s 0.0034% 86.89%    282.32s  3.03%  net.(*netFD).Read
     0.32s 0.0034% 86.90%    287.39s  3.08%  syscall.read
     0.30s 0.0032% 86.90%   1007.37s 10.80%  runtime.mPark (inline)
     0.27s 0.0029% 86.90%    298.62s  3.20%  io.ReadFull (inline)
     0.27s 0.0029% 86.91%    282.60s  3.03%  net.(*conn).Read
     0.26s 0.0028% 86.91%     50.66s  0.54%  runtime.(*mheap).alloc.func1
     0.20s 0.0021% 86.91%    283.15s  3.04%  runtime.send.goready.func1
     0.16s 0.0017% 86.91%   2441.39s 26.18%  internal/poll.ignoringEINTRIO (inline)
     0.15s 0.0016% 86.91%    507.09s  5.44%  fmt.Fprintln
     0.15s 0.0016% 86.92%    511.05s  5.48%  os.(*File).Write
     0.12s 0.0013% 86.92%    287.51s  3.08%  syscall.Read (inline)
     0.09s 0.00097% 86.92%    307.61s  3.30%  main.convertIndividualClusterToHumanReadable
     0.04s 0.00043% 86.92%    307.17s  3.29%  main.removeNearDuplicates
     0.03s 0.00032% 86.92%    510.88s  5.48%  os.(*File).write (inline)
     0.03s 0.00032% 86.92%    315.88s  3.39%  runtime.pollWork
     0.02s 0.00021% 86.92%    352.38s  3.78%  main.convertBatchToHumanReadable
         0     0% 86.92%     46.66s   0.5%  cursor-twitter/src/pipeline.(*FrequencyClassProcessor).Start.func1
         0     0% 86.92%    501.27s  5.38%  cursor-twitter/src/pipeline.(*OptimizedTweetClusterer).ClusterTweets
         0     0% 86.92%    352.95s  3.78%  main.OutputClusterWithConfig
         0     0% 86.92%    352.38s  3.78%  main.convertToHumanReadable
         0     0% 86.92%   1227.61s 13.16%  main.startAnalysisThread.func1
         0     0% 86.92%    703.80s  7.55%  runtime.gcBgMarkWorker.func2
         0     0% 86.92%    246.25s  2.64%  runtime.gcDrainMarkWorkerDedicated (inline)
         0     0% 86.92%    456.79s  4.90%  runtime.gcDrainMarkWorkerIdle (inline)
         0     0% 86.92%   2204.95s 23.64%  runtime.main
```