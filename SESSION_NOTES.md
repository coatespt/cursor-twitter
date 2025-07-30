# Session Notes

This document covers
- Loud warnings to Cursor not to mess with the code on its own
- The purpose of the project (two fold)
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
This project has both a narrow goal and a broader goal.

## The Narrow Goal: News From the X/Twitter firehose in real time.

The weakness of Twitter, X, and other micro-blogging platforms is the difficulty of finding out what is there. In practice, this problem is mostly solved by punting it to the user.  It's up to the writer to markup posts with hashtags, and up to the receiver to find people, hashtags, etc. to subscribe to. There's no good way to find out what's new unless you already know about it, which means it's not new.

The project is a way to read the Twitter firehose and find new subjects appearing in the Tweet stream in near real time. By "near real time" we mean in a span of time roughly comparable to how long it might take to read a Tweet amd compose a response. 

Ideally, significant new subjects are identified within seconds or tens of seconds of their surging into the stream. 

Watching the flow can be entertaining, but knowing what's happening minutes or even hours before it breaks on conventional new channels can have real cash value.

## Broader Goal: History
X and similar platforms are a potentially valuable historical record.  The trouble is, it is all but useless for anything but confirmation of what you already know, because it's so overwhelming to analyse.  

Being able to analyse the stream at a speed that is a multiple of the real rate opens the history of Tweets to a granularity of analysis that is unprecedented. 

Being able to see the subjects arise and ramify minute by minute makes it possible to understand at flow of information in the crowd during historical events like the BLM movement, the Arab Spring, the invasion of Ukraine. It can also be useful in tracking the spread of political and financial rumors, conspiracy theories, at a sub-minute level of granularity.
 
# Statistics and Semantics

A driving insight is that it would be essentially useless to 
analyse all the subjects that are active at any one time because this number is in the tens of thousands, depending upon how you define "subject." 

By any standard, it is orders of magnitude too much data to be useful. However, if one settles for only seeing new subjects as they arrive:
- The amount of data for a human to be exposed to is manageable
- The subjects arrive newest, i.e., most important, first
- As time passes, you asymptotically approach getting **all** subjects.
- Focussing on newness at the level of seconds, you can see a new subject ramify into sub-subjects in near real time. You can watch the hive-mind communicate with itself.

While X/Twitter is a firehose of data, fortunately, new subjects tend to arrive at a manageable rate. Depending upon exactly how you parameterize the definition of "subject," new subjects arrive perhaps every few seconds. With proper tuning, you get something with about the information density of the Times Square News Ticker.

To find thes subjects by characterizing the semantics of thousands of Tweets per second, and then grouping them together based upon subject similarity would be a daunting task computationally. It's not even clear how to do it, as it's not just a matter of understanding Tweet-by-Tweet. You have to cluster them. Certainly it would be extremely difficult and resource intensive to do in real time. 

Fortunately, however, groups of meaningful words used together are an excellent proxy for subjects, and semantics can be entirely ignored. 

### Pure Statistics
 
Most frequency-based approaches to this problem look for the most fequently used words.

However, these words tend not to be very useful for finding new subjects in real time.  At any given moment, literally thousands of subjects include "P Diddy", "Donald Trump", or "Superbowl."

The words that characterize new subjects are more often words that are typically rare, perhaps even novel, but which are suddenly being used anomalously frequently. When you find several words that normally appear say, zero to a handful of times a day, suddenly appearing together in dozens of Tweets, you almost certainly have identified a "subject."

Words have a Zipf distribution, which means that overwhelming majority of words are very rarely used. By a wide margin, the most common number of times per day for a random word to be used is zero. The next most common number of times is one, again by a wide margin. The distribution tail is so long that the most common 250 words in a decahose of English Tweets are used more than the next 11,000,000 words combined. 

For this to be helpful, the anomalously frequently used words (called hereafter, busy words) must be detected quickly enough that it remains computationally practical to analyse the corresponding window of Tweets for clusterings of the busy words. This is demanding because the clustering operation is non-linear in the number of Tweets concerned. Logically, it's a simple problem. In practice, it is difficult to do in real time.

#### What's Tough About Frequency
Ordinary word frequency computation is hard to do fast enough, because of the sheer volume of the firehose. You need the relative frequency of words from a set of Tweets in a rolling window of perhaps an hour's duration computed every couple of seconds or so. This includes comparing the latest frequencies to the previous frequencies to determine which words are suddenly anomalously busy. At fifty to one hundred thousand tokens per second over a domain of millions of words, it's a lot of crunching to do every two seconds.

#### Frequency Is a Red Herring
We tend to think of relative frequency as the core problem, but looked at a different way, the only point of computing relative frequency is to use it to recognize surges in frequency of use. They are two different things. The surging is what we actually care about, not the relative frequency itself.
  
The key to getting past this barrier is a probabilistic heuristic that identifies surges in word frequency without doing global word counting and frequency analysis inline. The core of the heuristic tends to be limited by the flow of Tweets to operate on before it is limited by its own CPU demands.

