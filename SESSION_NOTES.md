# Session Notes

This document covers
- Loud warnings to Cursor not to mess with any code without asking
- Two-fold purpose of the project
- Notes on whay conventional frequency and semantic analysis aren't enough
- Notes on how it works
- Instructions for running the main program and utilities
	- There are numerous parameters for the main program
	- Utilities for pre-processing the data
	- Utilities for analysis
- Notes on throughput and behavior
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


# Project Goal
This project has both a narrow goal and a broader goal. Both concern the ability to understand what people are saying on X/Twitter in near real time at a granularity of seconds.

## The Narrow Goal: Getting New Subjects (News)From the X/Twitter firehose in Real Time

The weaknesses of X/Twitter and similar micro-blogging platforms has always been the difficulty of finding out what is there, or seen the other way, the difficulty bloggers have connecting with readers with whom the don't have prior. There's no TV-Guide.

In practice, this problem is mostly solved by punting it to the users.  It's up to the blogger to markup posts with hashtags, and up to the receiver to discover people, hashtags, etc. to subscribe to. Ironically, this is usually done indirectly, by finding interesting people by other means, then seeking them out on X.  

The project presents a way discover the new subjects appearing in the Tweet stream in near real time, i.e., within a span of time comparable to how long it might take to read a Tweet amd compose a response. 

Even important news often takes on the order of hours to surface in conventional media, so real time identification within seconds is a major advantage if knowing things first matters. When you see a subject characterized by "train, chemical, crash, evacuation, and environment" suddenly pop up, you may want to short Dow Chemical in a hurry.
 
## Broader Goal: History

To be able to analyze the full stream at a granularity of seconds and at a multiple of the real firehose rate makes it possible to study this ephemeral record of history in a new way. 

You can see the subjects that arise and ramify minute by minute during events as ordinary and well-defined as the Super Bowl or a concert, or as complex as evolving conspiracy theories, the BLM movement, the Arab Spring, or the invasion of Ukraine.   

To study the flow in real time by conventional means takes an enormous investment in equipment and technical expertise, and it may not be effective because one way or another, it generally requires at some level, knowing what you are looking for in advance.

With the resources typically available to researchers, it is often only practical to analyze limited dimensions of the stream, which must be segmented by:
- Subject
- Time slice
- Analyzing only a random fraction of the full stream
- Possibly most importantly, by limiting the granularity of examination, i.e.,look at subjects that are big enough and persistent enough. 

 
# Statistics and Semantics

A driving insight is that it would be essentially useless to 
analyse all the subjects that are active at any one time because this number is enormous by any reasonable definition of "subject." 

However, if one settles for only seeing new subjects as they arrive:
- The amount of data for a human to be exposed to is manageable
- The subjects arrive newest, i.e., most important, first
- As time passes, you asymptotically approach getting **all** current subjects.
- Focussing on newness at the level of seconds, you can see a new subject ramify into sub-subjects in near real time. You can watch the hive-mind communicate with itself.

While X/Twitter is a firehose of data, fortunately, new subjects tend to arrive at a manageable rate. Depending upon exactly how you parameterize the definition of "subject," new subjects that will ever involve more than a handful of Tweets arrive perhaps every few seconds. Significant subject arrive at an almost stately pace. With proper tuning, you get something with about the information density of the Times Square News Ticker.

To find these subjects by characterizing the semantics of thousands of Tweets per second, and then grouping them together based upon subject similarity is a daunting task computationally. It's not even clear how to do it, as it's not just a matter of understanding Tweet-by-Tweet. Semantic understanding is useful only if you can related subjects of Tweets into clusters. Certainly it would be extremely difficult and resource intensive to do this in real time on thousands of Tweets per second. 

### Semantic Insight is More Helpful In Understanding Subjects Than In Finding Them

The critical insight is that semantic analysis isn't necessary, and arguably, isn't particularly helpful. In practice, groups of meaningful words used together are an excellent proxy for subjects, and semantics can be ignored in the process of indentifying new subject in the stream.

### Pure Statistics
 
Frequency-based approaches seem like an obvious way to go, but it turns out that frequently used words are not very useful for finding new subjects. At any given moment, literally thousands of subjects include "P Diddy", "Donald Trump", "Taylor Swift", or "Superbowl."

The words that characterize identifiable subjects are more often words that:
- Are usually rare, perhaps not normally used at all.
- Are suddenly being used anomalously frequently. 

Conveniently, words have a Zipf distribution, which means that overwhelming majority of words are very rarely used. By a wide margin, the most common number of times per day for a randomly selected word to be used is zero. The next most common number of times is one, again by a wide margin. 

The distribution tail is so long that the most common 250 words in English language Tweets are used more than the next 11,000,000 words combined. 

The importance of this is that when you find several one-in-millions words that normally appear between zero and a handful of times a day suddenly appearing together in the same Tweets dozens of times, you almost certainly have identified a "subject."

For this to be helpful, the anomalously frequently used words (called hereafter, busywords) must be detected quickly enough that it remains computationally practical to analyse the corresponding window of Tweets for clusterings of the busy words. This is demanding in part because clustering tends to be non-linear in the number of elements concerned. Logically, it's a simple problem. In practice, it is difficult to do in real time.

