# Session Notes

 
# TTD
 
## Tuning

- Poor busy word detection.
	- Increasing the number of frequency classes. Say to 20 or 25.
		- I upped it from 10 to 24 and it seems to be better. 
		- More experimentation is in order
		- Who knows, maybe more?  But you start to get too many busy words, so it probably interacts with the Z scores.

- Current clustering is graph-based Louvain/modularity. 
  - Add k-means based on the busy words alone.
	- k-means didn't see to work very well.
	- Still worth fiddling with to see if it's just a tuning problem.
	- Couldn't find a good k-means library that worked so Cursor wrote the routine.

  
## Other Tuning
- The window is now 20m tokens, or about 2m Tweets. 
	- That's about 66 minutes of the decahose, or about 28 minutes of actual time at the rate we consume.  
	- 66 minutes seems like a reasonable turnover. How far out of sync with the day would you get in an hour? But it could be shorter.  Worth trying.
- Batch size is now 10,000. 
	- Verify in the code that it really is Tweets and not tokens. 
	- 10k Tweets is 20 seconds of decahose. That's a short time. Try a larger value. 

- Examine the de-duplication functionality in the cluster display. Make sure it does what it's supposed to. The purpose is to not show a zillion copies of basically the same Tweet.

- Jacquard similarity for the clustering seems to be applied to all the tokens in the Tweets. 
  - Is that true, and if so, is it correct? 
  - Maybe it should be only on the busywords. 
  - In which case, Jacquard similarity might not be important. Maybe just the raw occurrences?

- I could swear that I used k-means clustering before and had good results. Think about swapping in a k-means implementation and being able to swtich among them.

- There are a lot of tweets like the following, filled with ???. Maybe the should be stripped out in the tokeininzing phase?  Pehaps even have the entire Tweet discarded as they usually seem to dominate the Tweet they are in.

│
┌─ Cluster 2 (15 tweets) [earthquake, jishin]
│  ├─ "RT @eew_jp: ???? 2012/01/29 16:46???????????????20km????????4.6???????????????????????3??? http://t.co/qk0HXUfl #jishin #earthquake" (13 instances)
│  ├─ "????(&gt;_&lt; ) RT @eq_tokyo: ??????????(??20km)?M3.7????????????????0??(16:47:03)?????????????#saigai #eqjp #earthquake #jishin #?? #??"
│  └─ "RT @eq_tokyo: ??????????(??20km)?M3.7????????????????0??(16:47:03)?????????????#saigai #eqjp #earthquake #jishin #?? #??"
│
 
## Cluster Quality
- The clusters don't seem nearly as good as they were with the old Java implementation.

## Language

The language field in the JSON is worthless. The parser has a flag to detect language but it makes it run at a small fraction of the speed. 

The parser is now multi-threaded that speeds it up significantly, but it's still quite slow.
 
Detecting language on the fly isn't even a starter.  It's dog slow.

## Language Imbalance in Clusters

At some settings, 100% of the clusters appear to be in Spanish (or maybe some Portuguese). At other settings English Tweets show up better. 

This seems to be influenced by the Z scores. (?!?)
  - z_scores: [4.0, 5.0, 4.0, 3.5, 3.5, 3.5, 3.5, 3.5, 3.5, 3.0]  got mucho Spanish clusters.
  - z_scores: [5.0, 5.0, 5.0, 5.0, 3.5, 3.5, 3.5, 3.0, 2.5, 2.5]  got mucho English clusters and only some Spanish and Portuguese.



# Cursor Principles 

Before doing anything that changes code, please ask me any questions you have about this before you begin.

Never, under any circumstances do any git operation for any reason until we have explicitly discussed the operation you contemplate. Assume you misunderstood me. 

Don't run the program yourself without asking. The program runs forever and I have to kill it to get your attention back. I will handle running the program. 

If you are going to make changes that you cannot rewind without getting further input from me, warn me so that I can commit everything in a working state.