A global frequency computation is done periodically offline in order to keep up with background word usage changes caused by time of day, day of week, etc., as well as with ongoing changes due to the emergence of new subjects that people Tweet about. This computation is not in the main processing pipeline, and does not need to take place with a frequency similar to the granularity of the subject analysis. 
- Subject analysis takes place at intervals of a few seconds
- The offline frequency analysis would typically occur at intervals of many minutes--two orders of magnitude less often.

The heuristic is only sensitive to the leading edge of a surge in usage of a word, and is inherently blind to a sustained increase in frequency. Threfore,it automatically forgets words that surge briefly in usage, as well as words that surge and then stay at an elevated level unless/until that word again surges in relative frequency.

The heuristic is extremely fast--on an adequate server, it is considerably faster than the firehose rate. Running on a twelve-year old iMac, the heuristic processes about three Decahoses, including feeding the Tweets. A modern server could run at multiple firehose speeds.  

Speed is critical, because a main goal of the project is the ability to analyse historical data, which can't be done if you can't even keep up with the present.

# Input and Output

The input used for development purposes is two weeks of the decahose. It consists of JSON-formatted Tweets in files that have the file order encoded in the filenames as Unix start and end times. The files are about five minutes of Tweets at about 500 Tweets/second. The files are GZIP'ed.  

In production the input could be either files (for historical processing) or the actual live Tweet stream (for real time processing). This isn't a "cheat" because any system would have to receive Tweets and feed them. the fact that we are storing them in files is irrelevant. (The actual feed is over RabbitMQ.)

The non-graphical output is a series of clustered sets of Tweets that are about new subjects. There is not yet a graphical front end.
 

# The Approach

The fundamental principle is that "subjects" are best identified by joint use of shared sets of words that are suddenly appearing with anomalous frequency.

Word usage famously has a Zipf distribution, which means that 99% of words are very rarely used. "Rarely" in this context typically means perhaps zero, once, or a few times a day. A word that is suddenly appearing unusually frequently we call a "busy word."

Because most words are very rare, not even one-in-a-million in a text stream, if two or three busy words suddenly start to appear together in Tweets, it's almost certainly an indicator of a new subject or an existing subject that is seeing sudden growth.
 
A word being busy is not about its absolute frequency. It is about an abrupt
increase, i.e., the first derivative, of the frequency of use, often from zero instances in the last hour, to several in the last few seconds or minutes. 

Very common words, like "the", "a", "Trump", "Beyonce", or "Swift" contain almost no information because they appear in thousands of ongoing conversations. It is less common words, such as people and place names, economic, scientific, geographic, or political terms, etc., suddenly popping up together, that signify that people are saying something new. Counterintuitively, when it comes to identifying subjects, it is the rare, offbeat words that tend to drag the super-common names along with them.

If a hot subject comes along, i.e. people are tweeting and retweeting text that contains some subset of three or four busy words, the subject will tend to be forgotten after a while because the anomalous rates of use that caused them to be marked as busy words slowly either becomes the new norm, or decline to their old obscurity. Frequency can't increase indefinitely!  This process typically happens over a span of minutes.

In practice, however, a major subject will keep getting augmented with other unusual but related words used by new Tweeters. These tend to keep the subject fresh far beyond the expiration of the original busy words. This is in keeping with our intuition. As a conversation evolves, the original subject words hang around, but they are elaborated with language that reflects evolving viewpoints and new commenters.

The key to making it works is that the sliding window on the stream of Tweets has to be small, e.g., typically in the range of thousands of Tweets. With too wide a window the granularity of the view erodes and the computational cost increases disproportionately.

So sum up, the algorithm 
- Detects the leading edge of surges in word usage.
- Finds the Tweets in the current window that use a proper subset of these words
- Discards all the other Tweets
- Clusters the remaining Tweets by the busy words they use. Clusterings need not be unique or deterministic.


# The Wrong Ways of Doing It

Most attempts to understand the firehose are based on some kind of frequency calculations, sometimes augmented by semantic analysis, combined with various non-text attributes of Tweets such as location, time, etc.

Any approach centered on frequency calculations is inherently hampered by the need to do those calculations on the universe of words because that universe is large: on the order of millions of distinct words. This is inherently in tension with the need to deal with a relatively brief window on the stream of Tweets because:  

Global frequencies mean you must:

- Count the usage of all words in a current window of time. This window must be fairly large in order to get enough Tweets to get a reliable distribution for all the common words, i.e., to keep the variance down.

- Continually limit the number of Tweets represented in the token counters to a limited size that represents usage typical of the time of day.

- Compute the current relative frequencies implied by the word counts every few seconds.

- Compare the relative frequencies of the current window to those of previous window to compute usage-growth rates for every word over the interval.

- Select for the ones that have excessive growth rates.

- Discard the previous frequency computation, demote the current frequency computation to previous, and start a new set of counts.

You only have at most a few seconds to accomplish this before the set of Tweets that arrive in the time span grows to unwieldy size. It's not just the frequency computation that is the problem--clustering algorithms are inherently non-linear in the size of the set to be clustered.
 

# The Heuristic
This section explains how it works.

## Word Frequency Computations

As explained above, to achieve adequate speed, it is essential not to not put global frequency calculations in the processing path. 
 