#### What's Tough About Frequency
Ordinary word frequency computation is hard to do fast enough because of the sheer volume of the firehose.

The relative frequencies of word use change continuously throughout the day, both because human activity changes with the hour and because new subjects come and go.

You can't just rely on frequency of word use over a few seconds, because the variance is too high. Only a tiny fraction of words, even of relatively common words, get used in any given few seconds. 

To manage the variance, you need a rolling window of perhaps many minutes to an hour's duration, with relative frequency computed at the time-granularity you require. This includes comparing the latest relative frequencies to the previous relative frequencies to determine which words are suddenly anomalously busy. 

And herein is the rub. Tweets arrive at a rate of thousands per second, and each tweet uses ten to thirty words. In just ten seconds you'd be clustering over fifty thousand Tweets and perhaps a million words. It's a serious computing challenge.

#### Relative Frequency Turns Out to Be a Red Herring

We tend to think of relative frequency as the core problem, but looked at a different way, the only point of computing relative frequency is to use it to recognize surges in frequency of use. They are two different things. The surging is what we actually care about, not the relative frequency itself.
  
The key to getting past this barrier is a probabilistic heuristic that identifies surges in word frequency without doing global word counting and frequency analysis inline. The core of the heuristic is fast enough that it tends to be limited by waiting for enough Tweets to operate on before it is limited by processing capacity. The non-linearity of the clustering implies the seeming tautology, but the faster it runs, the faster it can run.

You can't get away global frequency computations entirely. You need keep up with background word usage changes caused by time of day, day of week, etc., as well as with ongoing changes due to the emergence of new subjects that people Tweet about. But you can get away with doing them only periodically, and keeping them offline with respect to the main processing pipeline. While busyword and subject analysis must place at intervals of a few seconds, these offline frequency analysis operations can take place at intervals of many minutes, not only orders of magnitude less often, but also outside of the main processing flow.

The heuristic can be fast because it is only sensitive to the leading edge of a surge in usage of a word. Moreover, it is inherently blind to a sustained increase in frequency. Threfore,it automatically forgets words that surge only briefly in usage, as well as words that surge and then stay at an elevated level unless/until that word again surges in relative frequency.

The heuristic is extremely fast--on an adequate server, it can be considerably faster than the firehose rate. Running on a twelve-year old iMac, the heuristic processes about two and a half Decahoses, including feeding the Tweets. A modern server with many hardware cores could run at multiple firehose speeds.  
 

# Input and Output

The input available for development purposes is two weeks of the decahose (about 500 Tweets/second.) It consists of JSON-formatted Tweets in files that have the file order encoded in the filenames as Unix start and end times. The files are about five minutes of Tweets at about 500 Tweets/second. The original files are unpacked into uncompressed CSV files.  

In production the input could be either files (for historical processing) or the actual live Tweet stream (for real time processing).  

Using files is not a "cheat" because any system would typicaly receive Tweets and feed them somehow. The actual development feed is from a process running on the same meachine and sending the Tweets via RabbitMQ.

The non-graphical output is a series of clustered sets of Tweets that are about new subjects. There is not yet a graphical front end.
 

# The Approach

The fundamental principle is that "subjects" are best identified by joint use of shared sets of words that are suddenly appearing with anomalous frequency.

Word usage famously has a Zipf distribution, which means that 99% of words are very rarely used. "Rarely" in this context typically means perhaps zero, once, or a few times a day. A word that is suddenly appearing unusually frequently we call a "busyword."

Because most words are very rare, not even one-in-a-million in a text stream, if two or three busy words suddenly start to appear together in Tweets, it is a strong indicator of either a new subject or of an existing subject that is seeing sudden growth.
 
A word being busy is not about its absolute frequency. Rather, it is about an abrupt
increase, i.e., the first derivative, of the frequency of use, which might suddenly go from zero instances per hour, to several instances in the last few seconds or minutes. 

Counterintuitively, when it comes to identifying subjects, it is the rare, offbeat words that tend to drag the super-common names along with them, rather than the other way around. Common words, like "the", "a", "Trump", "Beyonce", or "Swift" contain almost no information because they appear in thousands of ongoing conversations. It is proper names of people and places, economic, scientific, geographic, or political terms, suddenly popping up together, that usually signify that people are saying something new. 

If a hot subject comes along, i.e. people are tweeting and retweeting text that contains some subset of three or four busy words, the subject will tend to be forgotten after a while because the anomalous rates of use that caused them to be marked as busy words slowly either becomes the new norm, or decline to their old obscurity. It is a truism that frequency can't increase indefinitely--it can't even increase rapidly for very long. 

But doesn't this imply that a subject will only be visible for a short time? Yes, even if it is hugely important. But if a subject is significant, it will usually keep getting augmented with other unusual words used in subsequent Tweets. These tend to keep the subject fresh far beyond the period of novelty of the original busy words. This is in keeping with our intuition. As a conversation evolves, the original subject words hang around, but they are elaborated with language that reflects evolving viewpoints and new commenters.

