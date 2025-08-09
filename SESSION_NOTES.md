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
A 3pk is just an ordered triple of hashes modulo C for some token or word.  The hashes are each parameterized with a different value.  This value of C is a configurable global parameter, but it is ordinarily somewhere around the cube root of one or two billion, which is the size of the key space. More on this later.  

As mentioned above, a global mapping of 3pk's to tokens, and tokens to 3pk's is maintained. If a 3pk corresponds to an existing token, it will always exist in this mapping.

A 3pk hash range of 1000 gives the 3pk's a logical space of a billion different pseudo-random triples. 
There are only at most few million distinct words in the global computation at any one time, so there is always a possibility of a collision for any given token. 
However, as we will see later, collisions are neither frequent nor very harmful.  

The range can be increased to reduce this probablility farther, but there are computational costs to a larger range, so it's a trade-off. 
How to compute the optimal range size is an open question.

## The Busyword Processor Threads
 
Each of the F busyword processors works the same way. 

The purpose of the frequency partitioning is to ensure that the background frequency of the tokens received by a busyword processor are appoximately equal.
The equality is less approximate for higher frequency classes, so a set of F configurable values allows compensatory adjustments. More on this elsewhere.

The hashes from each 3pk are used to index into three corresponding arrays of counters of size C. (The array size is the same as the 3pk range.) One counter in each of the three arrays gets bumped for each incoming 3pk.

### The Z Scores
Because the 3pk values are pseudo random, if every word had the same probability (which the frequency classes attempt to ensure) the counts would have a Gaussian distribution, i.e., the familiar bell curve.  

However, we are assuming that some of the words assigned to a given frequecy class are anomalously busy (after all, that is the point of the exercise.) 
Therefore, some of the counters will have exceptionally high counts.  
For instance, if you have 10,000 Tweets in a batch, with an average of 10 tokens in each Tweet text, that's 100,000 tokens spread over, say, 24 busyword processors. 
This means each of the F processors would get about 4,166 tokens, and the counter values would have a mean of 4.166 when the signal 3pk is detected. (We use {-1, -1, -1}) at which point the processor suspends reading the queue and processes the batch.

For low-frequency words, it doesn't take a great number of usages to show up against such a background distribution. 
If a frequency class is for one-or-two-in-a-million words, half a dozen usages in a batch would raise the three counters its 3pk lands on to more than double the expected value.

In other words the anomalously busy words impose a non-Gaussian distribution on top of the underlying background distribution. In summary:

- A Z-score is computed for each counter as if the distribution were Gaussian.  
- Technically, our counters are not Gaussian
	- Firstly because the values are discrete, not continuous
	- Secondly, because while the frequency class is constructed so that all the words have the same frequency, the busywords are by definition non-conforming. 
  - However, Z scores are fairly robust and this is an acceptable technique for isolating anomalies.  
 - The average Z is 0, by definition (Z can be positive or negative), and the higher the Z, the less likely that a large value is just random variation. 
- This is useful for detecting anomalies because
	- A Z score beyond 4.0 would occur less that once per cycle if all words in the frequency class were arriving at their normal background rate. 
    A Z of 5.0 or greater would almost never occur.
	 - By choosing our Z cut-off, we can adjust how freakishly un-random a count has to be for us to treat it as "busy."
- The busy words we are looking for typically bump these Z scores for the corresponding counters up to double digit numbers, a range far beyond what would ever be seen in the background bell curve.

### Where the magic happens:

- The sets of indexes of counters with freakishly high Z scores are collected for each of the three counter arrays. 
Note that we chose Z to keep cardinality of the set quite small relative to the number of counters. 

- The Cartesian product of the three index sets is computed. 
This gives you a large set of three-part keys, some of which should correspond to actual busy words, and the others (the vast majority) are just spurious junk. 
The lambs are separated from the goats by checking for whether each 3pk's corresponds to a value that exists in the global mapping. 
Any token that is not in the mapping is necessarily a goat.

- The 3pk's that exist give you the busy words for the current window. Note that this is a somewhat leaky filter. 
	- Some randomly generated 3pk's will usually correspond to real tokens just by random chance because with a range of 1000, there are only a billion possible keys. 
	- Collisons happen. Occasionally, more than one token could map to a 3pk.

The leakiness isn't as bad as one might suppose. 
The probability of a key collision for a given token is the the cardinality of the 3pk map divided by the cardinality of the key space, which is no more one in several thousand. 

The number of distinct tokens used in a day is in the millions, but over the briefer token window, it is much smaller. 
A mechanism culls the tokens that have counts that have gone to zero from the token counter, and puts them on a queue for the main pipeline to use in culling them from the 3pk mappings. 
This reduces the probability of a given token colliding with an existing 3pk mapping to one in many thousands.

## Processing the Batch of Busywords.

Each of the F frequency classes puts the set of busyword it has computed on a queue to an analysis thread. The results are kept in synch by a concurrency "barrier" to keep the F batches of busywords together.

When the analysis thread gets all F batches of busywords, it does the clustering as follows.

- It filters a configured range of the set of recent Tweets for Tweets that contain at least M busywords. M is configurable and might typically be from two to five.

- With proper configuration, this set of Tweets is quite small compared to the total number of Tweets upon which the batch of busywords was computed. This makes sense because most Tweet subjects at any one moment are not new.

- A graph clustering algorithm finds groups of Tweets among the reduces set that have the most joint similarity in the sets of busywords they contain. (Whether busywords or all words are used is configurable.)

- Clusters can overlap. For instance, one group might be characterized by five busywords, and another by four, but they happen to share one busyword. However, despite that overlap, they the two groups are identifiably different.  

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

## How To Tune Meta-Clusters

