# Session Notes

The document PROJECT_DESCRIPTION.md has information on project goals, what it does, and most general interest material.

This document covers
- Loud warnings to Cursor not to mess with any code without asking
- Instructions for running the main program and utilities
	- There are numerous parameters for the main program
	- Utilities for pre-processing the data
	- Utilities for analysis
- Notes on throughput and behavior
- Lessons learned
- TTD for fixes, enhancements, analysis, etc.


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


# Speed of Computation and Significant Events

We can do about 16 decahoses on the System76 Laptop.

The dataset: 
- Starts around 2012-01-28 16:16:46, i.e. quarter after five on New Years Day
- Ends at 2012-02-15 03:22:36, which is 3:22 AM the day after Valentine's day.

There are a number of newsworthy evens during that period.

## News of Whitney Houston's Death

Her death was discovered at 3:30 PM PST February 11, 2012 and she was pronounced dead at 3:55 PM PST

So that would be 11:30/11:55 GMT aka Zulu i.e. 23:30/23:55 GMT

### Super Bowl
Note the superbowl is also a great place to see real conversations starting up. It occurs on Feb 5 2012.  

## Beyonce Has a Baby

## The Lion King 

## The Grammy Lead Up


# Data Set Time Fields
 
The timestamps in the original data are Unix time, which is Zulu, aka GMT, aka UTC.


# Distinct Tokens for English (en)
This is interesting because is shows how extremely Zipfy the data is.  

- 11,522,129 distinct tokens appear in "en" Tweets.  
- The top 220 tokens (which are almost all words) account for half of all usage.
- The top 3,572 tokens account for 80% of all usage.
- The top 13,690 tokens account for 90% of all usage.
- The top 615,473 tokens account for 99% of all usage.
- The other 95%, 10,906,656, tokens account for only 1% of all usage.
  
It's mostly the relatively rare words we care about because they carry the most meaning when they surge in usage.
 
## Tests
We have fallen behind a little in unit tests!  Some of this may be obsolete. 

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

## Processing Rate
A large set of enhancements have greatly improved throughput.

Formerly, it was getting 1.1k Tweets/second on the iMac and a little more than 2.1 on the 76 laptop. Now it is more like 2300 on the iMac and 8000+ on the System 76, which is also a four-core machine.  That is 1.6 firehoses.

The feeder is running on the same box feeding CSV via RabbitMQ.

Throughput might increase further if either:
- The feeder were writing to STDOUT and the main reading fro STDIN
- The main were reading the files directly

The latter approache is very consistent with using this for historical data.

It is also not clear whether the feeder running on another machine with Rabbit writing over the LAN would net slower of faster. You move CPU consumption to another machine, but you add a network hop. Who knows?

The speedups were a combination of many things that are worth remembering:
- Removing contention caused by unnecessary concurrency protections was a big one.
- Tightening up encapsulation. Everything is now independent, connected by queues. Hardly anything ever has to wait for a resource.
- Better memory management, especially getting rid of unused 3pK mappings for zero count tokens
- Deduplication of clusters, i.e. scrapping nearly identical Tweets. (A modified verison of Levenshtein distance that works on the word level rather than the character level.)
- Greatly reduced logging and use of a framework that controls how much gets written

## Confirmation of the Z-score Principle
The data violates the assumptions of Z because a Gaussian distribution is for continuous data, not integer counts. There is an argument to be made for Poission based scoring.

Possibly more significantly, while we pre-suppose that only the underlying frequencies are approximately Gaussian, the busywords impose an explicitly non-Gaussian distribution on top of the underlying distribution of pseudo random values.

Nevertheless, use/abuse of Z in this way is fairly common for anomaly detection. We are only identifying anomalies and don't really need to know exactly how anomalous they are. "Very anomalous" is accurate enough.

The following are excerpted from the logs. Note that the low Z scores are all quite small negative numbers, as you'd expect with a Z distribution.  The high Z scores are up in the double digit positive numbers. This says it's spotting anomalies effetively.

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
Most distinct tokens are, for practical purposes, between unique and non-existent on the time scale of hours. The majority, in fact, don't appear more than once in two weeks. There is little value in storing 3pk mappings for these when they can easily be recomputed. Blindly storing them incurs significant costs. 
- It wastes a ton of space
  - With a token window size of three million, and fifty token files on disk, each time we age out a cached file of tokens, about 20k tokens that are in the counter map go to zero--about 40%.
  - The FCT thread removes these zero count tokens from the counter map to prevent bloat.
  - Those useless mappings accumulate a steadily growning cost about 100MB of memory per hour to store permanently.  