The key to making it work is that the sliding window on the stream of Tweets has to be small, e.g., typically in the range of thousands of Tweets. With too wide a window the granularity of the view erodes and the computational cost increases disproportionately.  

So sum up, the algorithm 
- Detects the leading edge of surges in word usage.
- Finds the Tweets in the current window that use a proper subset of these words
- Discards all the other Tweets
- Clusters the remaining Tweets by the busy words they use. Clusterings need not be unique or deterministic.

We optionally do one more thing. We can also cluster the clusters over the preceding multiple batches of processing to discover the persistence of subjects.

 
# The Heuristic
This section explains the core heuristic for finding the busywords.

## Word Frequency Computations

As explained above, to achieve adequate speed, the global frequency calculations must not be in the main processing path. 
 
We use a periodic offline computation of global word frequency to create a set of filters that are used to partition the flow of words into F frequency classes, with the most frequently used words in the first class and the least frequent words in the last class. 

The classes have very different numbers of words, but they each represent the same amount of word usage. (The filters are just hashed sets. Partitioning the stream is fast because of the extreme Ziph distribuition. Most words are in the first class, and almost all words are in the first few.) A large enough F value is chosen that there is not much variation in frequency among the words in a given class. An F between 7 and 28 is a reasonable choice.

This frequency partitioning filters are computed over a substantial sliding window of Tweets, perhaps several hundred thousand to millions. This is necessary to lower the variance in frequency. To see why, imagine if the window were really small, say 1000 Tweets. Even common words might not show up at all, leading to many spurious busywords when they show up in the Tweet stream.

Note that the size of the window and the frequency of the re-computation of the filters are two different things. The frequency filters might be recomputed many times over the duration of the window of Tweets. 

Note also that the frequency of the cycles of computing busywords and clustering are independent of both. The big window might be 500k Tweets, but the filters might be recomputed every 100k Tweets, while the busyword/clustering might be computed every 15k Tweets.
 
New words come along all the time, even after weeks of the firehose. A novel word doesn't match to any frequency class the first time it is seen. It won't show up in the frequency filters until they are recomputed.  This is not a problem, however. The window is large, so any incoming word that doesn't match to a frequency class is almost certainly very rare and can therefore default to being treated as being in the least frequent class which, courtesy of Ziph's law, which will have only one occurrence of each word. When the next update to the frequency class filters is occurs, the novel word will no longer be an unknown.
 

## Receiving Tweets
The main routine reads inbound Tweets from RabbitMQ in CSV format. This detail is a convenience because the Tweet data is canned. They could be received in any format, including the native JSON. Either way, as each new Tweet is received, the main routine parses it and converts it to Tweet structs in memory.

The receiving phase:

- Maintains an in-memory sliding window of the latest Tweets. This is not to be confused with the window used for frequency analysis. The Tweet window is much shorter and holds the entire Tweet struct for every Tweet that contributed to the last several cycles of busy-word processing. This will typically be less than a minute of Tweets. They are continually aged out and discarded to limit the size in memory.

- Extracts the words from each incoming Tweet and normalizes them. This include unifying the case, dealing with diacritics and such, discarding junk words, etc. The result is a stream of meaningful tokens.

- Puts the tokens on a queue for the off-line frequency calculations to use for the periodic asynchronous frequency calculations.

- Obtains the frequency class for each token using the frequency class filters.

- Looks up the "three part key" (3pk) for each token. If a 3pk doesn't yet exist, it computes one for the token and inserts it into the lookup table.

- Puts the 3pk on the busyword queue for the token's frequency class.

When one "batch" (perhaps 10 or 20k) of incoming Tweets has been processed, the main processor puts a special 3pk that cannot correspond to any real token, on each queue to signal to the busyword processors that the latest batch is complete. The busyword processors all get the signal at the same place in the Tweet stream, which keeps them synchonized.

With the handing off of the 3pk's for the tokens in the current Tweet to the busy-word processor queues, the main processing loop is at its end, and it starts over with the next Tweet. 

The queues for the frequency counting thread and the F busy-word processing threads provide elasticity, so no synchronization with the main loop is necessary other than what is implicit in the thread-safe queues. 
 
 
## The 3pK's
We haven't said exactly what a 3pk is.  A 3pk is just an ordered triple of hashes modulo C for some token or word.  The hashes are each parameterized with a different value, so it is really just three pseudo-random numbers in a defined range.  This size of this range is a global parameter, but it is ordinarily somewhere around a thousand or so for reasons that will be made clear later.

As mentioned above, a global mapping of 3pk's to tokens, and tokens to 3pk's is maintained. If a 3pk corresponds to an existing token, it will always exist in this mapping.

If the 3pk hash range is 1000, that gives a 3pk a logical space of a billion different pseudo-random triples. There are only at most few million distinct words in the global computation at any one time, so there is a possibility of a collision for any given token. However, as we will see later, collisions are neither frequent nor very harmful.  

The range can be increased to reduce this probablility farther, but there are computational costs to a larger range. How to compute the optimal range size is an open question.

## The Busyword Processor Threads
 
Each of the F busyword processors works the same way. 

