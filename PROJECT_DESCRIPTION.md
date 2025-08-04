# Session Notes

This document covers
- Two goals of the project
- What it does 
- Notes on why conventional frequency and semantic analysis aren't enough
- Notes on how it works

# Project Goal
This project has two goals. 
Both concern the ability to understand what subjects people are talking about on X/Twitter in near real time at a granularity of seconds.

## The Narrow Goal: Getting New Subjects (News)From the X/Twitter firehose in Real Time

The weaknesses of X/Twitter and similar micro-blogging platforms has always been the difficulty of finding out what is there. Seen the other way around, it is the difficulty bloggers have connecting with readers with whom the don't have a prior relationship. There's no TV-Guide.

In practice, this problem is mostly solved by punting it to the users.  It's up to the blogger to markup posts with hashtags, and up to the receiver to discover people, hashtags, etc. to subscribe to. Ironically, this is usually done indirectly, by finding interesting people by other means, then seeking them out on X.  
 
Even important news often takes on the order of hours to surface in conventional media, so real time identification within seconds is a major advantage if knowing things first matters. When you see a subject characterized by "train, chemical, crash, evacuation, and environment" suddenly pop up, you may want to short Dow Chemical.
 
## Broader Goal: History

Treating the history of X/Twitter as an historical resource or as a way to understand current events is problematic because of the scale of computing required. It's extremely difficult to do it at a fine grain without huge computational and technologist resources.

To be able to see the subjects that arise and ramify minute by minute during events as ordinary and well-defined as the Super Bowl or a concert is eye opening. Every song or play gets a burst of talk. The possibilities for events as complex as evolving conspiracy theories, the BLM movement, the Arab Spring, or the invasion of Ukraine have real possibilites for historian, journalists, and understanding the public mind for public safety, etc. 

With the resources likely to be available to researchers, it would be difficult to take advantage of the data unless it were segmented in dimensions such as:
- Time slice
- Analyzing only a random longitudinal fraction of the full stream
- Possibly most importantly, by limiting the granularity of examination, i.e.,look at subjects that are big enough and persistent enough. 

# What Is Different Here?

The most important insight is that identifying all of the subjects in play at a given moment is useless. It is too much data--many thousands of subjects are too many to make sense of.  The key is to identify only new subjects. 
- You get the most relevant immediately
- In the limit, you get them all
- It's a flow a human can grasp and AI or semantic analysis can cope with at reasonable computational expense.
  
Perhaps the most import insight is that "subjects" are not characterized by common words. "Trump", "Diddy", "Superbowl", and "Taylor Swift" characterise thousands of subjects.  What distinguises a new subject is the joint occurrance of clusters of normally rare words that are being used unusually frequently. The common words are just along for the ride.

If you can identify anomalously busy words fast enough that you can find them jointly occurring among the most recent few seconds of Tweets, clustering becomes trivial.

# Statistics and Semantics

With Tweets arriving at several thousand per second, it is only practical to cluster them into subjects if you can do it within a very few seconds. Any interval in two or more digits is too coarse, in part because clustering algorithms are inherently non-linear.  It sounds tautological, but in order to be fast, you have to be fast.

Relative frequency analysis is computationally expensive, slow, coarse grained. it is just very hard to do it every few seconds on a universe of words with a cardinality in the tens of millions. You can't just ignore the rare words because the overwhelming majority of words are extremely rare. The set of Tweets you have to cluster over is in direct proportion. Not only does the clustering time increase, the granularity goes down.  
 
Semantic analysis is vastly more time consuming. Semantic analysis can be parallelized, but at great cost, and even if you could derive a "subject" for each Tweet semantically, It's not clear how you would then cluster them. The semantic analysis is just the predicate for the clustering operation.


### Semantic Insight is More Helpful In Understanding Subjects Than In Finding Them

The key thing is that semantic analysis isn't necessary, and arguably, is misplaced as a way to identify clusters of subjects. It is better suited to understanding subjects that have been identified. More on this later.

# A Better Way
 