- More importantly, a bloated 3pk<->token map increases the probability of collisions. 
  - There are something like 135M distinct tokens in the two-week data set.
  - The 3pk mapping space is in the range of one or two billion, eventually leading to better than a one in eight chance of a collision even in our limited data set.  

We solve this bloat problem by having the FCT put tokens on a queue when their counts in the FCT's counter map go down to zero. 

The main thread takes a chunk of thes values from the queue every few hundred Tweets and deletes the corresponding entries in the 3pk<->token mapping.  

This is harmless, because if the token ever shows up again, the main thread will automatically recreate it anyway before it is used.

## Seemingly Excessive Token Rejection

We are getting huge percentages of rejection of tokens because of too-short token length and/or tokens being on the ingore list. More than 30% and 40% respectively. The list of rejected tokens is in config/token_filters.txt.
 
Those numbers were not supurious. Turning that functionality off has the expected effect. This means both are very effective and doing their job. You definitely want min length = 3. More might be better, except that it would rule out important TLA's.  The two practices strip out a huge amount of processing at the very beginning of the pipeline.

## Effect if Computing Cluster Persistence

I went from checking over six batches to checking over 1 batch without producing a huge speedup. Maybe 100/second faster.

### Checking all words v checking busy words when doing cluster persistence checking
This aspect of the problem has NOT been reviewed
 

## Is token splitting on periods, commas, and dashes happening?

No, it was not happening, but that is fixed.  The token cleanup in Go now does this. Note that we can optionally split or not split on apostrophes. (See config.yaml) The default is to not split so that O'Brian and M'kele M'beme, and f'oc'sle can continue to be tokens.

## Overhaul of ASCII Output and Reduction of Logs

Logging has been overhauled with almost all of it now going to the designated log file and almost none to the screen except some startup info that gets written to stderr.

The program output is now all in JSON format sent to stdout. It can be configured to be more complete, for machines, or more readable, for humans. 

The rest of the logging now has the customary DEBUG/INFO/WARNING/ERROR levels in config.yaml

The new policy is to put only critical messages on the screeen and to write them to stderr so we can preserver stdout for proper output.
    
## Annoying Nonsense Tweets

We now have a file called banned_phrases.txt that contains a number of phrases that occur in a disproportionate number of essentially meaningless Tweets. No doubt the contents of such a file should be adjusted to a moment in time.
Needless to say, it can be left empty.

"The awkward moment"

"That awkward moment"

...

"Do you want more Followers" 

# TTD and Direction

## Write a GUI Interface For Canned Results 

## Produce a flag that supresses all the Tweets other than the medioid for compact output.

It's not as exciting as live, but it would be quite useful for exploring.

## A Possible Logic Error
We get notification of burst of 3pk's not mapping to tokens. This should be almost impossible (it says so right in the warning message.) 'Sup with that?
This be an artifact of startup reading token files that are out of date with respect to the restart.
 
## Possible New Input Mode(s)
The main is now fed through RabbitMQ. It would be interestin to have a history mode specifically 
for mass consumption as fast as possible.

- Supplement the feeder with a program that just cats the lines in the CSV to standard out. Does it need to be speed limited or will it somehow self-throttle?
- Add a mode for reading from standard in

Alternatively, have a main program mode that reads files of CSV itself.
 
## Ongoing  Items
- Comments key areas need to be commented to keep out don't touch, etc.

- Testing has been neglected. Make tests around everything.

- More fully populating the token_filters.txt file. Check the busywords in the logs and see what else jumps out.  This may be nearly done. Apostrophized words may need to be added.
 

## Major: Improving Busy Word Detection Quality with Computing Redundantly

This would be a significant effort.  Interesting idea, but it's not 100% clear that it's worth doing. We need some investigation.

Consider that dual sets of pipelines, or even three sets, each with different hash functions, could do a much more accurate job of filtering out the true busy words for a given set of parameters.  
 
It is not clear how big an impact this would have on performance. All the BW processors are doing is counting and periodically computing Z on a thousand values in each of the three counter arrays. There is a tiny bit more work in the analysis to take only the words that appear in the required number of sets. It doesn't really affect the analysis phase that follows, and it's just a little more work for the main to put the tokens on more queues than before.

Risk. If multiplying the work in the busyword processors made them in aggregate slower than the combined main pipeline and the clustering, it would cause the queues to grow without bound and crash the program. You'd need to detect the problem and throttle the reads if this is a problem. Actually, this should probably be done anyway! Who knows if some combination of config parameters could cause this to happen.
   