The purpose of the frequency partitioning is to ensure that the background frequency of the tokens received by a busyword processor are appoximately equal.

The hashes from each 3pk are used to index into three corresponding arrays of counters of size C. (The array size is the same as the 3pk range.) One counter in each of the three arrays gets bumped for each incoming 3pk.

Because the 3pk values are pseudo random, if every word had the same probability, the counts would have a Gaussian distribution.  

However, we are assuming that some of the words assigned to a given frequency class are anomalously busy (after all, that is the point of the exercise.) Therefore, some of the counters will have exceptionally high counts.  For instance, if you have 10,000 Tweets in a batch, with an average of 10 tokens in each Tweet text, that's 100,000 tokens spread over, say, 24 busyword processors. This means each processor would get about 4,166 tokens, and the counter values would average about 4.166 when the signal 3pk is detected. (We use {-1, -1, -1}) at which point the processor suspends reading the queue and processes the batch.

- A Z-score is computed for each counter as if the distribution were Gaussian.  
	- A Z-score is a normalized standard deviation that gives the improbability of any given sample value's deviation from the mean due to randomn variability. 
	- Technically, our counters are not Gaussian
		- Firstly because the values are discrete, not continuous
		- Secondly, because while the frequency class is constructed so that all the words have the same frequency, the busywords are by definition non-conforming. 
	- However, Z scores are fairly robust and this is an acceptable technique for isolating anomalies.  
 	- The average Z is 0, by definition (Z can be positive or negative), and the higher the Z, the less likely that a large value is just random variation. Any statistics book will give a fuller explanation.  
- This is useful for detecting anomalies because
	- A Z score beyond 4.0 be expected by random chance no more than about once per cycle if all words in the frequency class were arriving at their normal background rate. A Z of 5.0 or greater would almost never occur.
	- By choosing our Z cut-off, we can adjust how freakishly un-random a count has to be for us to treat it as "busy."

One of the configuration parameters is an F-length array of Z-values, one for each frequency class (as some classes tend to have more natural volatility than others.)

Here is where the magic happens:

- The sets of indexes of counters with freakishly high Z scores are collected for each of the three counter arrays. Note that we chose Z to keep cardinality of the set quite small relative to the number of counters. A Z that limits the average number of high values to under ten would be reasonable value.

- The Cartesian product of the three index sets is computed. This gives you a large set of three-part keys, some of which should correspond to actual busy words, and the others (the vast majority) are just spurious junk. The lambs are separated from the goats by checking for whether each 3pk's corresponds to a value that exists in the global mapping.

- The 3pk's that exist give you the busy words for the current window. Note that this is a somewhat leaky filter. 
	- Some randomly generated 3pk's will usually correspond to real tokens just by random chance because with a range of 1000, there are only a billion possible keys. 
	- Collisons happen. Occasionally, more than one token could map to a 3pk.

## Processing the Batch of Busywords.

Each of the F frequency classes puts its set of busyword on a queue to an analysis thread. The results are kept in synch by a concurrency "barrier" to keep the F batches of busywords together.

When the analysis thread gets all F batches of busywords, it does the clustering as follows.

- It filters a configured range of the set of recent Tweets for Tweets that contain at least M busywords. M might typically be 2, 3, or 4, but any number can be used.

- With proper configuration, this set of Tweets is quite small compared to the total number of Tweets upon which the batch of busywords was computed. This makes sense because most Tweet subjects at any one moment are not new.

- A graph clustering algorithm is applied to the set of Tweets that contain at least the minimum number of busywords. The algorithm finds groups of Tweets that have the most joint similarity in the sets of busywords they contain. 

- This is a non-deterministic process because clusters can overlap a little. For instance, one group might be characterized by five busywords, and another by four, but they happen to share one busyword. However, despite that overlap, they the two groups are identifiably different.  These properties can be tuned to suit.

## The Output
The is text, for the moment.  The clusters are computed and grouped in clusters that are printed out as an ASCII tree. 

A more complete text output should print the essential information needed for display to standard out or to a named file.
This would be include:
- The clusters, in a computationally friendly format.
- The busy word information, in a computationally friendly format.
- Both should include admin information like the run time, the time as given in the Tweets (this will be easy to obtain down to a minute or so,) etc.
- The lines can be commingled so long as they have an identifying leading field, e.g, CLUSTER, BUSYWORD, ADMIN, ETC.
- We would need to print all the diagnostics and other screen junk to stderr.


A graphical front end is in the offing.
 

# Speed of Computation and Significant Events

Ideally, we want to be able to process at a multiple of the firehose rate. These results of testing referred to here are on a 14-year old iMac. 
 
The Tweet sample was recorded from two weeks of a Decahose. However, the Tweets can be consumed at any speed. 

The program sustains a thoughput of about 1100 Tweets/second. It is convenient to run it with filtering for English only to make the output more comprehensible. It processes at the rate of about 2.2 decahoses.  

The Tweets are read from Rabbit MQ and Tweets excluded because of language are discarded after parsing. This means you don't get much difference in run speed due to filtering or not filtering for language. 