Do not implement anything without explicit approval. Sometimes I just want to talk about an approach. Asking a question or soliciting your opinion doesn't mean that I want to rewrite the codebase!

For every feature, we need to add unit tests.  The tests should be carefully commented about what is being tested and why the pass/fail conditions are what they are.

For any significant code change, we must run the test suite! 

When a test doesn't pass, we have to look at why before changing anything. No removing tests because they don't pass unless we agree the test is obsolete.

If any changes seem to involve multiple threads, be sure to get my agreement before doing anything. Anytime there thread safety constructs like mutex, etc., always check. We keep getting into trouble with unnecessary and incorrect thread complexity.



# Project Goal
The project is a way to read the Twitter firehose and find new subjects appearing in the Tweet stream in near real time. Ideally, significant new subjects will be identified within seconds of their surging into the stream.

The underlying insight is that it would be essentially useless to 
analyse all the subjects that are active at any one time because this number is in the tens of thousands--at least 500x too much for a human to grasp. However, if one settles for only seeing new subjects as they arrive:
- The amount of data for a human to be exposed to is vastly smaller
- The subjects arrive most-important-first
- You asymptotically approach getting **all** subjects with the most useful first.

Fortunately, new subjects tend to arrive at a manageable rate. Depending upon exactly how you parameterize the definition of "subject," new subjects arrive perhaps every few seconds. Tuned well, you get something with about the information density of the Times Square News Ticker.

To find subjects by characterizing the semantics of thousands of Tweets per second, and then grouping them together based upon subject similarity would be a daunting task computationally. Certainly it would be extremely difficult to do in real time. Fortunately, however, groups of meaningful words used together are an excellent proxy for semantics. 

Interestingly, a subject is easiest to identify not by grouping based upon the most frequently used words, but by spotting less frequencly used words that being used are anomalously frequently at the moment. The most frequently used words are mostly useless because they are either filler, like "the" and "they" or they are part of subjects so vast as to be meaningless. There are 1000 conversations about Diddy or Trump at any one time.

Words have a Zipf distribution, which means that overwhelming majority of words are very rarely used (on the order of zero to a handful of times a day). This is good, because in practice, new subjects are typically characterised by the appearance together of two or three (or more) words that are suddenly being used with unusual frequency.

For this to be helpful, the anomalously frequently used words (called here busy words) must be detected very quickly because it must be practical to analyse the very recent window of Tweets for clusterings of busy words. To get a fine-grained view, the granularity of the analyisis should be on the order of a few seconds.

Ordinary word frequency computation would be very hard to do sufficiently quickly,because new Tweets are arrivign at up to 5000/second. It is also a poor method for finding words that are suddenly being used unusually often for fundamental reasons. Moreover, it imples the need to keep the results of two frequency analyses so that the current window's results can be compared to the previous window's results. It's cumbersome.

Therefore we use a probabilistic heuristic that identifies surges in word frequency without doing frequency analysis inline. A global frequency computation is done periodically off line in order to keep up with background word usage changes with time of day, day of week, etc., as well as with ongoing changes due to the emergence of new subjects, but the main pipeline of analysis does not need do the global computations necessary that are so cumbersome.

The heuristic does not do frequency counts, but detects only the leading edge of surges in frequency. As it is only sensitive to the leading edge of a surge, it automatically forgets words that surge briefly in usage, as well as words that surge and then stay at an elevated level.

The heuristic is extremely fast--potentially considerably faster than the firehose rate. Running on a ten-year old iMac, it processes about three Decahoses.A server could run it multiple firehose speeds.  Speed is important, because a main goal of the project is the ability to analyse historical data economically.

# Input and Output

The input for development purposes is JSON-formatted Tweets in files that have the file order encoded in the filenames as Unix start and end times. The files are GZIP'ed.  In production the input could be either files
(for historical processing) or the actual live Tweet stream (for real time processing).

The non-graphical output is a series of clustered set of Tweets that are about new subjects. There is not yet a graphical front end.
 