We use a periodic offline computation of global word frequency to create filters that are used to partition the flow of words into F frequency classes, with the most frequently used words in the first class and the least frequent words in the last class.

Note that the frequency classes comprise wildly diffent numbers of distinct words, but they each represent approximately the same number of word usages.

This frequency analysis is done outside of the processing of the stream of inbound Tweets and is done much less often, as it covers a larger time span than the seconds required for the heuristic itself. The time span covered by the off-line frequency calculations is sized to provide a background look at what normal word frequencies are at the particular time of day, day of week, etc. It covers a half hour or an hour of Tweets.  The frequency with which the filters are updated is independent of this window size, and indeed is usually much shorter, on the order of every few minutes.
 
New words come along all the time, even after weeks of the firehose. Clearly, between updates to the frequency filters, these novel words remain unknown.  This is not a problem, howerver. The sliding window is large enough (millions of tokens) that any incoming word that doesn't match to a frequency class is necessarily very rare and can therefore be treated as being in the least frequent class which, courtesy of Ziph's law, which will have only one occurrence of each word. Moreover, when the next update to the frequency class filters is made, the novel word will no longer be an unknown.
 

## Receiving Tweets
The main routine reads inbound Tweets in CSV format. This detail is a convenience because the Tweet data is canned. They could be received in any format, including the native JSON. Either way, as each new Tweet is received, the main routine parses it and converts it to Tweet structs in memory.

The receiving phase:

- Maintains an in-memory sliding window of the latest Tweets. This is not to be confused with the word-count window. This window is much shorter and holds the entire Tweet struct for the last several cycles of busy-word processing. 

- A cycle, or batch, is few seconds worth of Tweets (e.g. two to ten seconds) so this window will typically hold less than the most recent minute's worth of Tweets. These Tweets are kept available for the clustering phase that will happen at the end of every busy word processing batch. They are continually aged out and discarded to limit the size in memory.

- Extracts the words from each Tweet and normalizes them, unifying the case, dealing with diacritics and such, discarding junk words, etc.

- Puts the tokens on a queue for the off-line frequency calculations to use. This results in the periodic asynchronous frequency calculations having very little speed impact for the main pipeline.

- Checks a global table to see if a "three part key" (3pk) has been computed for each token. If not, it computes one and sticks it in the lookup table.
 
Busy words are computed separatedly for each of the F frequency classes. Each busyword processor gets the data to work on as follows.

- The main processing loop obtains the 3pk and frequency class for each token using the global table and the frequency class filters.

- The 3pk is handed off to the corresponding busyword queue.

When one "batch" of incoming Tweets has been processed, the main processor puts a special 3pk on each queue to signal to the busyword processors that the latest batch is complete. The busyword processors all get the signal at the same place in the Tweet stream, which keeps them synchonized.

With the handing off of the 3pk's for the current Tweet to the busy-word processor queues, the main processing loop is at its end, and it starts over with the next Tweet. 

The queues for the frequency counting thread and the busy-word processing threads provide elasticity, so the no synchronization with the main loop is necessary other than what is implicit in the thread-safe queues. 
 
 
## The 3pK's
We haven't said exactly what a 3pk is.  A 3pk is an ordered triple of hashes for some token or word.  The hashes are each parameterized with a different value, so it is really just three pseudo-random numbers in a defined range.  This size of this range is a global parameter, but it is ordinarily somewhere around a thousand or so for reasons that will be made clear later.

As mentioned above, a global mapping of 3pk's to tokens, and tokens to 3pk's is maintained. If a 3pk corresponds to an existing token, it will always exist in this mapping.

If the 3pk hash range is 1000, that gives a 3pk a logical space of a billion pseudo-random triples. There are only at most few million distinct words in the global computation at any one time, so there is a possibility of a collision for any given token. However, as we will see later, collisions are not very harmful.  

The range can be increased to reduce this probablility farther, but there are computational costs to a larger range. How to compute the optimal range size is an open question.

## The Busyword Processor Threads
 
Each of the F busyword processors works the same way.

The hashes from each 3pk are used to index into three arrays of counters. (The array size is the same as the 3pk range.) One counter in each of the three arrays gets bumped for each incoming 3pk.

Because the 3pk values are pseudo random, if every word had the same probability, the counts would have a Gaussian distribution.  

However, we are assuming that some are anomalously busy (after all, that is the point of the exercise.) Therefore, some of the counters will have exceptionally high counts.  For instance, if you have 10,000 Tweets in a batch, with an average of 10 tokens in each Tweet text, that's 100,000 tokens spread over, say, 24 busyword processors. This means the counter values would average about 3.3 when the signal 3pk is detected (-1, -1, -1) at which point the processor suspends reading the queue and processes a batch.

- A Z-score is computed for each counter. Random 3pK would have a very predictable Gaussian distribution, and a Z-score is a normalized standard deviation that gives the improbability of any given count's deviation from the mean due to randomness.

- The Z's average to 0, by definition (Z can be positive or negative), and the higher the Z, the less likely the value is just random variation. Any statistics book will give a fuller explanation.  

- Suffice it to say here that scores beyond 4.0 would very rarely occur if all words in the frequency class were arriving at their normal rate. Something like one per cycle. In other words, counters with high Z scores probably did not get their high counts by random chance.