Each CSV file is about five minutes of the decahose, i.e., about 30,000 Tweets per file, or 33 files per million Tweets, or about 2.7 logical hours of the decahose. We are actually processing that million Tweets in about 1.2 hours.

This is useful to know if you want to start processing at a given date/time.  You need to start at a point sufficiently many files earlier than your nominal start date in order to have the system primed. 

If you are starting and stopping frequently, the system can optionally read in the existing frequency data, for an instant start.  Beware that if you are starting at some random point, the saved frequency data will be wrong to an unpredictable degree. This means you need to either start fresh or disregard the results until you have run for long enough to completely update the background statistics.

You can use the utilities mentioned below to find the date/time you want to start at a specific place in the two-week interval.

### Persistence of Subjects

In addition to computing the new subjects, the system can give information about how long those new subjects have existed in terms of the number of batches. This is given in the form of a list of the previous N batches that also match the new subject.

Persistence of subjects is a major consumer of CPU, bigger than the cost as the clustering itself.  It can be turned on and off, but the effectiveness of turning it off is still to be determined. It's not clear that it affects throughput, which may be bottlenecked elsewhere.
 

# Data Set Start and End Time

We have a little more than two weeks of almost unbroken data. The feed was restarted briefly once or twice over that time span--probably not enough to matter.


## The Data Set

- Starts around 2012-01-28 16:16:46, i.e. quarter after five on New Years Day
- Ends at 2012-02-15 03:22:36, which is 3:22 AM the day after Valentine's day.

The very first hour or so may or may not be fully trustworthy as there may have been some starts and stops. This can be investigated using the file/time utility. It doesn't seem to matter.

## News of Whitney Houston's death Seems to Start Around Here.
gnip.csv_1329008058666_1329008358666.csv:168498929640550400, Feb 12 00:57:30 

This is toward the end of the 2-week+ period covered by the feed.


### Super Bowl
Note the superbowl is also a great place to see real conversations starting up. It occurs on Feb 5 2012.  


# Running the Program and Utilities

In addition to the main program, there are several programs to parse the original JSON into CVS, run RabbitMQ, send the data, and various utilities for data conversion and analysis. This section tells how to run them all.

## Parsing JSON files to CSV

### Build and Run the Golang JSON->CSV Parser (Obsolete)

The Golang JSON->CSV parser is not currently in use. The following is for future reference only.

cd /Users/petercoates/python-work/cursor-twitter
go build -o parser/parser parser/parser.go
./parser/parser -inputdir ../twits/msg_input/ -outputdir ../twits/test_output

### The Python JSON->CSV Parser
This is the JSON to CSV parser you should use.

This assumes you have a directory full of GZIP'ed JSON and an empty target directory for the CSV.  The program will unpack the zips into /tmp so as not to risk corrupting the original data.

Do the following. (You may need to adjust the directory names.)

- cd to cursor-twitter.
 
- python3 parser/parser.py ../twits/msg_output_language ../twits/msg_output_3

The parser can be run with an optional --num-workers flag the gives the number of processing threads to be applied to the parsing. The speedup is roughly proportional to this value up to the number of hardware cores on your machine (which on this antique iMac is four.)

- python parser.py input_dir output_dir  

## Go Utility to do Language Identification on CSV files

The lang: field in the original Tweets is very unreliable. This utility post-processes the CSV files do language detection via NLP. (Twitter uses your environment settings.)

This is not in the Python parser because Python language processing is agonizingly slow. Go is at least 200 time faster than doing it in Python.  

It also seems to do a noticeably better job classifying than the Python library.

### Build it
- make build-language-detector

or

- go build -o language_detector language_detector.go

### Run It
Use -help for details. There are flags to turn progress off and to set the number of threads working. The default of 8 threads is fine on a four core machine (like mine.)

With -progress,  the time to run the test set was 2m 40s,  and it reports 16,156 lines/second.

Without -progress the time to run the test was x and it reports 17,538 lines/second.

Note, you can stop and restart the program and it will ignore the input for which there are already files in the output. You may want to remove the newest 8 files in the taget directory because they may be incomplete. (8 files is nothing-there are 5000+ files in total.)

- ./language_detector -input ../twits/test_language_detect -output ../twits/test_language_detect_out 


## Test program for parsed data
This program reads CSV files to ensure that we can create Tweets from them. Most users will not need this.

- go run tests/csv_tweet_parse_test.go <path_to_your_csv_file>
  
## Building and Running The Main Program
    
The codebase is in ~/python-work/cursor-twitter. 

Data that it writes in order to persist data structures, etc. is configured to be in ~/python-work/data, i.e., at the same level as the project root. You can change this in config/config.yaml.  Using relative paths is treacherous.
 

## Starting RabbitMQ

RabbitMQ feeds Tweets in CSV form to the processor. The service normally running  as a daemon which in theory can be started as follows.

brew services start rabbitmq  

If this doesn't go well for you, you can just ask Cursor to start rabbit in Docker or manage it yourself with the following commands.
 
- docker start rabbitmq
- docker stop rabbitmq
- docker restart rabbitmq
- docker ps | grep rabbit
- docker logs rabbitmq