# The Approach

The fundamental principle is that "subjects" are best identified by sets of words that are suddenly appearing with unusual frequency.

Word usage famously has a Zipf distribution, which means that 99% of words are very rarely used. "Rarely" in this context typically means perhaps zero, once, or twice a day. A word that is suddenly appearing unusually frequently we call a "busy word."

Because most words are very rare, literally one-in-a-million in a text stream, if two or three busy words appear together in a few Tweets, it's almost certainly a new subject or an existing subject that is seeing sudden growth.
 
A word being busy is not about its absolute frequency. It is about it's relative 
increase in frequency of use. Very common words, like "the", "a", "Trump", "Beyonce", or "Swift" contain almost no information. It is less common words, such as people and place names, suddenly popping up together that signify that people are saying something new, dragging the super-common names along with them.

If a hot subject comes along, i.e. people are tweeting and retweeting text that contains some subset of three or four busy words, the subject will tend to be forgotten after a while because the rates of use that caused them to be marked as busy words slowly becomes the new norm, or decline to their old obscurity. This process can happen over seconds or minutes.

However, in practice, a major subject will keep getting augmented with other unusual but related words, so it keeps the subject fresh far beyond the expiration of the original busy words. This is in keeping with our intuition. As a conversation evolves, the original words hang around, but they are elaborated with more and more varied vocabulary.

The key to this working is that the sliding window has to be small, e.g., typically in the range pf thousands of Tweets. With too wide a window the granularity of the view erodes.  

So sum up, the algorithm 
- Detects all the currently surging words.
- Finds the Tweets in the current window that use a proper subset of these words
- Discards all the other Tweets
- Clusters the busy-word Tweets by the busy words they use.  This isn't necessarily a unique clustering.

# What's Wrong with Using Frequency Calculations
 
Conventional frequency calculations don't work well because they operate on the universe of words, with is large--several tens of millions. You have to:
- Count the frequencies of all words in the current window of time.
- Compute the relative frequencies for the words every few seconds.
- Compare the relative frequencies of the current window to those of previous window to compute usage-growth rates for every word.
- Filter out the ones that have excessive growth rates.
- Discard the previous frequency computation, demote the current to previous, and start a new set of counts.

You only have a few seconds to accomplish this before the set of Tweets that arrive in the time span grows to unwieldy size. At 5000/second, it does not take many seconds before the number of Tweets that must be analyzed and clustered grows to unwieldy size (clustering algorithms are inherently non-linear in the size of the set to be clustered.)

# The Heuristic

## Word Frequency Computations

We use a periodic offline computation of global word frequency to divide the universe of words into F frequency classes, with the most frequently used words in the first class, and the least frequent words in the last class. 

This frequency analysis is done outside of the processing of the stream of inbound Tweets and covers a larger time span than the seconds required for the heuristic itself. The purpose is to provide a background look at what normal word frequencies are at the particular time of day, day of week, etc.

The global computation is done at a coarse grain, e.g., several minutes. The word freqencies are computed and the universe of words are divided into F equivalence classes according to frequency. The division is done in such a way that the words in each class will account for approximately equal usage. 

The frequency classes are used to construct a filter that allows an incoming word's background frequency to be identified. The first few classes contain only a few words, and the last class might have more words than all the other classes put together. Consider that any incoming word that doesn't match to any frequency class is necessarily rare, so it it can be treated as being in the least frequent class.

The word counts, frequency class computation, etc. are done on a separate thread, so it has no reall effect on processing speed. Behind the scenes it swaps in new frequency filters to be used by the main processing pipeline.

## Receiving Tweets
The main routine reads inbound Tweets in CSV format. This detail is a convenience because the Tweet data is canned. They could be received in any format, including the native JSON. It parses them and converts them to Tweet structs in memory.

The receiving phase:

- Maintains a sliding window of the latest W Tweets. The ouput of the system is computed over batches of a few seconds worth of Tweets, so W is chosen keep a few batches of Tweets available for the clustering phase that will happen a few seconds later.