- An arbitrary Z-score is set in configuration. This Z says how high the counter value must be to be considered anomalous.   

Here is where the magic happens:

- The sets of indexes of the counters with freakishly high Z scores are collected for each of the three counter arrays. The cardinality of the set is much smaller than the number of counters.

- The Cartesian product of the three index sets is computed. This gives you a large set of three-part keys, some of which should correspond to actual words, and the others (the vast majority) are just junk.  The lambs are separated from the goats by checking for whether each 3pk exists in the global mapping.

- The residuum that correspond to actual words givea you the busy words for the current window. Note that this is a somewhat leaky filter. Some randomly generated 3pk's will usually correspond to real tokens just by random chance. This is because with a range of 1000, there are only a billion possible keys. We will see later that this is relatively harmless. 

## Processing the Batch of Busywords.

Each of the F frequency classes puts its set of busyword on a queue to an analysis thread. They are kept in synch by a concurrency "barrier" to keep the F batches of busywords together.

When the analysis thread gets all F batches of busywords, it does the clustering as follows.

- It filters a configured range of the set of recent Tweets for Tweets that contain at least M busywords. M might typically be 2, 3, or 4, but any number can be used.

- With proper configuration, this set of Tweets is quite small compared to the total number of Tweets upon which the batch of busywords was computed. 

- A clustering algorithm is applied to the set of Tweets with at least the minimum number of busywords. The algorithm finds groups of Tweets that have the most similarity in the sets of busywords they contain. 

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

# Starting and Verification Issues

# Speed of Computation and Significant Events
Ideally, we want to be able to process at a multiple of the firehose rate. These results are on a 12-year old iMac.

# Speed

The Tweet sample was recorded from two weeks of a Decahose. However, the Tweets can be consumed at any speed. 

The program sustains a thoughput of about 1100 Tweets/second. It is convenient to run it with filtering for English only. It processes about 2.2 decahoses.  Currently, the Tweets are read from Rabbit MQ and the ones you don't want because of language are discarded, so it's not clear that you'd get much difference in run speed due to filtering or not filtering for language. If this were done in the sender it would definitly make a difference.

Each CSV file is about five minutes of the decahose, i.e., about 30,000 Tweets per file, or 33 files per million Tweets. About 2.7 hours of the decahose is a million Tweets, but we're actually processing that million Tweets in about 1.2 hours.

This is useful to know if you want to start processing at a given date/time.  You need to start with sufficient files earlier than your start date in order to have the system primed. 

If you are starting and stopping frequently, the system can optionally read in the existing frequency data, for an instant start, but if you are starting at some random point the saved frequency data will be past it's use-by date. You need to start fresh or disregard the results until you have run for long enough to update the background statistics.

You can use the utilities mentioned below to find the date/time you want to start at in the two-week interval.

### Persistence of Subjects
In addition to computing the new subjects, the system can give information about how long those new subjects have existed in terms of the number of batches. This is given in the form of a list of the previous N batches that also match the new subjet.

Persistence of subjects is a major consumer of CPU, almost as big a cost as the clustering itself.  It can be turned on and off, but the effectiveness of turning it off is still to be determined.

The reason is that it isn't clear where the bottlenecks are. If it's the main thread that feeds the busy word processors, speeding up the analysis wouldn't speed up throughput.


# Data Set Start and End Time

We have a little more than two weeks of almost unbroken data. It might have been restarted briefly once or twice--probably not enough to matter.

## The data set:

- Starts around 2012-01-28 16:16:46, i.e. quarter after five on New Years Day
- Ends at 2012-02-15 03:22:36, which is 3:22 AM the day after Valentine's day.

The very first hours may or may not be fully trustworthy as there may have been some start and stop. Check using the file/time utility.

## The Whitney Houston Stuff Seems to Start Around Here.
gnip.csv_1329008058666_1329008358666.csv:168498929640550400, Feb 12 00:57:30 

This is toward the end of the 2-week+ period covered by the feed.

### Super Bowl
Note the superbowl is also a great place to see real conversations starting up. Feb 5 2012.  

# Running the Program and Utilities

In addition to the main program, there are programs to parse the original JSON into CVS, run RabbitMQ, send the data, and various utilities for data conversion and analysis. This section tells how to run them all.

## Parsing JSON files to CSV

### Build and Run the Golang JSON->CSV Parser--

The Golang JSON->CSV parser is not currently in use. The following is for future reference only.

cd /Users/petercoates/python-work/cursor-twitter
go build -o parser/parser parser/parser.go
./parser/parser -inputdir ../twits/msg_input/ -outputdir ../twits/test_output

### The Python JSON->CSV Parser
This is the JSON to CSV parser you should use.

This assumes you have a directory full of GZIP'ed JSON and an empty target directory for the CSV.  The program will unpack the zips into /tmp so as not to risk corrupting the original data.

Do the following. (You may need to adjust the directory names.)

cd to cursor-twitter.
 
python3 parser/parser.py ../twits/msg_input_3 ../twits/msg_output_3