There was drama aound getting this right. Rabbit was set up wrong so that it just silently dropped messages that weren't instantly picked up.  It is now configured right but it can be monitored on the web app.

- http://localhost:15672/#/

The username and passwords are guest,guest.
 
## Sending Tweets

Running the actual software assumes you have a directory of CSV files of Tweets, in this case I'm using ../twits/test_language_detect_out. These have been language-processed so I can demo with just English. The choice in the main processor of all, v en v <anything else> is set in config.yaml.

The sender program makes a note in the data/sender of what the last file of CSV it sent was, and picks up with that when restarted. If the file is deleted or emptied, it starts at the beginning of whatever directory you point it at. The file (feel free to delete) is:

- data/sender/sender_status.txt

There is code in the sender that pauses if the number of Tweets in MQ becomes excessive, but it seems to be redundant now, and the ACK mechanism seems to throttle on its own. The number of Tweets in MQ never seems to get out of two digits.

This is all you really need to send CSV Tweets.

- start RabbitMQ (see above)

- cd to the cursor-twitter directory

- python ./send_csv_to_mq.py ../twits/msg_output
 
 The following are probably obsolete but they exist.
 --max-queue-depth 10000 
 --pause-duration 1.0
--max-queue-depth 5000 --pause-duration 2.0
 
##  Build and Run the Main Program
 
Note, this assumes you are in the project root. Sometimes cursor wants to run it from src which is not right. It is better to build yourself and run from the root directory. Don't let cursor run it at all.

- cd /Users/petercoates/python-work/cursor-twitter

- go build -o main src/main.go 

- ./main  -config ./config/config.yaml -print-tweets=false 

### Some Useful Flags
- -profile  causes it to produce profiler output (see section on profiling). Profiling slows it down significantly but it doesn't matter because the results are relative. Instructions for using the resulting file are found elswhere in this document.

- -load-state  Causes it to load the latest state from disk. Without this it takes some minutes to build up enought tweets to start computing busy-words.  Use this is you are starting and stopping for development and testing. Huge time saver!

### Shutting down the main

Control-C will send a signal. If the process of writing persisted data to disk is underway it will finish before it shuts down. Otherwise it shuts down immediately.

## The Analysis Program

This is a utility to get a picture of how the universe of words grows with the number of Tweets processed. You need to point it at a large directory of CSV files. It's relatively fast at about 70k Tweets per second.

See elsewhere for results. Spoiler--new words are frequent and don't decline much in frequency over the full data set of more than two weeks.

cd to cursor-twitter
go build -o analyze_tokens analyze_tokens.go 
./analyze_tokens -input ../twits/msg_output

## Find the CSV File For a Given Date

There are more than two weeks of decahose in 5000+ files. If you process Tweets starting at some given point in the multi-week interval available starting at January 1, 2012 and running to approximately January 15, 2012, you can find the file to start with by using this utility.

You can put the file name in the data/sender directory to tell the sender where to start.

Note that it takes an argument, N, which is the number of CSV files prior to the one you want. This is because in most cases, you will have to fire it up from scratch in order to have it primed when it gets to the date you seek.  The number of files is a function of how many tokens you specify in config. I've been using 2,000,000, which is about a quarter of a million tweets, divided by 30k tweets in a file should be N=eight or nine files.


Build it:

make build-find-csv

Run it:
./find_csv_file -dir /path/to/csv/files -datetime "2012-02-14 19:35:55" -n 3


A handy way to check it:
The following will print the first few lines of the file.

 ./find_csv_file -dir ../twits/msg_output_3  -datetime "2012-02-14 19:35:55" -n 5 | xargs -I {} head ../twits/msg_output_3/{}


## Print Out the Time Intervals For the CSV files

The time interval covered by a given CSV file varies a little, but except in a few spots, like the beginning, it's about five minutes. This program will tell you the times for all the files.

./csv_file_mapping -dir /data/csv > file_mapping.csv

./csv_file_mapping -dir /data/csv | head -10

./csv_file_mapping -dir /data/csv | grep "2012-02-14"


## Frequency Analyzer

This utility gives rank, token counts, relative frequency, and cumulative frequency for all tokens in the CSV's found in the given directory. This utility will differentiate by language. Be sure you ran the language utility, or the results for any given language will be nonsense.

Note that language analysis is decent, but not flawless, so words from Tweets in any specific language will not be exclusively of that language. In addition, of course, many are nonsense words, names, screen names, etc., which aren't necessarily part of any particular language.

The output is CSV of the form {rank, token, count, frequency, cumulative frequency}

The output by default goes to global_frequency.csv but you can set it to whatever file name you want.

This program is quite fast--it will do all 5,300 files in just a few minutes.

### Build It
make build-token-frequency

### Examples
./token_frequency_analyzer -input /data/csv
./token_frequency_analyzer -input /data/csv -lang en
./token_frequency_analyzer -input /data/csv -lang en -output english_frequency.csv


### Distinct Tokens for English (en)
This is interesting because is shows how extremely Z  