- It extracts the words and normalizes them, unifying the case, discarding junk, etc.

- It puts the tokens on a queue for the off-line frequency calculations to use. 

- It checks a global table to see if a "three part key" or 3pk has been computed for the token. If not, it computes one and sticks it in the table.
 
Busy words are computed for separatedly for each of the F frequency classes. Each busyword processor gets the data to work on as follows.

- The main processing loop obtains the frequency class for each token using the frequency class filters.

- The busyword processors work on 3pk's, not tokens, so the 3pK's for the tokens are looked up and the handed off to a corresponding queues.

- When one "batch" of Tweets has been processed, the main puts a special 3pk on each queue to signal to the busyword processors that the batch is complete. The get the signal at the same place in the Tweet stream, which keeps them synchonized.

Handing off 3pk, to the frequency queues is the end of the main loop, which continues forever, receiving Tweets and putting the 3pk's on the frequency queues.

Note that the busy word processors work or shorter windows of Tweets--a few thousand in a window, which for the decahose is just a few seconds worth of Tweets. When a window of tokens have been handed to the busy word processors.

## The 3pK's
We haven't said exactly what a 3pk is.  It is an ordered triple of hashes for some token, or word.  The hashes are each parameterized with a different value, so it is really just three pseudo random numbers in a defined range.  This range is a parameter, but it is somewhere around a thousand or so. 

As mentioned above, a global mapping of 3pk's to tokens, and tokens to 3pk's is maintained.

If the range is 1000, that gives a 3pk space of a billion pseudo-random triples. There are a few million word in the global computation at any one time, so there is a small chance of a collision for any token. However, as we will see later, collisions are not very harmful.  

The range can be increased to reduce this probablility farther, but there are computational costs to a larger range. Computing the optimal range size is an open question.

## The Busyword Processor Threads
 
Each of the F busyword processors works the same way.

The hashes from each 3pk are used to index into three arrays of C counters. (Array size is the same as the 3pk range.) One counter in each of the three arrays gets bumped for each incoming 3pk.

Because the 3pk values are pseudo random, if every word had the same probability, the counters would be approximately equal with a Gaussian distribution.  However, we are assuming that some are anomalously busy. After all, that is the point of the exercise. Therefore, some of the counters will have exceptionally
high counts.

When the signal 3pk is read (-1, -1, -1) the processor suspends reading the queue and processes a batch.

- A Z-score is computed for each counter. This is an normalized value that corresponds to the standard deviation. So every counter position has a corresponding Z value that gives the improbability of a count having been
reached at random.  The Z's average to 0, by definition, and the higher the Z, the less likely the value is just random variation.

- An arbitrary Z is set in configuration. The Z says how high the counter value must be to be considered freaky high.

- The set of indexes of the abnormally high Z score counters are collected for each of the three counter arrays.

- The Cartesian product of the three index sets is computed. This gives you a set of three-part keys, some of which should correspond to actual words, and the others (the vast majority) are just junk.  The lambs are separated from the goats by checking for whether each 3pk exists in the global mapping.

 
- The residuum that correspond to actual words give you the busy words for the current window.

# Processing the Batch of Busywords.

Each of the F frequency classes puts its set of busyword on a queue to an analysis thread. They are kept in synch by a "barrier".

When the analysis thread gets all F batches of busywords, it does the clustering.

- It filters a configured amount of the set of recent Tweets for Tweets that contain at least M busywords. M might be 1, 2, or 3.

- This set of Tweets is quite small compared to the total number of Tweets upon which the batch of busywords was computed.

- A clustering algorithm is applied to the Tweets with busywords. The algorithm finds groups of Tweets that have the most similarity in their busywords. It's a non-deterministic process because clusters can overlap a little. For instance, one group might be characterized by five busywords, and another by four, but they share one busyword. However, despite that overlap, they the two groups are identifiably differen