Avoiding semantic analysis in favor of pure statistics is the key, but it's critical to avoid any kind of statistical analysis that puts counting words and analyzing relative frequency in the computational path. Which sounds like a contradiction.

The important observation is that new subjects are characterized by ordinarly rare word that are suddenly being used frequenly appearing together in the same Tweets. 

Most words in fact are extremely rare. Words famously have a Zipf distribution in which the use of a word is in inverse proportion to it's rank. I.e., the frequency of the x most common word is in proportin to 1/x.

The distribution tail is so long that the most common 250 words in English language Tweets are used more than the next 11,000,000 words combined! Thus, by a wide margin, the most common number of times per day for a randomly selected word to be used is zero, and the next most common number of times is one, again by a wide margin.  Therefore, when you find a cluster of several normally one-in-millions words suddenly appearing together in the same Tweets dozens of times in a period of ten seconds, you almost certainly have identified a "subject."

In theory, if machines were infinitely fast, you could probably identify anomalously frequent words using relative frquency, but it's just too much computing to do in single-digit seconds continually.


## Relative Frequency Turns Out to Be a Red Herring

At first glance, computing relative frequency seems to be the problem, but looked at a different way, you don't really care about relative frequency per se. The only reason to compute it is to use it to recognize surges in relative frequency. The thing you care about is the first derivative of the frequency, not the frequency.  

It turns out, there is a way to find these surges in frequency without computing the frequency itself, at least not inline, i.e., in the main processing path.

You can't get away global frequency computations entirely. The heuristic to find surging words needs to have some idea of the background frequency of a word's use because it requires partitioning the stream of words into categories of frequency of use. 

Frequency calculations need to be done periodically in the background because you need keep up with background word usage changes caused by time of day, day of week, etc., as well as with ongoing changes due to the emergence of new subjects that people Tweet about. But you can get away with doing them only periodically, and offline with respect to the main processing pipeline. While busyword and subject analysis must place at intervals of a few seconds, these offline frequency analysis operations can take place at intervals of many minutes. And they are transparent to the main flow of data.

## Properties of the Heuristic

The heuristic can be fast because it is only sensitive to the leading edge of a surge in usage of a word. It is not explicitly aware of the actual relative frequency.

Because it only sees the leading edge, it is inherently blind to a sustained increase in frequency. Threfore,it   
- Automatically forgets words that surge only briefly in usage
- Forgets words that surge and then stay at an elevated level unless/until that word again surges in relative frequency.

Because it does not explicitly count words, it is fast. On a four-core System76 Laptop, it can identify new subjects and cluster them into groups of Tweets at about 8000 to 9000 Tweets/second, which is substantially more than the full firehose. 
 

# Data Set

The input available for development purposes is two weeks of the decahose (about 500 Tweets/second.) It consists of JSON-formatted Tweets in files that have the file order encoded in the filenames as Unix start and end times. The files are about five minutes of Tweets at about 500 Tweets/second. The original files are unpacked into uncompressed CSV files.  

These can be processes at about 15x the actual rate of the decahose.

In production the input could be either files (for historical processing) or the actual live Tweet stream (for real time processing).  

Using files is not a "cheat" because any system would typicaly receive Tweets and feed them somehow. The actual development feed is from a process running on the same meachine and sending the Tweets in via RabbitMQ.

The non-graphical output is a series of clustered sets of Tweets that are about new subjects. There is not yet a graphical front end.
 

# The Approach

The fundamental principle is that "subjects" are best identified by joint use of shared sets of words that are suddenly appearing with anomalous frequency, referred to here as "busywords."
  
  
Counterintuitively, when it comes to identifying subjects, it is the rare, offbeat words that tend to drag the super-common names along with them, rather than the other way around. Common words, like "the", "a", "Trump", "Beyonce", or "Swift" contain almost no information because they appear in thousands of ongoing conversations. It is proper names of people and places, economic, scientific, geographic, or political terms, suddenly popping up together, that usually signify that people are saying something new. 