The parser can be run with an optional --num-workers flag the gives the number of processing threads to be applied to the parsing. The speedup is roughly proportional to this value up to the number of hardware cores on your machine (which on this antique iMac is four.)

python parser.py input_dir output_dir  

## Go Utility to do Language Identification on CSV files

The lang: field in the original Tweets is crap. This utility post-processes the CSV files do language detection. 

This is not in the Python parser because Python language processing is agonizingly slow. Go is at least 200 time faster than doing it in Python.  

It also seems to do a noticeably better job classifying than the Python library.

### Build it
make build-language-detector

or

go build -o language_detector language_detector.go

### Run It
Use -help for details. There are flags to turn progress off and to set the number of threads working. The default of 8 threads is fine on a four core machine (like mine.)

With -progress,  the time to run the test set was 2m 40s,  and it reports 16,156 lines/second.

Without -progress the time to run the test was x and it reports 17,538 lines/second.

Note, you can stop it and restart it and it will ignore the input for which there are already files in the output. You may want to remove the newest 8 files in the taget directory because they may be incomplete.

./language_detector -input ../twits/test_language_detect -output ../twits/test_language_detect_out 


## Test program for parsed data
This program reads CSV files to ensure that we can create Tweets from them. Most users will not need this.

go run tests/csv_tweet_parse_test.go <path_to_your_csv_file>
  
## Building and Running The Main Program
    
The codebase is in ~/python-work/cursor-twitter. 

Data that it writes in order to persist data structures, etc. is configured to be in ~/python-work/data, i.e., at the same level as the project root. You can change this in config/config.yaml.  Using relative paths is treacherous.
 

## Starting RabbitMQ

RabbitMQ feeds Tweets in CSV form to the processor. The service normally running  as a daemon which in theory should be started as follows.

brew services start rabbitmq  

However, this doesn't seem to work right, so you can just ask Cursor to start rabbit in Docker or do manage it yourself with the following commands.
 
- docker start rabbitmq
- docker stop rabbitmq
- docker restart rabbitmq
- docker ps | grep rabbit
- docker logs rabbitmq

There was considerable drama aound getting this right. Rabbit was set up wrong so that it just silently dropped messages that weren't instantly picked up.  It is now configured right but it can be monitored on the web app.

http://localhost:15672/#/

The username and passwords are guest,guest.
 
## Sending Tweets

Running the actual software assumes you have a directory of CSV files of Tweets, in this case I'm using ../twits/msg_output_3. These have been language-processed so I can demo with just English.

The sender program makes a note in the data/sender of what the last file of CSV it sent was, and picks up with that when restarted. If the file is deleted or emptied, it starts at the beginning of whatever directory you point it at. The file (feel free to delete) is:

data/sender/sender_status.txt

There is code in the sender that pauses if the number of Tweets in MQ becomes excessive, but it seems to be redundant now, and the ACK mechanism seems to throttle on its own. The number of Tweets in MQ never seems to get out of two digits.

This is all you really need to send CSV Tweets.

cd to the cursor-twitter directory

python ./send_csv_to_mq.py ../twits/msg_output
 
 The following may be obsolete but they exist.
 --max-queue-depth 10000 
 --pause-duration 1.0

--max-queue-depth 5000 --pause-duration 2.0
 
##  Build and Run the Main Program

CD to the root cursor-twitter diectory.

./main -help tells you all the flags.

Note, this assumes you are in the project root. Sometimes cursor wants to run it from src which is not right. It is better to build yourself and run from the root directory. Don't let cursor run it at all.

cd /Users/petercoates/python-work/cursor-twitter

go build -o main src/main.go 

./main  -config ./config/config.yaml -print-tweets=false 

IMPORTANT FLAGS:
- profile  causes it to produce profiler output (see section on profiling). Profiling slows it down significantly but it doesn't matter because the results are relative.

- load-state  Causes it to load the latest state from disk. Without this it takes some minutes to build up enought tweets to start computing busy-words.

### Shutting down the main

Control-C will send a signal. If the process of writing persisted data to disk is underway it will finish before it shuts down. Otherwise it shuts down immediately.

## The Analysis Program

This is a utility to get a picture of how the universe of words grows with the number of Tweets processed. You need to point it at a large directory of CSV files.  

For up to a million Tweets, there are more distinct words than Tweets. At two million, there are slightly more distinct words than Tweets. By four million, the ratio is getting a little smaller. Don't really know when/if it tapers off. It seems like it would almost have to at some point but up to 11,000,000, which is a couple of weeks of the decahose, it doesn't seem to taper off that much.

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
This is interesting because is shows how extremely Ziph like the distribution is. Half of all word usage is accounted for by just 250 words, and 99% of usage is accounted for by the top 6156,473 words. The rest of the 10,906,656 words account for only 1% of all usage.

- There are 11,522,129 distinct tokens used in "en" Tweets.
- Many, particularly among the unusual words, do not appear to be English.
- The top 220 tokens (which are almost all words) account for half of all usage.
- The top 3,572 words account for 80% of all usage.
- The top 13,690 words account for 90% of all usage.
- The top 615,473 words account for 99% of all usage.
 