- 11,522,129 distinct tokens appear in "en" Tweets.  
- The top 220 tokens (which are almost all words) account for half of all usage.
- The top 3,572 words account for 80% of all usage.
- The top 13,690 words account for 90% of all usage.
- The top 615,473 words account for 99% of all usage.
- The other 95%, 10,906,656, account for only 1% of all usage.
  

It's mostly the relatively rare words we care about because they carry the most meaning when they surge in usage.
 
## Tests
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

# Notes On Things That Have Been Checked/Explored

## Seemingly Excessive Token Rejection
We were getting huge percentages of rejection of tokens because of token length and/or tokens being on the ingore list. 30% and 40% respectively.
 
Those numbers were not supurious. I turned that functionality off and they were as expected. It means both are very effective and doing their job. You definitely want min length = 3

## Effect if Computing Cluster Persistence
### Checking at all
I went from checking over six batches to checking over 1 batch. Not a huge effect. Maybe 100/second.

### Checking all words v checking busy words
This aspect has NOT been reviewed


## Running with the subject persistence turned off 
This is a big user of CPU but turning it off did not cause a huge speed increase.

## Running with config/token_filters.txt empty

Emptying out the test_filters.txt file did not result in a noticable speedup but it did reduce the number rejected on that basis to zero. So the 40% rejection rate when that file is active is real.

## Setting minimum token length to 1 
This (as it should) resulted in the rejections for minimum token length to go from 35% to 0%. So that parameter is functioning.

## Is token splitting on periods, commas, and dashes happening
It was not happening.  The token cleanup in Go now does this. Note that we can optionally split or not split on apostrophes. The default is to not split so that O'Brian and M'kele M'beme can continue to be tokens.

## Overhaul of ASCII Output and Reduction of Logs

Logging has been majorly overhauled with logging almost all going to the designated log file and almost none to the screen except some startup info going to stderr.

The program output is now all in JSON format sent to stdout. It can be configured to be more complete, for machines, or more readable, for humans. 

The new policy is to put only critical messages on the screeen and to write them to stderr so we can preserver stdout for proper output.
    
## Annoying Nonsense
Consider a file of phrases that would get a cluster or tweet dropped.
"The awkward moment"
"That awkward moment"
"Do you want more Followers" 

There is now such a file and it seems pretty effective. There aren't that many such phrases.

# TTD and Direction