## Details

### The Big Tweet Window.

Throughout the day and the week, the things people Tweet about change. People don't Tweet the same things at breakfast on Monday that they do at 1:00 AM on Saturday. 

The number of Tweets necessary to get a large sliding window of the universe of words into the word counts is quite large. The machinery for keeping them in memory and aging out the old ones becomes oppressive. 

Accordingly, the tokens underlying the token counters are written to disk in batches. When the number of token batch files reches a defined limit, each time a new batch is written out, the oldest batch is read in (and deleted from disk). The contents of the old batch are used to decrement the global token counts.

This keeps the token counts relatively up to date with the flow of Tweets.

### The Small Window of Tweets
The big Tweet window is only of concern to the off-line frequency calculating thread.  The main thread keeps short window of Tweets in memory for use in the clustering algorithm.

This is a conventional queue that keeps a configured number of Tweets, againg out the old ones as new ones are added.  This would be configured to hold a few processing batches of Tweets. As a batch is typically in the few-thousands, it would be a small multple of that size.


# Output display

The current output is an ASCII representation of the clusters.  A graphical display is contemplated.

  
# Useful Commands

## Parsing JSON files to CSV

### Build and Run the Golang JSON->CSV Parser--

There is or was a Golang JSON->CSV parser, but it is not in use. The following is for future reference only.

cd /Users/petercoates/python-work/cursor-twitter
go build -o parser/parser parser/parser.go
./parser/parser -inputdir ../twits/msg_input/ -outputdir ../twits/test_output

## Python JSON->CSV Parser
This assumes you have a directory full of GZIP'ed JSON and an empty target directory for the CSV.  The source directory must be writable for unpacking the zips. I recommend copying the set of files to a working input directory not using the reference set, to avoid corrupting any files.

cd to cursor-twitter.

Run the following.
 
python3 parser/parser.py ../twits/msg_input_3 ../twits/msg_output_3

python parser.py input_dir output_dir --num-workers 8

## Test program for parsed data
This program reads CSV files to ensure that we can create Tweets from them.

go run tests/csv_tweet_parse_test.go <path_to_your_csv_file>
  
## Building and Running The Main Program
    
The codebase is in ~/python-work/cursor-twitter. 

Data that it writes in order to persist data structures, etc. is configured to be in ~/python-work/data, i.e., at the same level as the project root. You can change this in config/config.yaml.  Using relative paths is treacherous.
 

## Starting RabbitMQ

RabbitMQ feeds Tweets in CSV form to the processor. The service normally running  as a daemon which in theory should be started as follows.

brew services start rabbitmq  

However, this doesn't seem to work right, so just ask Cursor to start rabbit in Docker. 

In theory the following commands will run it in docker, restart, etc.
- docker start rabbitmq
- docker stop rabbitmq
- docker restart rabbitmq
- docker ps | grep rabbit
- docker logs rabbitmq

There was considerable drama aound getting this right. Rabbit was set up wrong so that it just silently dropped messages that weren't instantly picked up.  It is now configured right but it can be monitored on the web app.

http://localhost:15672/#/


 
## Sending Tweets

Running the actual software assumes you have a directory of CSV files of Tweets, in this case ../twits/msg_output_3 has about 38 million tweets which is about 21 hours of the decahose. We process at about 1200/second, so the entire set should run in 9 hours or so.

The sender makes a not in the data/sender of what the last file of CSV it sent was, and picks up with that when restarted. If the file is deleted or emptied, it starts at the beginning.

data/sender/sender_status.txt

There is code in the sender that pauses if the number of Tweets in MQ becomes excessive, but it seems to be redundant now, and the ACK mechanism seems to throttle on its own. The number of Tweets in MQ never seems to get out of two digits.

This is all you really need to send CSV Tweets.

cd to the cursor-twitter directory

python ./send_csv_to_mq.py ../twits/msg_output

 
 The following may be obsolete but they do exist.
 --max-queue-depth 10000 
 --pause-duration 1.0