Moreover, relatively few tokens with rank less than 100k are dictionary words. The majority of words of rank between 100k and a million low appear to be ordinary names, screen names, non-English words, misspellings, or nonsense. Beyond a rank of million, almost all are such.

It's mostly the relatively rare words we care about because they carry the most meaning when they surge in usage.

### Distinct Tokens for All Languages (all)
- There are 16,863,516 distinct tokens for all langauges.
- The overall properties seem pretty similar.
- 306 tokens comprise 50% of all usage.

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


# TTD and Direction

## Immediate Stuff
- Run with the subject persistence turned off. See if you get a performance bump.
- Run with the useless words file empty and the minimum length set to 1 to see if it really is eleminating that many words.

## Possible Parser Bug
IMPORTANT. CHECK THE GOLANG PARSER FOR PROPER SPLITTING ON PERIODS AND COMMAS.

## Overhaul of ASCII Output and Reduction of Logs

### Clean up logs
A ton of unnecessary logging is left over from primary development.
- Eliminate unnecessary logging
- Find some logging framework that will allow any level to be suppressed, etc.
- Move all remaining screen logging to standard error.

### Create usable output
A more complete text output should print the essential information needed for display to standard out or to a named file.
This would be include:
- The clusters, in a computationally friendly format.
- The busy word information, in a computationally friendly format.
- Both should include admin information like the run time, the time as given in the Tweets (this will be easy to obtain down to a minute or so,) etc.
- The lines can be commingled so long as they have an identifying leading field, e.g, CLUSTER, BUSYWORD, ADMIN, ETC.
- We would need to print all the diagnostics and other screen junk to stderr.

    

## Ongoing  Items
- Comments Key areas need to be commented to keep out don't touch, etc.

- Make tests around everything.

## Possible Errors

### Verify That Writing Token Files is Correct

Consider if the logic around writing token files is correct w.r.t. restarts: token_batch_002309.txt. 

I think it is--it picks up as closely as possible to where the new feed will be. 

However, that is only valid if the restart is fast. If a significant amount of time has elapsed, both these files and the countmap will be far from accurate. Consider how long before this is likely to be a problem.  A warning line would be in order!
 
## Things That are Stubbed or Partially Built
### Useless and Offensive Word Filter
- The word file could be better populated by looking through the logged busyword output and the cluster output,
- Sanity check whether the list can actually be comprising 40 percent of tokens! That seems high. 

 
## Better Busy Word Detection

This would be a significant effort.

Consider that dual sets of pipelines, or even three sets, with different hash functions, could do a much better job of filtering of the true busy words. 

If you use two sets, take only tokens that appear in both busy word pipelines.  If you use three sets, take only tokens that appear in 2/3 or 3/3. This would eliminate almost all spurious busywords.

The word space size is on the order of a billion. The actual set of words probably has at most a few tens of millions. Call it 10 million. That's one in a hundred random 3pk's corresponds to a real word. If you had two completely distinct computations of busy words, the chance of a given random 3pk colliding with a real token is one in ten thousand.  

This means that for a given acceptable level of collisions, one can make the filter less discriminatory, i.e., a lower minimum Z value, or you can make the probability of an error much smaller.  My intuition is that the former is probably what you want.

Possible benefits:
- You could ditch the spurious words more effectively because they won't show up in both or all three sets.

- You can tolerate a lower Z score because you have a way to remove the ones that result from a non-busy token just happening to have landed on othewise slightly outlying counts.

It is not clear how big an impact this would have on performance. Possibly not that much. Run performance monitoring to see how much processing power goes into the busy word filters. It might be that the real processing is in the message reading and parsing.

Risk. If trippling the work in the busyword processors made them in aggregate slower than the combined main pipeline and the clustering, it would cause the queues to grow without bound and crash the program. You'd need to detect the problem and throttle the reads if this is a problem.
 

## 3pk Collisions
Over two weeks there are about 11 million distinct tokens.  That means there is about a 1/90 chance of a given token colliding with another token's 3pk.

This could be greatly mitigated if we only kept 3pk's for the tokens that have non-zero counts, because most of the words will appear once and vanish without a trace. This would keep the actual number of mapped tokens way down.

This could be done in a thread-safe efficient way if:
- The FCT put any token the count of which hit's zero on a queue that is read by the main thread. The FCT would have to remove the entry from the counts, not just set the count to zero!
- The main checks this queue on each cycle, or every N cycles, and deletes the 3pk's for those tokens from the global mapping of 3pk<->tokens.
- This means that the probablilty of a given token colliding would be more like one divided by the cardinality of the global counter map, which is typically in the 100's of thousands, i.e. about 1% of the universe of words.

This does not seem like a hugely expensive item. You'd pay a little bit for the extra insertions and deletions from the map. The map is read asynchronously by the busyword processors so thread safety is an issue.

You could to the check every, say, 100 tokens. 

Note, it would be very unusual, but in theory the busyword processors could require a token for a 3pk that has been deleted since the cycle started. This has to be guarded against.

Also, this needs concurrent access protection. RWMutex is probably ideal as we have many more reads than writes, and reads don't block each other. We are probably lready be doing this. At least we should be!