The key to making it work is that the sliding window on the stream of Tweets has to be small, e.g., typically in the range of perhaps ten thousand Tweets. With too wide a window the granularity of the view erodes and the computational cost increases disproportionately.  

So sum up, the heuristic
- Detects the leading edge of surges in word usage.
- Finds the Tweets in the current window that use a proper subset of these words
- Discards all the other Tweets
- Clusters the remaining Tweets by the busy words they use. Clusterings need not be unique or deterministic.

We optionally do one more thing. We can also cluster the clusters over the preceding multiple batches of processing to discover the persistence of subjects.

The interesting part is the first thin: detecting the leading edge of a surge in frequency.

  
## Background Word Frequency Computations

The main processing pipeline normalizes the words from each incoming Tweet and puts them on a queue that is read by an offline thread that does frequency calculations in the usual way on a sliding window of the stream of words (tokens.)
The size of this window is set in configuration. It is in the millions.

The tokens are written to disk in ordered batches, so that when the target size of the window is reached, the older batches can be read in and the tokens subtracted from the counts.

The background process converts the counts into frequency statistics that are turned into filters and supplied transparently to the main processing loop. These are used to partition the stream of incoming words into equivalence classes based on frequency.

Each frequency class gets approximately the same number of usages, but they comprise wildly differnt numbers of unique tokens. The lowest frequency class--one usage in millions, has more tokens by far than all the rest put together. The most frequent class has a couple of hundred words and has just as many usages.

Note that new words come along all the time, even after weeks of the firehose. A novel word doesn't match to any frequency class the first time it is seen and therefore won't show up in the frequency filters (until they are recomputed.)  This is not a problem, however. The window is large, so any incoming word that doesn't match to a frequency class is almost certainly very rare and can therefore default to being treated as being in the least frequent class which, courtesy of Ziph's law, which will have only one occurrence of each word.  This is a special case of the general reason why the token window must be large.

All of this happens in the background and not in the main processing pipeline. 
 
## Receiving Tweets

The main routine reads inbound Tweets from RabbitMQ in CSV format.  

The receiving phase:

- Maintains an in-memory sliding window of the latest Tweets. This is not to be confused with the window of tokens used for frequency analysis. The Tweet window is much shorter and holds the entire Tweet struct for every Tweet that contributed to the last several cycles of busy-word processing. This might typically be a minute or so of Tweets. They are continually aged out and discarded to limit the size in memory.

- Extracts the words from each incoming Tweet and normalizes them. This include unifying the case, dealing with diacritics and such, discarding junk words, etc. The result is a stream of meaningful tokens.

- Puts the tokens on a queue for the off-line frequency calculations to use for the periodic asynchronous frequency calculations.

- Obtains the frequency class for each token using the frequency class filters.

- Looks up the "three part key" (3pk) for each token. If a 3pk doesn't yet exist, it computes one for the token and inserts it into the lookup table. (More on 3pk below.)

- Puts the 3pk on the busyword processing queue for the token's frequency class.

When one "batch" (perhaps 10) of incoming Tweets has been received, the main processor puts a special 3pk that cannot correspond to any real token on each queue to signal to the busyword processors that the latest batch is complete. The busyword processors all get the signal at the same place in the Tweet stream, which keeps them synchonized.

With the handing off of the 3pk's for the tokens in the current Tweet to the busy-word processor queues, the main processing loop is at its end, and it starts over with the next Tweet. None of the processes need to wait for each other, as queues provide elasticity between the processing steps.

 
## The 3pK's
We haven't said exactly what a 3pk is.  A 3pk is just an ordered triple of hashes modulo C for some token or word.  The hashes are each parameterized with a different value, so it is really just three deterministic but pseudo-random numbers in a defined range.  This size of this range is a global parameter, but it is ordinarily somewhere around a thousand or so for reasons that will be made clear later.

As mentioned above, a global mapping of 3pk's to tokens, and tokens to 3pk's is maintained. If a 3pk corresponds to an existing token, it will always exist in this mapping.