After running for 18 hours, we see 3170 meta clusters and 23976 individual cluster, which is about seven individual clusters for each meta cluster. 

Meta-clustering is doesn't seem to be as good at primary clustering. 
Some of them seem ok and some of the seem to be unrelated.  
We need a program for systematically seeing config parameters work and what do not.

Ther are a number of parameters that affect meta clustering. See the config files for details.

## Processing Rate
A large set of enhancements have greatly improved throughput.

The feeder is running on the same box feeding CSV via RabbitMQ.

It is not clear what limits processing. It may be RabbitMQ, which shares the compute platform with the main app. 

This might be considerably faster for historical data if RabbitMQ were eliminated entirely:
- The feeder were writing to STDOUT and the main reading fro STDIN
- The main were reading the files directly

It is also not clear whether the feeder running on another machine with Rabbit writing over the LAN would net slower of faster.

## Speedups

The speedups were a combination of many things that are worth remembering:
- Removing contention caused by unnecessary concurrency protections
- Tightening up encapsulation. Everything is now independent, connected by queues that provide elasticity.
- Better memory management, especially getting rid of old 3pK mappings for zero count tokens
- Deduplication of clusters, i.e. scrapping nearly identical Tweets. (A modified verison of Levenshtein distance that works on the word level rather than the character level.)
- Greatly reduced logging and use of a framework that controls how much gets written

## Confirmation of the Z-score Principle
The data technically violates the assumptions of Z because a Gaussian distribution is for continuous data, not integer counts. (There is an argument to be made for Poission based scoring.)

Also, we pre-suppose that only the underlying frequencies are approximately Gaussian. We are assuming that the busywords impose an explicitly non-Gaussian distribution on top of the main distribution of pseudo random values.

Nevertheless, use of Z in this way is fairly common as we are only identifying anomalies and don't really need to know exactly how anomalous they are.

The following are excerpted from the logs. Note that the low-end Z scores are all quite small negative numbers, as you'd expect.  
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
Most distinct tokens are, for practical purposes, unique on the time scale of a day. The majority, in fact, don't appear more than once in two weeks. Permanently storing the 3pk values for super-rare tokens incurs two very significant costs:
- It wastes a ton of space
- It reduces quality because of high collision rates

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

## Clustering Improvement
Would clustering be improved by weighting the frequency classes?

- The rarer the word class, the more it is worth?
- Or possibly the opposite.

It doesn't seem like it would be that hard to do.
- Make the frequency filters accessible to the analytic thread
- Assign a weight to each busyword based on its F class
- Weighted edges are amost certainly part of the graph algorithm. We probably use them now, but just defaulted to 1.

## Meta-Clusters Don't seem nearly as good as the primary clusters.
See clustering improvement above.
- We now have a choice between clustering on the busy words or clustering on all tokens. 
- Investigate exactly how this is done. 
- Does "all teh words" mean just the medioids or does it mean all the Tweets?

## Dynamic adjustment of the Z minima
Sometimes you see a gross inflation of the number of busy words. It is not clear why.  A facility to dynamically adjust the Z values to keep them at some optimim number might be useful.

Such a facility might take the list of Z scores from config as a starting point and bump them up and down to seek a level that results in an average of B busywords for each frequency pipeline.

Note this would require a decent sized window of batches because there seems to be considerable variance across batches. There seenms to be at least as much variance across batches as there is across frequency classes.

## Possible logic error
We sometimes get notification in the logs of burst of 3pk's not mapping to tokens. 
This should be almost impossible (it says so right in the warning message.) 'Sup with that?

Suspect this is a phenomenon of startup from token counts on disks.
This needs to be confirmed.
 
## New Input Mode(s)
The main is now fed through RabbitMQ. It would be interestin to have a history mode specifically 
for mass consumption as fast as possible.

- Supplement the feeder with a program that just cats the lines in the CSV to standard out. Does it need to be speed limited or will it somehow self-throttle?
- Add a mode for reading from standard in

Alternatively, have a main program mode that reads files of CSV itself.

 
## Ongoing  Items
- Comments Key areas need to be commented to keep out don't touch, etc.

- Testing has been neglected. Make tests around everything.

- More fully populating the token_filters.txt file. Check the busywords in the logs and see what else jumps out.  This may be nearly done.
 

## Major: Improving Busy Word Detection Quality with Computing Redundantly

This would be a significant effort.  Interesting idea, but it's not 100% clear that it's worth doing. We need some investigation.

Consider that dual sets of pipelines, or even three sets, each with different hash functions, could do a much more accurate job of filtering out the true busy words for a given set of parameters.  
 
It is not clear how big an impact this would have on performance. All the BW processors are doing is counting and periodically computing Z on a thousand values in each of the three counter arrays. There is a tiny bit more work in the analysis to take only the words that appear in the required number of sets. It doesn't really affect the analysis phase that follows, and it's just a little more work for the main to put the tokens on more queues than before.

Risk. If multiplying the work in the busyword processors made them in aggregate slower than the combined main pipeline and the clustering, it would cause the queues to grow without bound and crash the program. You'd need to detect the problem and throttle the reads if this is a problem. Actually, this should probably be done anyway! Who knows if some combination of config parameters could cause this to happen.

## Possible Blow-up From Overload
Relating the the previous item, if the frequency processors or the analysics processors were overwhelmed, the queues that feed them would bloat without bound, causing failure from OOM.

This implementation is not bullet proof in that respect. 

Need to investigate the possibility of periodically checking queue lengths and throttling Tweet consumption or taking other measures if the analytic part is getting overwhelmed.

- This is easy for historical data--just consume more slowly.

- For real-time feeds you might need to discard an sliding percentage of the input. 
   

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