### Performance
We started to get a significant slow-down at some point. It's about 1100/second now, and it used to be in the mid-2000's. This could be consequence of some real piece of functionality, or it could be something stupid. 

Went over this with the profiler and chased down a lot of stuff, but it's still slower. Cursor's opinion is that it is probably primarily in taking tweets from RabbitMQ. I doubt it, because we've taken them off at up to 3000/second in the past when we weren't doing anything with them.  

The bottleneck seems to be on the main processing line because the FCT is not in the path of the Tweets, and the busyword and analysis portion won't block the main line processing because it is insulated behind queues.

### Pipeline Stats Had Extensive Repairs
Verify that the very high rates of rejection of tokens are correct. Most of the rejection comes from the list of words to ingore as busy words, and words of one or two characters.

Those are 36.6% and 40.9% of all tokens!

If this is just because we're ignoring a ton of meaningless junk words, great, but it needs to be verified. 

Get Cursor to write a utility to see if those stats are consistent with the proportions of those kinds of words in the raw data.  Or just remove the words from the file, set the minimum length to one, and see if it still rejects a lot of tokens!
 
### Speedup From Turning Off Cluster Persistence
Cluster persistence is a huge consumer of CPU, but it's not clear that turning it off or optimizing it would help because it's not clear where the bottlneck is.
 
If the busyword and analysis were not keeping up, why should that slow down the main-line processing? Would it not cause a crash from OOM?  Not necessarily, as we only have four cores--it could be starving the main line of CPU and causing the slowdown. The back end might use less CPU than the main line, but still use so much that the main line slows down.
 
If you multi-threaded the main pipeline, it might help, but if it did, you'd need to check the busyword queue lengths to throttle reads if they start to bloat!

Probably part of the bottleneck is taking the Tweets from RabbitMQ, which means any other efficiency cleanups would be moot unless multiple threads can take data from RabbitMQ faster than one can.
  
### RT's \#hashtags and @this_n_that
Many Tweets are identical except for RT's, @this_and_that, #this_and_that. What would be the effect of removing these in the either the clustering or the display?  

We have config parameters to drop \#hashtags, @sign_words,

One suspects that the majority of clusters would disappear or shink radically if these items were not included. Not clear what the meaning and/or desirability of this would be. They may be a legitimate contributor to identifying the emergence of a subject.
 

### ????????? Strings in Texts

Still getting lots of ??????????????????????. I thought we were dropping those Tweets? This is not done in the main pipeline. It could be implemented there--if a Tweet text has more than N question markes, ignore it.   

Make sure the ????'s are actual ASCII question marks and not just an artifact of displaying un-printable Unicode values. I think they are in fact ASCII '?' characters because the output does show up a lot of non-ASCII stuff like smiley faces and hearts.
  

## Ongoing 
- Comments Key areas need to be commented to keep out don't touch, etc.

- Clean up the excessive logging and print outs

- Make tests around everything.

 
### Things That are Stubbed or Partially Built
- Build out the offensive word detection mechanism. Check. This is in the tokenizer. There is a file of explicit words. There is also some generic activities, like a minimum token length in config.yaml. 

- The busy words contain a lot of dreck. Scan the clustering output for more words that can go in the ignore tokens file
  

## Clustering Across Batches

Note, this has now been implemented but may not be adequately tuned up yet. See above relating to this under speedups. 

## A Graphical Front End
This is a big one!  Not sure how to go about it with Cursor. I wrote the graphics by hand in Java last time!

A busy-word fade-out like the bubbles would be great.
- The size proportional to the number of Tweets the busy word is in. Or perhaps logarithmically proportional.
- When the BW was last seen, fading off the left
- Vertical axis is the frequency class

# Tuning 

## Improve busy word detection
This relates to the first item in this list, which is parallel busy word detection to eliminate spurious busy words.

### Increasing the number of frequency classes. Say to 20 or 25.
		- I upped it from 10 to 24 and it seems to be better. 
		- More experimentation is in order
		- More threads? More threads might allow higher minimum Z scores.

### Current clustering is graph-based Louvain/modularity. 
	- Seems to works OK
	- Added k-means but it didn't seem to work very well.
		- Still worth fiddling with to see if it's just a tuning problem.
		- K-means seemed to work well enough on the Java version
		- Disadvantage is, you need a guess for K and the number of clusters at a given moment seems to vary a lot. 
	- Couldn't find a good k-means library that worked in Go, so Cursor wrote the routine.

   
## Understand the effect of window size and batch size better.
ARE THE NUMBERS HERE RIGHT? 

The window is now 20m tokens, or about 2m Tweets. 
	- That's about 66 minutes of the decahose, or about 28 minutes of actual time at the rate we consume.  
	- 66 minutes seems like a reasonable turnover. How far out of sync with the day would you get in an hour? But it could be shorter.  Worth trying.

- Batch size is now 10,000
	- 10k Tweets is 20 seconds of decahose, just 7 or 8 seconds of actual processing time. Try a larger value.
		- 10k Tweets is approximately 100k tokens--less than one five minute file.
		- 100k tokens over 24 busywork processors is about 4000 tokens per processor. Over 1200 counters, that's a mean of only 3.3 per counter.
 