## Major: Clustering Across Batches

Note, this has now been implemented in the form of looking to see many cycles a cluster has been present for. 

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

## The Big Token Window.

People don't Tweet the same things at breakfast on Monday that they do at 1:00 AM on Saturday.  Also, the world turns, and while New Yorkers are getting up in the morning, people in Bejing are out for the evening.

Also, surges in usage continually change the current frequencies.

Because most words are extremely rare, and words beyond the rank of a few thousand it still takes quite a lot of tweets to get even a rough estimate of frequency. Consider that
- "Dog" is about the 1000th most common word, yet only 1/10,000 of words will be dog. 
- "Mirror" is the 3000th most common word, and only 3 words out of 100,000 will be mirror.  
- "Edge" is the 5000th most common word, and it appears only once in 100,000 words

Therefore, you need a lot of words (millions) to get even a reasonably accurate estimate of a word's background frequency. The decahose is 500 Tweets/second or about 5,000 words/second, which is about 3.3 minutes of data. So a window of five million words is about 16.6 minutes.

Three million is a reasonable window size because it's easily fine enough to keep up with the time of day, and not so coarse that it takes a long time to age surges in frequency out.

You have to age tokens out if you want a stable size window, and that gets to be a lot to keep in memory and a lot to do token by token. 

Accordingly, the tokens underlying the token counters are written to disk in batches. After the number of token batch files reaches a defined limit, each time a new batch is written out, the oldest batch is read in (and deleted from disk). The contents of the old batch are then used to decrement the global token counts.

This keeps the token counts relatively up to date with the flow of Tweets.  

Note that frequency calculations would typically happen multiple times in the span of time represented by an entire token window.  They are controlled by separate parameters. Say you have a five million word window, i.e., 16 or 17 minutes, you might recompute the frequency filters every couple or three minutes.

## The Small Window of Tweets

The big token window is only of concern only to the off-line frequency calculating thread.  The main thread keeps short window of Tweets in memory for use in the clustering algorithm, which is in the main processing line.

This is a conventional queue that keeps a configured number of Tweets, ageing out the old ones as new ones are added.  This would be configured to hold a few processing batches of Tweets. As a batch is typically in the range of one thousand to a few thousands, it would be a small multiple of that size.  

Clustering happens with respect to the latest batch.  However, the persistence of clusters is tracked across previous batches. You can see in this way whether a subject just popped up, or has it been around a while.

The size of a batch, the number of batches kept in memory, and the number of batches used to compute a clustering are all configurable.
 

# Running the Profiler

The profiler is very useful for finding where the CPU goes and whether you are suffering from contention due to concurrency. You can run the program for a while with the following command line to get profile data.  Run it for a good while so that the output is not dominated by startup processing, which is more or less irrelevant to steady state performance.
 
 ./main -config ./config/config.yaml -print-tweets=false -profile

 Let it run

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
- The big Tweet set is in ../twits/msg_output_3

- The same data set post-processed for language identification is in ../twits/test_language_detect_out

- The full dataset of 5399 files has 598,725,870 Tweets and is about 105 Gigabytes in uncompressed CSV format (Which is significantly smaller than the compressed JSON.)

- The analysis program reports that there are 
  - 44,298,119 distinct tokens in 137 million Tweets, 
  - 135,315,247 distinct tokens in 584,230,000 Tweets

The analyis program is pretty fast, at 52,683 Tweets/sec    

- 599 million Tweets equals approximately six billion tokens give or take.  
 
- Volume of decahose = 500/sec

- Volume of firehose = 5000/sec

- 500/sec = 30k Tweets/min = 450,000 Tweets/15min  = 1.8m Tweets/hour in nominal Tweet time, or 3m 45s in real time on the fast laptop.

- Average number of words per Tweet is approximately 10.

## Our Processing Speed
- The sender can send about 3000/second with ACKs. Which you need, or it will simply drop msgs.

- We process about 2300/second on the iMac, app  4.6 decahoses.
- We process about 8500/second on the System76, or about 16 decahoses.

- The Tweets get ACKed, so RabbitMQ manages to never let the queue get very long--it seems to never get up to 50.
  
- Neither machine is old. The iMac is about 13 years old, and the System 76 is about 5 years old. Both have four hardware cores.

- Note that for historical data it will scale in direct proportion to the number of machines because you can assign large time slices to each machine. This means you can do bulk processing essentially as fast as you are willing to pay for.

- We have about 350 hours of decahose. The fast machine can do it all in less than 24 hours. 
 