--max-queue-depth 5000 --pause-duration 2.0
 
##  Build and Run the Main Program

CD to the root cursor-twitter diectory.

./main -help tells you all the flags.

Note, this assumes you are in the root. Sometimes cursor wants to run it from src which is not right. It is better to build and run from the root directory. Don't let cursor run it at all.

- cd /Users/petercoates/python-work/cursor-twitter

- go build -o main src/main.go 

- ./main  -config ./config/config.yaml -print-tweets=false 

IMPORTANT FLAGS:
- profile  causes it to produce profiler output (see section on profiling). Profiling slows the whole thing down--the results are relative.

- load-state  Causes it to load the state from disk. Without this it takes some minutes to build up enought tweets to start computing busy-words.

## Shutting down the main

Control-C will send a signal. If the process of writing persisted data to disk is underway it will finish before it shuts down. Otherwise it shuts down immediately.

## The Analysis Program

This is a utility to get a picture of how the universe of words grows with the number of Tweets processed. You need to point it at a large directory of CSV files.  

For up to a million Tweets, there are more distinct words than Tweets. At two million, there are slightly more distinct words than Tweets. By four million, the ration is getting a little smaller. Don't really know when/if it tapers off. It seems like it would almost have to at some point!

cd to cursor-twitter
go build -o analyze_tokens analyze_tokens.go 
./analyze_tokens -input ../twits/msg_output


## Tests
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

## Run the Profiler
 
 ./main -config ./config/config.yaml -print-tweets=false -profile

 Let it run

  go tool pprof -text cpu.prof

  or 

    go tool pprof -http=:8080 cpu.prof


  You can get a quick summary with
  go tool pprof -top cpu.prof

 
 

# Some Sample Code for PTC's Edification
sample of how to do mutex to protect the data structures
that are modified by other threads.

var (
    mu   sync.RWMutex
    data map[string]string  // or whatever structure(s)
)

func mainLoop() {
    for {
        mu.RLock()
        d := data["someKey"]
        mu.RUnlock()

        fmt.Println(d)
        time.Sleep(100 * time.Millisecond)
    }
}

func updateData() {
    newData := make(map[string]string)
    newData["someKey"] = "newValue"

    mu.Lock()
    data = newData
    mu.Unlock()
}

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
 

# TODO / Next Steps

This section categorizes things to do, bug fixes, and upcoming development tasks.

## Minor   

## Ongoing 
- Comments Key areas need to be commented to keep out don't touch, etc.

- Clean up the excessive logging and print outs

- Make tests around everything.

## Possible Errors

### Verify Writing Token Files is Correct

Consider if the logic around writing token files is correct w.r.t. restarts: token_batch_002309.txt. 

I think it is--it picks up as closely as possible to where the new feed will be. 

However, that is only valid if the restart is fast. If a significant amount of time has elapsed, both these files and the countmap will be far from accurate. Consider how long before this is likely to be a problem.  

Not sure if MQ will stop allowing stuff to go ont he queue if they aren't being taken off. Needs investigation.

If not, and hte sender runs a long time without a receiver, at some point you have to bite the bullet and do a cold restart.  Also how long does it take to completely recover? I don't think the countermap ever does.

### Things That are Stubbed or Partially Built
- Build out the offensive word detection mechanism. Check. This is in the tokenizer. There is a file of explicit words. There is also some generic activities, like a minimum token length in config.yaml. 

- The busy words contain a lot of dreck. 
 

### The Analysis Thread
- The analysis thread does cluster but the quality of the clustering is uncertain. 
- This gets complicated, as the parameters in configuration are all just guesses.
- Figure out the parameters that give the best busy words.

   
# The analyze_tokens Program Output

The analyze_tokens program reads CSV files and accumulates the number of distinct tokens that it has encountered.