### Jacquard Similarity
Jacquard similarity for the clustering seems to be applied to all the tokens in the Tweets. 
  - Is that true, and if so, is it correct? 
  - Maybe it should be only on the busywords. 
  - Jacquard similarity might not be important. Maybe just the raw occurrences?
 
## Clustering Quality
- The clusters don't seem as good as they were with the old Java implementation. See tuning steps above. My guess is, the clusterng is probably off.  See above
 
## Language Imbalance in Clusters

At some settings, 100% of the clusters appear to be in Spanish (or maybe some Portuguese). At other settings English Tweets show up better. 

This seems to be influenced by the Z scores. (?!?)
  - z_scores: [4.0, 5.0, 4.0, 3.5, 3.5, 3.5, 3.5, 3.5, 3.5, 3.0]  got mucho Spanish clusters.
  - z_scores: [5.0, 5.0, 5.0, 5.0, 3.5, 3.5, 3.5, 3.0, 2.5, 2.5]  got mucho English clusters and only some Spanish and Portuguese.
 
 
# Miscelaneous Details

## The Big Token Window.

People don't Tweet the same things at breakfast on Monday that they do at 1:00 AM on Saturday.  Also, the world turns, and while New Yorkers are getting up in the morning, people in Bejing are out for the evening.

The number of Tweets necessary to get a large sliding window of the universe of words into the word counts is quite large. The machinery for keeping them in memory and aging out the old ones one at a time becomes burdensome.

Accordingly, the tokens underlying the token counters are written to disk in batches. After the number of token batch files reaches a defined limit, each time a new batch is written out, the oldest batch is read in (and deleted from disk). The contents of the old batch are then used to decrement the global token counts.

This keeps the token counts relatively up to date with the flow of Tweets.  Note that the number of Tweets underlying a given moment's token counting is much larger than the number of Tweets underlying a busyword processing batch. Batches might be 10,000 Tweets, perhaps 100k tokens. The big token window is typically in the millions.

Note also that frequency calculations typically happen multiple times in the span of time represented by an entire token window.  They are controlled by separate parameters.  

## The Small Window of Tweets

The big token window is only of concern to the off-line frequency calculating thread.  The main thread keeps short window of Tweets in memory for use in the clustering algorithm.

This is a conventional queue that keeps a configured number of Tweets, ageing out the old ones as new ones are added.  This would be configured to hold a few processing batches of Tweets. As a batch is typically in the range of one thousand to a few thousands, it would be a small multiple of that size.  

The size of a batch, the number of batches kept in memory, and the number of batches used to compute a clustering are all configurable.
 

# Running the Profiler
 
 ./main -config ./config/config.yaml -print-tweets=false -profile

 Let it run

  go tool pprof -text cpu.prof

  or 

    go tool pprof -http=:8080 cpu.prof


  You can get a quick summary with
  go tool pprof -top cpu.prof

 
# How to Create Short Lived Git Branches so Cursor Doesn't Trash Yo Shit

## Start From Main branch
git checkout main
git pull origin main


## Create and Switch to New Branch
git checkout -b my-feature


## Make Your Changes and Commit Them
git add .
git commit -m "Test change"

## Switch Back to Main and Merge the Feature Branch
git checkout main
git merge my-feature

This will fast-forward merge if no other changes have been made to main.

## Remove an Unneeded Feature Branch 
This is if it only exists locally.

 git branch -d branch-name
 

   
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

- It is 139,610,0077, i.e. 139 million Tweets.  This is about a quarter of the entire data set available.

- The analysis program reports that there are 44,298,119 distinct tokens.

- 139 million Tweets equals approximately 1.39 billion tokens. So, about one token in 32 is novel over a vast number of Tweets.

- Note that we don't ever have 44 million tokens in the current set, as they are aged out after being in the current count for long enough to process about 2 million tokens, or 220,000 tweets, or with the decahose, about seven minutes of processing.


## Twitter Stats
- Volume of decahose = 500/sec

- Volume of firehose = 5000/sec

- 500/sec = 30k Tweets/min = 450,000 Tweets/15min  = 1.8m Tweets/hour

- There are an average of 10 words/Tweet = 4,400,000 words/15 min or 18m words/hour

## Our Processing
- The sender can send about 3000/second

- We only receive about 1200/second, or 2.4 Decahoses.

- The Tweets get ACKed, so rabbitMQ manages to never let the queue get very long--it seems to never get up to 50.
  
- Google says a modern Mac could probably do about 10x as fast as this antique iMac. So, a new machine could handle multiple fire hoses.  This is important, because one of the purposes of this is to make it possible to handle historical data. If a powerful server could handle four firehoses, and you applied one to each year of Tweets, you could process the entire history of Twitter in three months.

I'm skeptical. The processor on this machine is actually really fast, despite being 12 years old. The speedups would be because of SSD storage and more cores. So I'm guessing that the number of cores would be the overwhelming factor. 

So a 16 core machine would probably just about do the firehose. Possibly more if you moved the feeder to a different box.

- We have about 350 hours of decahose. At 2.5 decahoses, we could process the entire data set on this machine in about 140 hours or a little under six days. Which seems about right--I think it took a more than two weeks to collect it.
 