If the 3pk hash range is say, 1000, that gives a 3pk a logical space of a billion different pseudo-random triples. With a typical window size, there are typically fewer than million distinct words in the global computation at any one time, but there is definitely a collision for any given token. However, as we will see later, collisions are neither frequent nor very harmful.  

The range can be increased to reduce this probablility, but there are computational costs to a larger range. How to compute the optimal range size is an open question.

## The Busyword Processor Threads

The busyword processor threads are the heart of the algorithm. Each of the F processors works the same way. Finding the busywords for a frequency class is incredibly cheap computationally.

The frequency partitioning ensures that the background frequency of the tokens received by a busyword processor are appoximately equal.

The hashes from each 3pk are used to index into three corresponding arrays of counters of size C, the same as the range for the hashes. Thus, one counter in each of the three arrays gets bumped for each incoming 3pk.

The math is quite simple. Because the 3pk values are pseudo random, if every word had the same probability, the counts would have a Gaussian distribution.  

However, we are assuming that some of the words assigned to a given frequency class are anomalously busy (after all, that is the point of the exercise.) Therefore, some of the counters will have exceptionally high counts.  For instance, if you have 10,000 Tweets in a batch, with an average of 10 tokens in each Tweet text, that's 100,000 tokens spread over, say, 24 busyword processors. This means each processor would get about 4,166 tokens, and the counter values would average about 4.166 when the signal 3pk is detected (We use {-1, -1, -1} for the signal 3pk.) When the signal is received, the processor suspends reading the queue and processes the batch.

- A Z-score is computed for each counter as if the distribution were Gaussian.  
	- A Z-score is a normalized standard deviation that gives the improbability of any given sample value's deviation from the mean due being due to randomn variability. 
	- Technically, our counters are not Gaussian
		- Firstly because the values are discrete, not continuous
		- Secondly, because while the frequency class is constructed so that all the words have the same frequency, the busywords are by definition non-conforming. 
	- However, Z scores are fairly robust and this is an acceptable technique for isolating anomalies.  
 	- The average Z is 0, by definition (Z can be positive or negative), and the higher the Z, the less likely that a large value is just random variation. Any statistics book will give a fuller explanation.  
- This is useful for detecting anomalies because
	- A Z score beyond 4.0 would be expected by random chance no more than about once per cycle if all words in the frequency class were arriving at their normal background rate. A Z of 5.0 or greater would almost never occur.
	- By choosing our Z cut-off, we can adjust how freakishly un-random a count has to be for us to treat it as "busy."

One of the configuration parameters is an F-length array of Z-values, one for each frequency class (as some classes tend to have more natural volatility than others.)

Here is where the magic happens:

- The sets of indexes of counters with freakishly high Z scores are collected for each of the three counter arrays. Note that we chose Z to keep cardinality of the set quite small relative to the number of counters. A Z that limits the average number of high values to under ten would be reasonable value.

- The Cartesian product of the three index sets is computed. This gives you quite a large set of three-part keys, some of which should correspond to actual busy words, and the others (the vast majority) are just spurious junk. The lambs are separated from the goats by checking for whether each 3pk's corresponds to a value that exists in the global mapping.

- The 3pk's that exist give you the busy words for the current window. Note that this is a somewhat leaky filter. Some randomly generated 3pk's will usually correspond to real tokens just by random chance because with C=1000, there are only a billion possible keys. 

## Processing the Batch of Busywords.

Finding the busywords is the tricky part. Using them to identify clusters of Tweets is fairly simple.

Each of the F frequency classes puts its set of busyword on a queue to an analysis thread. The results are kept in synch by a concurrency "barrier" to keep the F batches of busywords together.

When the analysis thread gets all F batches of busywords, it does the clustering as follows.

- It filters a configured range of the set of recent Tweets for Tweets that contain at least M busywords. M might typically be 2, 3, or 4, but any number can be used. The set of Tweets to be searched usually corresponds to the frequency interval of the busy word computation, but it can be configured.