This is the result of running on about 1200 CSV files which is at least a couple of days of data.  Notice it rises quickly at first, and then settles down to a pretty stead rate of about one new token for every 3.15 Tweets, or roughly every 31.5 tokens.  That rate looks like it holds pretty steady, as this is two days of the Decahose.

It is unknown if the full Firhose would behave differently. It could be that most of the diversity at any given moment is contained in a fraction of the Tweets.

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
- Volume decahose = 500/sec

- Volume firehose = 5000/sec

- 500/sec = 30k Tweets/min = 450,000 Tweets/15min  = 1.8m Tweets/hour

- There are an average of 10 words/Tweet = 4,400,000 words/15 min or 18m words/hour

## Our Processing
- The sender can send about 3000/second

- We only receive about 1200/second, or 2.4 Decahoses.

- The Tweets get ACKed, so rabbitMQ manages to never let the queue get very long--it seems to never get up to 50.
  
- Google says a modern Mac could probably do about 10x as fast as this antique iMac. So, a new machine should handle multiple fire hoses.

- We have about 350 hours of decahose. At 2.5 decahoses, we could process the entire data set on this machine in about 140 hours or a little under six days. Which seems about right--I think it took a more than two weeks to collect it.

# Toy Golang Code for using K-means
Here's a complete, self-contained Go program that demonstrates how to:

    Represent short sentences as feature vectors based on selected important words

    Cluster them using k-means

    Print the cluster assignments

It uses the github.com/muesli/kmeans library.
✅ What This Code Does

    Input: short "sentences", each with 1–5 "important words"

    Output: sentences grouped by k-means cluster

    Uses Euclidean distance, but you can swap in Cosine if needed

🧱 Example Go Program

package main

import (
	"fmt"
	"log"

	"github.com/muesli/clustering/distance"
	"github.com/muesli/kmeans"
)

// Data: sentences and their key words
var data = map[string][]string{
	"I love pizza and pasta":        {"pizza", "pasta"},
	"She likes ramen and sushi":     {"ramen", "sushi"},
	"Cat sleeps all day":            {"cat", "sleeps"},
	"Dogs bark in the park":         {"dog", "bark", "park"},
	"I enjoy Italian food":          {"pizza", "italian"},
	"Japanese food is delicious":    {"sushi", "ramen"},
	"Dog plays fetch":               {"dog", "plays"},
	"Cat chases laser":              {"cat", "chases"},
	"He walks in the dog park":      {"dog", "walks", "park"},
	"She naps beside the cat":       {"cat", "naps"},
}

func main() {
	// Step 1: Build vocabulary
	wordToIndex := make(map[string]int)
	idx := 0
	for _, words := range data {
		for _, word := range words {
			if _, exists := wordToIndex[word]; !exists {
				wordToIndex[word] = idx
				idx++
			}
		}
	}
	vocabSize := len(wordToIndex)

	// Step 2: Build vectors
	sentences := make([]string, 0, len(data))
	vectors := make([]kmeans.Observation, 0, len(data))

	for sentence, words := range data {
		vec := make([]float64, vocabSize)
		for _, word := range words {
			if i, ok := wordToIndex[word]; ok {
				vec[i] = 1.0
			}
		}
		sentences = append(sentences, sentence)
		vectors = append(vectors, vec)
	}

	// Step 3: Cluster
	model := kmeans.New()
	clusters, err := model.Partition(vectors, 3, distance.Euclidean)
	if err != nil {
		log.Fatal(err)
	}

	// Step 4: Output
	for i, cluster := range clusters {
		fmt.Printf("\nCluster %d:\n", i)
		for _, obs := range cluster.Observations {
			// Find sentence corresponding to this observation
			for j, v := range vectors {
				if sameVector(v, obs) {
					fmt.Println("-", sentences[j])
				}
			}
		}
	}
}

// Helper: compare two vectors
func sameVector(a, b kmeans.Observation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

🛠 To Run This:

    Install the dependency:

go get github.com/muesli/kmeans

    Save the program to main.go

    Run it:

go run main.go