## Major: Skip Messaging for Historical Data
To process historical data (as opposed to simulating real-time data as we're doing in this demo) you could replace RabbitMQ with simply reading standard in. 
- Replace the feeder with a program that just cats the lines in the CSV to standard out
- Replace reading Rabbit msg by msg with a routine that simply reads standard in


## Minor: Excessive ????????? Strings in Texts
We have added code to drop the entire Tweet if there are long strings of question marks. This is usually an artifact of Tweets in some language we're not able to handle. Either way, it should be gone now.   But it seems not to be! I see some such tweets in the output.
  
 
## Ongoing  Items
- Comments Key areas need to be commented to keep out don't touch, etc.

- Testing has been neglected. Make tests around everything.
 
## Major: Improving Busy Word Detection Quality with Computing Redundantly

This would be a significant effort.  Interesting idea, but it's not 100% clear that it's worth doing. We need some investigation.

Consider that dual sets of pipelines, or even three sets, each with different hash functions, could do a much more accurate job of filtering out the true busy words for a given set of parameters.  
 
It is not clear how big an impact this would have on performance. All the BW processors are doing is counting and periodically computing Z on a thousand values. There is a tiny bit more work in the analysis to take only the words that appear in the required number of sets. It doesn't really affect the analysis phase that follows, and it's just a little more work for the main to put the tokens on more queues than before.

Risk. If multiplying the work in the busyword processors made them in aggregate slower than the combined main pipeline and the clustering, it would cause the queues to grow without bound and crash the program. You'd need to detect the problem and throttle the reads if this is a problem. Actually, this should probably be done anyway! Who knows if some combination of config parameters could cause this to happen.
 
## Major: 3pk Collisions
Over two weeks of data there are about 11 million distinct tokens in EN Tweets. (Note, this figure may be significantly higher than the effective number because we filter 2/3 or so of them out as being useless for busywords) Unfiltered would means there is about a 1/90 chance of a given token colliding with another token's 3pk. Given the birthday paradox, you're probably getting fake busy words in most cycles.

This could be greatly mitigated if we only kept 3pk's for the tokens that have non-zero counts, because most of the words will appear once and vanish without a trace. This would keep the actual number of mapped tokens way down.

This could be done in a thread-safe efficient way if:
- The FCT put any token the count of which hits zero on a queue that is read by the main thread. 
  - Minor point: The FCT would have to remove the entry from the counts, not just set the count to zero!

  - The main checks this queue whenever it is convenient. E.g. every thousand or ten thousand Tweets.

- This means that the probablilty of a given token colliding would be more like one divided by the cardinality of the global counter map, which is typically in the 100's of thousands, i.e. about 1% of the universe of words.

- This does not seem like a hugely expensive item. 
  - You'd pay a little bit for the extra insertions and deletions from the map. - The map is read asynchronously by the busyword processors so thread safety is an issue, and thefore contention for the resource. But a few thousand deletions from a map is not a huge item.
 

Note, it would be unusual, but in theory the busyword processors could require a token for a 3pk that has been deleted since the cycle started. This has to be guarded against.

Be careful about threading issues.

## Performance
We started to get a significant slow-down at some point. It's about 1100/second now, and it used to be in the mid-2000's. This could be consequence of some real piece of functionality, or it could be something stupid. 

Went over this with the profiler and chased down a lot of stuff, but it's still slower. Cursor's opinion is that it is probably primarily in taking tweets from RabbitMQ. I doubt it, because we've taken them off at up to 3000/second in the past when we weren't doing anything with them.  

The bottleneck seems to be on the main processing line because the FCT is not in the path of the Tweets, and the busyword and analysis portion won't block the main line processing because it is insulated behind queues.

We did an extensive review of concurrency protection in the processing pipeline and found a number of issues, but I'm not sure if we ever got to every item.
   

## Ongoing 
- Comments Key areas need to be commented to keep out don't touch, etc.

- Clean up the excessive logging and print outs

- Make tests around everything.

- More fully populating the token_filters.txt file. Check the busywords in the logs and see what else jumps out.
 

## Major: Clustering Across Batches

Note, this has now been implemented but may not be adequately tuned up yet. See above relating to this under speedups. 

## Major: A Graphical Front End
This is a big one!  Not sure how to go about it with Cursor. I wrote the graphics by hand in Java last time!

A busy-word fade-out like the bubbles would be great.
- The size proportional to the number of Tweets the busy word is in. Or perhaps logarithmically proportional.
- When the BW was last seen, fading off the left
- Vertical axis is the frequency class


## Major: Clustering Clusters
Could the same graph algorithm be applied to the already clustered clusters?   
It seems like it would be good to see these things grouped more hierarchically.
You often see clusters that seem to be almost the same.
If the clustering allowed clusters that are unrelated to other clusters and clusters that are composed of sub clusters, it would be great.

## Consider Stripping K-Means Out Entirely
It doesn't do any harm, but we're never going to use it and it could be confusing.

 
  
## Understand the effect of window size and batch size better.
Are the numbers below correct.
 
 
## Jacquard Similarity in Clusters
Jacquard similarity for the clustering seems to be applied to all the tokens in the Tweets. 
  - Is that true, and if so, is it correct? 
  - Maybe it should be only on the busywords. 
  - Jacquard similarity might not be important. Maybe just the raw occurrences?
   
# Miscelaneous Details

## The Big Token Window.

People don't Tweet the same things at breakfast on Monday that they do at 1:00 AM on Saturday.  Also, the world turns, and while New Yorkers are getting up in the morning, people in Bejing are out for the evening.

Also, surges in usage continually change the current frequencies.

Because most words are extremely rare, and words beyond the rank of a few thousand it still takes quite a lot of tweets to get even a rough estimate of frequency. Consider that
- "Dog" is about the 1000th most common word, yet only 1/10,000 of words will be dog. 
- "Mirror" is the 3000th most common word, and only 3 words out of 100,000 will be mirror.  
- "Edge" is the 5000th most common word, and it appears only once in 100,000 words

Therefore, you need a lot of words (millions) to get even a reasonably accurate estimate of a word's background frequency. The decahose is 500 Tweets/second or about 5,000 words/second, which is about 3.3 minutes of data. So a window of five million words is about 16.6 minutes.

Five million is a reasonable window size because it's easily fine enough to keep up with the time of day, and not so coarse that it takes a long time to age surges in frequency out.

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
  - 44,298,119 distinct tokens in 137 million tweets, 
  - x distinct tokens in 599 million tweets

- 599 million Tweets equals approximately six billion tokens.  
 
- Volume of decahose = 500/sec

- Volume of firehose = 5000/sec

- 500/sec = 30k Tweets/min = 450,000 Tweets/15min  = 1.8m Tweets/hour in nominal Tweet time

- There are an average of 10 words/Tweet = 4,400,000 words/15 min or 18m words/hour

## Our Processing Speed
- The sender can send about 3000/second with ACKs. Which you need, or it will simply drop msgs.

- We process about 1200/second, or 2.4 Decahoses.

- The Tweets get ACKed, so rabbitMQ manages to never let the queue get very long--it seems to never get up to 50.
  
- Google says a modern Mac could probably do about 10x as fast as this antique iMac. If true, that's a couple of firehoses.  This is important, because one of the purposes of this is to make it possible to handle historical data. 
  - I'm skeptical that better hardware would get a 10x bump. 
  - The processor on this old machine is actually really fast (4 GHZ), despite being 12 years old. 
  - Any speedups would be because of faster SSD storage, more cores, and improved instruction set. Most likely, the number of cores would be the overwhelming factor. 
  - So a 16 core machine would probably just about do the firehose. 
    - Possibly more if you moved the feeder to a different box.

- Note that for historical data it will scale in direct proportion to the number of machines so you can do bulk processing essentially as fast as you are willing to pay for.

- We have about 350 hours of decahose. At 2.5 decahoses, we could process the entire data set on this machine in less than a week.
 