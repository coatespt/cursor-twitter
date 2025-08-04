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


# Running the Program and Utilities

In addition to the main program, there are several programs to parse the original JSON into CVS, run RabbitMQ, send the data, as well as various utilities for analysis, data conversion, etc. This section tells how to run them all.

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

## Tailing the Logs
Nothing goes to standard out except the output. See the pipeline logs for all the diagnostic and performance output.

You can do it manually, but the following program run from cursor-twitter will find the latest one and run tail -f on it.

tail-the-log.sh	

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

You can put the file name in the data/sender/sender_status.txt.<whatever> and copy that file to sender_status.txt for repeatable sender start files.

Note that it takes an argument, N, which is the number of CSV files prior to the one you want. This is because in most cases, you will have to fire it up from scratch in order to have it primed when it gets to the date you seek.  The number of files is a function of how many tokens you specify in config. I've been using 2,000,000, which is about a quarter of a million tweets, divided by 30k tweets in a file should be N=eight or nine files.


Build it:

make build-find-csv

Run it:
./find_csv_file -dir /path/to/csv/files -datetime "2012-02-14 19:35:55" -n 3

The following will return a file where the first "batch_time" is "2012-02-11 16:49:05 UTC".

 ./find_csv_file -dir /home/petercoates/gnip-csv-lang/ -datetime "2012-02-11 23:00:00" -n 15

 starting with file:

 ../../gnip-csv-lang/gnip.csv_1328996644096_1328996944096.csv
 

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

Formerly, it was getting 1.1k Tweets/second on the iMac and a little more than 2.1 on the 76 laptop. Now it is more like 2300 on the iMac and 8000 on the System 76, which is also a four-core machine.  That is 1.6 firehoses.

The feeder is running on the same box feeding CSV via RabbitMQ.

This might be considerably faster if either:
- The feeder were writing to STDOUT and the main reading fro STDIN
- The main were reading the files directly

The latter is very consistent with using this for historical data.

It is also not clear whether the feeder running on another machine with Rabbit writing over the LAN would net slower of faster.

The speedups were a combination of many things that are worth remembering:
- Removing contention caused by unnecessary concurrency protections
- Tightening up encapsulation. Everything is now independent, connected by queues.
- Better memory management, especially getting rid of old 3pK mappings for zero count tokens
- Deduplication of clusters, i.e. scrapping nearly identical Tweets. (A modified verison of Levenshtein distance that works on the word level rather than the character level.)
- Greatly reduced logging and use of a framework that controls how much gets written

## Confirmation of the Z-score Principle
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

## 3pk Collisions
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

Interestingly, doing this seemed to produce a significant bump in speed. From around 1100/sec to 1300/sec on the iMac. Not clear why. It would make sense if it slowed down after hundreds of millions of Tweets, by why quickly?
 
 
## Seemingly Excessive Token Rejection

We were getting huge percentages of rejection of tokens because of too-short token length and/or tokens being on the ingore list. More than 30% and 40% respectively. The list of rejected tokens is in config/token_filters.txt.
 
Those numbers were not supurious. Turning that functionality off gave the expected result and it is consistent with the Ziph distribution. It means both are very effective and doing their job. You definitely want min length = 3.  The two things strip out a huge amount of processing at the very beginning of the pipeline.

## Effect if Computing Cluster Persistence

I went from checking over six batches to checking over 1 batch without producing a huge speedup. Maybe 100/second faster.

### Checking all words v checking busy words when doing cluster persistence checking
This aspect of the problem has NOT been reviewed
 

## Is token splitting on periods, commas, and dashes happening?

No, it was not happening.  The token cleanup in Go now does this. Note that we can optionally split or not split on apostrophes. (See config.yaml) The default is to not split so that O'Brian and M'kele M'beme, and f'oc'sle can continue to be tokens.

## Overhaul of ASCII Output and Reduction of Logs

Logging has been overhauled with logging almost all going to the designated log file and almost none to the screen except some startup info going to stderr.

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

## Possible logic error
We get notification of burst of 3pk's not mapping to tokens. This should be almost impossible (it says so right in the warning message.) 'Sup with that?
May be an artifact of startup.
 
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
  - 44,298,119 distinct tokens in 137 million Tweets, 
  - 135,315,247 distinct tokens in 584,230,000 Tweets

The analyis program is pretty fast, at 52,683 Tweets/sec    


- 599 million Tweets equals approximately six billion tokens.  
 
- Volume of decahose = 500/sec

- Volume of firehose = 5000/sec

- 500/sec = 30k Tweets/min = 450,000 Tweets/15min  = 1.8m Tweets/hour in nominal Tweet time

- There are an average of 10 words/Tweet = 4,400,000 words/15 min or 18m words/hour

## Our Processing Speed
- The sender can send about 3000/second with ACKs. Which you need, or it will simply drop msgs.

- We process about 1200/second, or 2.4 Decahoses.

- The Tweets get ACKed, so RabbitMQ manages to never let the queue get very long--it seems to never get up to 50.
  
- Google says a modern Mac could probably do about 10x as fast as this antique iMac. If true, that's a couple of firehoses.  This is important, because one of the purposes of this is to make it possible to handle historical data. 
  - I'm skeptical that better hardware would get a 10x bump. 
  - The processor on this old machine is actually really fast (4 GHZ), despite being 12 years old. 
  - Any speedups would be because of faster SSD storage, more cores, and improved instruction set. Most likely, the number of cores would be the overwhelming factor. 
  - So a 16 core machine would probably just about do the firehose. 
    - Possibly more if you moved the feeder to a different box.

- Note that for historical data it will scale in direct proportion to the number of machines so you can do bulk processing essentially as fast as you are willing to pay for.

- We have about 350 hours of decahose. At 2.5 decahoses, we could process the entire data set on this machine in less than a week.
 