- With proper configuration, this set of Tweets is quite small compared to the total number of Tweets upon which the batch of busywords was computed. This makes sense because most Tweet subjects at any one moment are not new.

- A graph clustering algorithm is applied to the set of Tweets that contain at least the minimum required number of busywords. The algorithm finds groups of Tweets that have the most joint similarity in the sets of busywords they contain. 

- The clustering is a non-deterministic process because clusters can overlap a little. For instance, one group might be characterized by five busywords, and another by four, but they happen to share two busywords. However, despite that overlap, they the two groups are identifiably different.  These properties can be tuned to suit.

## The Output
The is text. The clusters are computed and grouped in clusters that are printed out as an ASCII tree. 
 
Various paremeters allow configuration of how much data is produced. Things like minimum number of busy words in a Tweet, degree of similarity, etc.

A graphical front end is in the offing.
 

# Speed of Computation and Significant Events

It is very fast. A laptop will process a stream of about 8000 to 9000 Tweets/second.
 
The two-weed Tweet sample was recorded from two weeks of a Decahose. However, the Tweets can be consumed at any speed. 
 
Each CSV file is about five minutes of the decahose, i.e., about 30,000 Tweets per file, or 33 files per million Tweets, which is about 2.7 logical hours of the decahose. 
We are actually processing 2.7 logical hours in about 10 minutes.
 
If you are starting and stopping frequently for development purposes, the system can optionally read in the existing frequency data, for an instant start.  Beware that if you are starting at some random point, the saved frequency data will be wrong to an unpredictable degree. This means you need to either start fresh or disregard the results until you have run for long enough to completely update the background statistics.

You can use the utilities mentioned below to find the date/time you want to start at a specific place in the two-week interval.

### Persistence of Subjects

In addition to computing the new subjects, the system can give information about how long those new subjects have existed in terms of the number of batches. This is given in the form of a list of the previous N batches that also match the new subject.

Persistence of subjects is a significant consumer of CPU, bigger than the cost as the clustering itself.  It can be turned on and off, but the effectiveness of turning it off is still to be determined. It's not clear that it affects throughput, which may be bottlenecked elsewhere.


# Confirmation of the Z-score Principle
The data technically violates the assumptions of Z because a Gaussian distribution is for continuous data, not integer counts. There is an argument to be made for Poission based scoring.

Also, we pre-suppose that only the underlying frequencies are approximately Gaussian. We are assuming that the busywords impose an explicitly non-Gaussian distribution on top of the main distribution of pseudo random values.

Nevertheless, use of Z in this way is fairly common as we are only identifying anomalies and don't really need to know exactly how anomalous they are.

The following are excerpted from the logs. Note that the low Z scores are all quite small negative numbers, as you'd expect.  The high z scores are up in the double digits. This says it's spotting anomalies effetively.

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


# 3pk Collisions
Most distinct tokens are, for practical purposes, unique on the time scale of a day. The majority, in fact, don't appear more than once in two weeks. Permanently storing the 3pk values for super-rare tokens incurs two very significant costs:
- It wastes a ton of space
  - With a token window size of three million, and fifty token files on disk, each time we age out a cached file of tokens, about 20k tokens that are in the counter map go to zero.
  - The FCT thread removes these zero count tokens from the counter map to prevent bloat.
  - However, the main thread keeps a 3pk <-> token map that formerly had no way to age out useless token mappings. 
  - Those useless mappings accumulate a cost about 100MB of memory per hour to store permanently.  
- The bloated 3pk<->token map also increase the probability of collisions. 
  - There are something like 135M distinct tokens in the two-week data set.
  - The 3pk mapping space is only one or two billion. If you kept them all, 3pK collisions would be constant.

We solve this bloat problem by having the FCT put tokens on a queue when their counts in the FCT's counter map go down to zero. 

The main thread takes a chunk of thes values from the queue every few hundred Tweets and deletes the corresponding entries in the 3pk<->token mapping.  

This is harmless, because if the token ever shows up again, the main thread will automatically recreate it anyway.


   
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

 