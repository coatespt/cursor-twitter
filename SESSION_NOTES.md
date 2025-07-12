# Session Notes

## Project Architecture & Workflow

### Cursor Principles 

Never, under any circumstances do any git operation for any reason. Even if you think I have asked you to, don't do it. I will never ask you to do a git operation under any circumstances. Assume you misunderstood me. 

Don't run the program yourself without asking. You keep doing that and then I have to kill the program to get your attention again. 

If you are going to make changes that you cannot rewind without input from me, warn me so that I can commit everything in a working state.

Do not implement anything without explicit approval. Sometimes I just want to talk about an approach. That doesn't mean that I want to rewrite the codebase!

For every feature, we need to add unit tests.  The tests should be carefully commented about what is being tested and why the pass/fail conditions are what they are.

For each code change, we run the tests! 

When a test doesn't pass, we have to look at why before changing anything. No removing tests because they don't pass!

If any changes seem to involve multiple threads, be sure to get my agreement before doing anything. Anytime there thread safety constructs like mutex, etc., always check. We keep getting into trouble with unnecessary and incorrect thread complexity.



### Goal
The project is a way to read the Twitter firehose and find new subjects appearing in the 
Tweet stream. The underlying insight is that it would be essentially useless to 
analyse all the subjects that are active at one time because this number is in the 
tens of thousands--at least 100x too much for a human to grasp.  

Fortunately, new subjects actually arrive at a manageable rate. Depending upon exactly how you define "subject," new subjects arrive perhaps every few seconds. By exposing only the stream of new subjects (and the Tweets they are in) you see a useful, evolving view of what people are talking about. With reasonable parameterization of the definition of "subject" you get something about like the Times Square News Ticker.

To find subjects by characterizing the semantics of thousands of Tweets per second, and then group them together based upon subject likeness would be a daunting task computationally. Certainly it would be extremely difficult to do in real time. Fortunately, however, groups of meaningful words used together are an excellent proxy for semantics. 

Interestingly, a subject is easiest to identify not by grouping based upon the most frequently used words, but by spotting less frequencly used words that being used are anomalously frequently at the moment. 

As the overwhelming majority of words are very rarely used (on the order of zero to a handful of times a day), in practice new subjects are typically characterised by the appearance together of two or three (or more) words that are suddenly being used with unusual frequency.

For this to be helpful, the anomalously frequently used words (called here busy words) must be detected very quickly because it must be practical to analyse the current window of Tweets for clusterings of busy words. To get a fine-grained view, the granularity of the analyisis should be on the order of a few seconds.

Ordinary word frequency computation would be very hard to do sufficiently quickly, but is a poor method for finding words that are suddenly being used unusually often for fundamental reasons. Even at 5000 Tweets/second, it takes quite a while to get enough words to get a meaningful frequency analysis. By the time you have enough words to confidently assign a frequency to each, the window is too large to find rapidly developing subject in.

Therefore we use a probabilistic heuristic that identifies surges in word frequency without doing frequency analysis inline. A global frequency computation is done periodically off line in order to keep up with word usage changes with time of day, day of week, etc., as well as with changes due to the emergence of new subjects, but the main line of analysis does not need do the global computations necessary for relative frequency.

The heuristic detects only the leading edge of surges in frequency. As it is only sensitive to the leading edge of a surge, it automatically forgets words that surge in usage and then stay at an elevated level.

The heuristic is extremely fast--potentially considerably faster than the firehose rate. Thus it is practical not only to do real time processing, but to process historical 
data at reasonable speed.

### Input and Output

The input for development purposes is JSON-formatted Tweets in files that have the file order encoded
in the filenames. The files are GZIP'ed.  In production the input could be either similar files 
(for historical processing) or the actual firehose (for real time processing).

The non-graphical output is a series of clustered set of Tweets that are about new subjects. 
The output is primarily the original CSV Tweets, but the format for indicating clustering 
is still to be decided.

Computing the non-graphical output is the "hard" part. We will ultimately have a front end 
that displays the output in graphical form. We are not yet concerned with that.

### The Approach

The fundamental principle is that "subjects" are best identified by sets of words that
are suddenly appearing with unusual frequency.

Word usage famously has a Zipf distribution, which means that 99% of words are very rarely used. "Rarely" in this context typically means perhaps zero, once, or twice a day. A word that is suddenly appearing unusually frequently we call a "busy word."

Because most words are very rare, literally one-in-a-million in a text stream, if two or 
three busy words appear together in a few Tweets, it's almost certainly a new subject.
 
A word being busy is not about its absolute frequency. It is about it's relative 
change in frequency of use. Very common words, like "the" and "a" contain almost no information.  But surprisingly, somewhat less common words, such as people and place names, often don't carry much either. "Trump", "Beyonce", or "Swift" are used in so many conversations and subjects that they mean almost nothing. 

It is the clusters of less common people's names, place names, and unusual words that tend to identify a new subject. The super-common names tend to get dragged along.

If a hot subject comes along, i.e. people are tweeting and retweeting text that contains
some subset of three or four busy words, the subject will tend to be forgotten after a while because the rate of use that caused them to be marked as busy words slowly becomes the norm for them.

However, in practice, a major subject will keep getting augmented with other unusual but related words, so it keeps the subject fresh beyond the expiration of the original busy words. This is in keeping with our intuition. As a conversation evolves, the original words hang around, but they are elaborated with more and more varied vocabulary.

The key to this working is that the sliding window has to be small, e.g., typically in the range five or ten thousand Tweets. Too wide a window and the granularity of the view erodes. The number of Tweets that words must appear in to be noticed become larger and the resolution goes down. 

So sum up, the algorithm 
- Detects all the currently surging words.
- Finds the Tweets in the current window that use a proper subset of these words
- Discards all the other Tweets
- Clusters the busy-word Tweets by the busy words they use.  This isn't necessarily a unique clustering.

## What's Wrong with Using Frequency Calculations

The trick is, you can't use an ordinary frequency calculation. Frequency calculation
could be modified to show frequency relative to the previous frequencies, but you'd have to do that massive calculation every few seconds in order to keep the Tweet window small. To get meaningful results, you would need to work on a large window--many minutes of Tweets.
 
It would be difficult to execute such a large computation on millions of words every few seconds. You would need to 
- Count the frequencies of all words in the current window of time.
- When the limit of the window is reached, compute the relative frequencies for the words.
- Compare them to the relative frequencies of of the current window to those of previous window
- Derive a list of those words with changes too large to be random chance.

The other problems is that the span of Tweets necessary to get the word statistics is so large that the resolution of "new" subjects is extremely coarse. You can't find subjects until they are quite far from being new.

## The Heuristic

### Word Frequency Computations

We use a periodic computation of global word frequency to divide the universe of words into F frequency classes, with the most frequently used words in the first class, and the least frequent words in the last class. This frequency analysis is done outside of the processing of the stream of inbound Tweets and covers a much larger time span than the seconds required for the heuristic itself. The purpose is to provide a background look at what normal word frequencies are at the particular time of day, day of week, etc.

The global computation is done at a very coarse grain, e.g., say, between fifteen minutes and an hour.The word freqencies are computed and the words are divided into equivalence classes according to frequency. The words in each class will account for approximately equal usage.

The frequency classes are used to construct a series of filters that allow an incoming word's background frequency to be identified. The first few classes contain only a few words, and the last class contains considerably more than all the other classes put together. Any incoming word that doesn't match to any frequency class is necessarily rare, so it is treated as being in the least frequent class.

The word counts, frequency class computation, etc. are done on a separate thread. Behind the scenese it swaps in new frequency filters to be used by the main processing pipeline.

### Receiving Tweets
The main routine reads inbound Tweets in CSV format.
It maintains a queue of the latest W Tweets. (W might be fifteen minutes to an hour's worth of Tweets.)

It parses them, identifies the tokens in the text, and puts the tokens on a queue for the off-line frequency calculations to use.  When Tweets age out of the queue, it puts these on another queue for the the off-line frequency calculations to use to age them out of the frequency counts.

The tokens from the Tweets are divided into the F frequency classes and handed off to F queues for the busy word processors to work on. They are not handed off as Tokens but as three-part-keys 3pk's.

The busy word processors work or shorter windows of Tweets--a few thousand in a window. When a window of tokens have been handed to the busy word processors, th

### Processing the Stream of 

There is a separate pipeline for each of the frequency classes. 

Tweets are processed in a sliding window of width N.  

The text of each incoming Tweet is tokenized into words, numbers, etc.

Each word in each Tweet is tested for which frequency class it is in and handed off the one of
F parallel computations as follow.

Each word is hashed three different ways to produce a "three-part-key", i.e., an 
ordered triple of integer hashes. 

The hashes are used to index into three arrays of C counters. (Array size is configurable, but
it would be something like 1000 to 5000 for reasons that will be clear later.)

If every word had the same probability, the counters would be approximately equal.  However, we
are assuming that some are anomalously busy.  Therefore, some of the counters will have exceptionally
high counts.

When N Tweets have been fed in, the following happens with of the parallel computations.

A Z score is computed for each counter. The Z reflects the improbability of a count having been
reached at random.

An arbitrary z is set in configuration. This z says how unlikely the count must be to be considered
freaky high.

The indexes of the high Z score counters are collected for each of the counter arrays.

The Cartesian product of the index sets is computed. This gives you a set of three-part keys,
some of which correspond to actual words, and the others are just junk.  

Check each against the legitimate three part keys that is maintained (as described above)

The keys that correspond to actual words give you the busy words for the current window.

Only one window worth of Tweets is maintained, so hereafter, for every incoming Tweet, the oldest Tweet must be discarded.

This means that all it's busy word indexes must be removed from the F counter arrays to keep them current.  

So no matter how many windows you process, the counts are always correct.

### Details

In addition to the pipleline of Tweets being broken into words and three part keys, some other tasks
are done in the background.

Every word coming in should be checked for whether it has been seen before. If not
its three part key should be computed and recorded in a lookup table (and also to a database.)

The frequency class a word belongs to has to be computed by checking for its registration in 
a series of F Bloome filters.

To produce these bloome filters, an ongoing computation of global word frequency must be 
maintained. This doesn't happen as frequently as the processing window (which is on the order of a few seconds), but it has to be done frequently to correct for the ongoing time-of-day changes to
word usage as well as the results of big surges from major events.  So it's more like every ten minutes to an hour.  Configurable.

Every time the global stats are recomputed, the frequency class filters must be recomputed and
replaced.

### Output display

The output in text form is OK for starters. 
Going forward, we need a graphical display.

CONFIG FILE POLICY
There must be only one authoritative config file for the application.
The config file must be located at: cursor-twitter/config/config.yaml
Do not create or use any other config files (including backups, test configs, or files in other directories).
All code, scripts, and documentation must reference this single config file.
If you need to test changes, temporarily copy or rename the main config, but always restore the single-source-of-truth location.
If you see a config file anywhere else, delete it or move its contents into the main config.



## Current Implementation (WORKING FOUNDATION)

**We have a working, high-throughput pipeline with three essential components:**

1. **Parser (Pythong)** - JSON → CSV conversion (scaffolding)
   - Reads JSON input files
   - Converts them to CSV format
   - Writes CSV files to an output directory

2. **Sender (Python, `send_csv_to_mq.py`)**
   - Reads all CSV files in the output directory (in sorted order)
   - Sends each CSV row as a message to the RabbitMQ queue `tweet_in` on `localhost:5672`
   - Uses the raw CSV row as the message body (no extra formatting)
   - **Status: WORKING** - Successfully "slamming messages through" at high throughput

3. **Receiver (Go, `main.go`)**
   - **Simple, blocking consumer** that connects to RabbitMQ
   - Hardcoded connection to `localhost:5672` with guest/guest credentials
   - Hardcoded queue name `tweet_in`
   - Prints messages as they arrive
   - **Status: WORKING** - Successfully receiving and displaying messages in real-time

**Files in `src/`:**
- `main.go` - Simple Go receiver with blocking consumer (WORKING)
- `rabbitmq.go` - RabbitMQ connection logic (simplified, not currently used)
- `send_csv_to_mq.py` - Python sender (WORKING)

**Test Results:**
- ✅ **Pipeline tested end-to-end** with real CSV files
- ✅ **High throughput achieved** - "slamming messages through"
- ✅ **No bottlenecks** - blocking consumer handling load efficiently
- ✅ **Real-time capability** - ready for Twitter firehose rates

## Things Known To Be Wrong 

## Useful Commands

- ** Build and Run the Golang JSON->CSV Parser--

WE HAVE NOT BEEN ABLE TO GET THIS WORKING RIGHT. SO WE'RE TRYING PYTHON>

cd /Users/petercoates/python-work/cursor-twitter
go build -o parser/parser parser/parser.go
./parser/parser -inputdir ../twits/msg_input/ -outputdir ../twits/test_output

- ** Python JSON->CSV parser.
cd to cursor-twitter/sender.

Run the following.

python ./send_csv_to_mq.py ../../twits/msg_output_2

- ** Test program for parsed data
This program reads CSV files to ensure that we can create Tweets from them.

go run tests/csv_tweet_parse_test.go <path_to_your_csv_file>

## TODO / Next Steps


- Useful Comments
Let's make a practice of saying what each module does
at the top of the file.  Comment as if I'm an idiot.
   
 

- Check that 'sorted order' for the file names is the same as the order given by the times embeddede in the file names.

- Clean up the excessive logging and print outs

- Make tests around everything.

- Build out the offensive word detection mechanism. Check. This is in the tokenizer. There is a file of explicit words. There is also some generic activities, like a minimum token length in config.yaml. 

- Find the Tweets that the set of busy words applies to and do the clustering.

### Biggest Thing to Do: The Main Counts/Frequency Mechanism Is Too Short a Window.

The low quality of busy words we are computing is probably in part because the frequency counters are working over a tiny window. I think there should be millions of Tweets--like an hour's worth or more--in the window.

Right now it's something like 300k. It should probably be in the millions. However, each million Tweets is probably at least a gigabyte of memory. 

#### Some facts and assumptions  
  - A file is typically about 100k Tweets. So if a window is in the right ballpark of size it will represent many file, e.g. 10 to 30 files.
  - This is too long to wait for startup, and a lot of Tweets too keep in memory. Figure a Tweeit is probably a kilobyte, that's gigabytes of memory.
 

#### Proposed new design
  There is a tiny element of cheating in this compared to how it would have to work in real life, because in real life, the stream of Tweets would keep flowing even if the stored data structures were way out of date. But it is perfectly legit for handling historical data. 
 
 The scheme is this:

  - There is a queue of tokens fed by main and read by FCT. Unlke now, there is no stream of age-out tokens. The FCT will manage aging tokens out by storing N tokens on disk in uniform size bite-size files, say, 50k.

  - The FCT will also store the accumulated count data, frequency filters, and 3pk to token mappings on disk.  If there is a set present when it starts, it will read them in before main processing can start (other than to send tokens on the queue.)

  - Main can't run until the FCT has installed a set of filters. But it can receive Tweets. Until the filters are installed, it just puts each incoming Tweet on the queue. It can not  pass tokens to the busy word queues without the filters.

  - When the FCT starts:
    - It looks for existing files of counts, frequency filters, and 3pk mappings.
    - If it finds them, it loads the counter array, frequency filters, and 3pk mappings from disk.
    - If it doesn't find the files on disk:
      - It reads Tweets put on a queue by main until it has read enough to build those data structures. This takes a long time. At 3000 Tweets/second, that's five to seven minutes per million.
      - Note that it is continually logging all the tokens it reads fromthe queue to the tokenlog files at say, 50 or 100k per file (configureable.)
    -When it has received enough Tweets and built the datastructures it:
      - Writes the data structures to disk
      - installs the filters for main to use, which allows main to start feeding the busy word filters.

  - FCT is always on a loop to read incoming tokens from the queue unless it is building the datastructures, in which case, the tokens just build up on the queue until it is able to consume them. 

  Note, unless it's a fresh start, there should be counts, frequency filters, 3pk, and N 50k files of tokens.  
  
  There will also be a file indicating the last file read by the sender. That file should only be of concern to thesender.
   
  - When the system starts, main main always starts writing every inbound Tweet to the queue read by the FCT.

  - The FCT may or may not have created the filters yet. Probably not because it takes a couple of minutes to build them.  Until the filters are ready, main doesn't put tokens on the busyword queues. All it does is write the tokens to the new token queue for the FCT. 

  - Once the frequency filters are present, it can start feeding the busy word processor queues.

  - The Sender always logs the latest file it has processed in a disk file. This tells it where to pick up sending Tweets next time it starts. It will start with the next file after that when it runs again. 

  In steady state, the FCT is 
  - Registering all the incoming tokens in the counter map
  - Periodically writing a 50k file to disk.
  - Periodically re-creating the filters and writing the datastructures to disk.
  
  Note that in steady state, every time it writes a tokenlog file, it also read the oldest one in and deletes the file from disk. The tokens from the file are then used to decrement the token counts. 

  The recomputation of data structures should happen on the tokenlog boundaries, but not necessarily on every boundary.
  - Tokenlog files might be written, say every 100k tokens, which would be about 10k Tweets worth.
  - The recomputation interval might be every millon tokens, i.e., every 100k Tweets.
  - These numbers are totally made up. The exact values will be in config.yaml.


  #### Orderly Shutdown
  There should be a way to set a flag of some kind, e.g., a disk file, to tell the sender and the processor to shut down in an orderly way. This is a little tricky because both sender and FCT are probably mid cycle. 

  - The sender should check for the shutdown flag file before opening the next CSV file. If the flag is set, it quits.

  - The FCT should check for the flag whenever it finds the input queue empty. It could be empty because of a lull in sending, but usually if there are no Tweets to pick up, it will be because the sender stopped sending.  
  
  - When it finds the queue empty and the shutdown flag set, it will always be in the Tweet-reading part of its cycle, not the recomputing part. It will then should then write all the files to disk and alert the main loop to shut down.

  - In this way, when processing starts again
    - The sender will be able to read its record of the last file consumed, so it knows where to start sending. This could be thousands of files into the dataset.

    - The FCT will see it's files and load them. 

    - By the time it has got the data structures built, there may be a lonq queue of Tweets on its queue, but that's ok. 

    - It might be good to have the sender check for the presence of the files that the FCT writes. 
      - If they are present,it means that the FCT will be spending some time loading the datastructures, and therefore it could delay for some number of seconds before sending anything.
      - If they are not present, it means nothing can happen until the processor receives millions of Tweets, so start sending ASAP.

## Features of new Window Processing implented
- Sender logging latest file processed is working
- We have a function to write out the data structures to disk.
- We have a function to read the data structures from disk.
- The above are about to be tested with a command line flag to turn on a test.
- 

## Notes On Dealing with Cursor
- If you start a new session, paste relevant parts of this file into the chat to restore context for the AI assistant.

- Always check that files created in Cursor are saved to disk (visible in Finder) to avoid data loss.

- Every time you get to a settled point, commit it all to git.  Cursor will very likely fuck it all up, so do it frequently.

- I'm using git branches and somewhat less frequently, pulling them back into main.

 
# Volume Issues
-- Volume decahose = 500/sec

-- Volume firehose = 5000/sec

-- 500/sec = 30k Tweets/min = 450,000 Tweets/15min  = 1.8m Tweets/hour

-- There are an average of 10 words/Tweet = 4,400,000 words/15 min or 18m words/hour

- We're sending about 761,674 in 3m55sec  = about 3242 Tweets/second INTO MQ

- We're processing a bit less than that--2500 Tweets/second

- Not sure what happens if this goes on indefinitely. Does MQ start to throttle input?
  
- Google says a modern mac could probably do about 10x as fast. So, multiple fire hoses.

- We have about 350 hours of decahose.  Processing at three decahoses, that's about 
117 hours, or round the clock for five days.


  
# Building and Running Everything
    

The codebase is in ~/python-work/cursor-twitter. Data that it writes to persist data structures, etc. is in ~/python-work/data, i.e., at the same level as the project root.

## Creating the CSV from GZ files
The CSV is created as follows. This reads GZ files from msg_input and writes them as  CSV files in msg_output
 

### This way of parsing the JSON to CSV seems to work very well.
 python3 parser/parser.py /Users/petercoates/python-work/twits/msg_input_2 ../twits/msg_output_2

## Starting RabbitMQ

RabbitMQ feeds Tweets in CSV form to the processor. The service normally running  as a daemon which in theory should be started as follows.

brew services start rabbitmq  

However, this doesn't seem to work right, so just ask Cursor to start rabbit in Docker.

 
## Sending Tweets
Running the actual software assumes you have a directory of CSV files of Tweets, in this case msg_output for the small set and msg_output_2 for several million Tweets.

 python ./send_csv_to_mq.py ../twits/msg_output
 


##  Build and run the main program

Note, this assumes you are in the root. Sometimes cursor wants to run it from src which
is not right. It is better to build and run from the root directory.

cd /Users/petercoates/python-work/cursor-twitter
go build -o main src/main.go 
./main  -config ./config/config.yaml -print-tweets=false

## Shutting down the main

Control-C will send a signal. If the process of writing persisted data to disk is underway it will finish before it shuts down. Otherwise it shuts down immediately.

## The analysis program
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

 

# Processing Tweets
 
The processing may start from scratch or it may pick up where it left off.

The offline frequency calculation thrad (FCT) maintains a map of input tokens (words) to the count of times they have been seen in the last N tokens read. N is typically in the millions. This is used to prepare a set of filters used to determine the frequency class an input token is in.

This counter map takes quite a while to build, so it is persisted to disk and read read in on startup if it is available. If it is not available, it is built from scratch. This can take many minutes.

The counter map needs to be kept fresh, so old tokens are aged out as new tokens are fed in. As this could involve hundreds of thousands of tweets, batches of tokens are written to disk. 50k tokens might be a typical batch size, which would be 20 files per million. In steady state, as each new file gets written the oldest file gets read in and used to decrement the token counts. (The file is then deleted.)
 
Thus, the set of tokens in the token counter map is kept fresh with respect to the advancing day (people Tweet differently before breakfast than they do at midnight) and it is kept fresh with respect to changing is subject matter of Tweets.

## The FCT
The frequency computation thread manages all of the above. It operates almost entirely independently of the main processing, except that it reads a queue of tokens fed to it by the main processor as it reads in new Tweets.

Periodically (the period is configured) the FCT creates the data structures necessary to divide the incoming stream of tokens into F frequency classes, from most-frequently used to least frequently used. Each of the classes represents approximately the same amount of word usage. Therefore in the most frequently used class, there are just a few words, while in the least frequently used class there can be millions.

These filter data structures are prepared in the background and swapped in for the main processor's use. The main processor has no knowledge of how they get there, or indeed, that the FCT exists at all.

## Reading in the Tweets
The main rountine reads a stream of tweets in CSV format from RabbitMQ.

It tokenizes the words in the Tweet text, making them all lower case, filtering out garbage and some of the more extreme offensive words (variations on the "n" word are incredibly common) as well as URL's and some other obvious junk.

Each token is tested against the 3pk lookup table to see if a 3pk key has been computed for it. This data structure maps tokens to 3pk's and 3pk's to tokens.

Note: A 3pk is a three-part key, which is an ordered triple of hashes of the token. Each hash is different, so  you get three pseudo-random numbers in the range of 0 to a configured number which is typically about 1000. This value is also used in the busy-word processing threads.

The tokens are put on a queue for the FCT to consume, and then passed along.

If the filters have been prepared (which is the usual case except for at the beginning)
-  Looks up a 3pk for for the token. If it doesn't find on, it creates one and stores it in the lookup table for next time.
- Testing the token with the filters prepared by the FCT, it chooses one of F queues to put the corresponding 3pk on.

Then it goes and reads the next Tweet.

## Busy Word Queues and Processors.
THe F queues that the main assigns tokens to correspond to the F frequency classes. Each queue is read by an instance of a busyword processor running on its own thread.

The busyword processor are each built around three arrays of counters. These arrays, typically about 1000 counters wide. The exact size is the same as the range of th 3pk hash values.

The processor takes each of the three values in an incoming 3pk, and uses them to increment the counter at the corresponding counter array and index. 

Every time a batch value is reached (a batch is five to ten thousand) the main puts a special valued 3pk on each queue (-1, -1, -1).  This tells the busy word processors to stop reading the queue and compute statistics on the counters.

A Z score is computed for each value on each counter. (Z is just a normalized form of the standard deviation of the count with respect to the rest of the array.) If the 3pk's coming in were truly representative of the same frequency class, the counters would all have approximately the same counts. 

But they don't. We're assuming that busy words will over count some of the counters. Which is why they are "busy" words.

The mean Z score is by definition 0. The divergence of a Z from zero indicates that the counter is getting hit harder than it would be if all the words were of approximately the same frequency.

A min Z score is configured. Say it's value is z.  For any Z>z, the index is selected, given a small set of busy counters.  Each processor has three sets of {Z|>z}, so you have three sets of indexes.

The Cartesian product of the indexes is taken, giving a set of triples of indexes. Three of these values is exactly what a 3pk is.  So you have a fairly large set of these 3pk's, most of which are spurios, but some of which correspond to tokens that are actully in the token->3pk lookup.

The spurious 3pk's are discarded, and the ones that are legit are collected and passed on to another queue to be processed further.

When this process is done, each thread hits a "barrier."  When all threaads have reached he barrier, they can wipe their counters and go back to collecting more 3pk data.

## Finding and clustering the Relevant Tweets

NOT YET IMPLEMENTED.
### Next TO DO
- We still need a window of recent Tweets in memory. I think this has been removed.  It needs to be at least batch Tweets long, and the oldest need to be aged out just as before.  This will be used to find the Tweets the busy words correspond to.
- The busy word processors need to put their 3pk's on queues to be collected by the final stage, that analyzes the recent Tweets, identifies the subjecs, and groups the Tweets that use them
- The objects that the processors put on the queues need to identify the batch, their F, and the list of 3pk's.
- That analysis thread needs to read the queues from the busyword processors. We can start by having it simply print out the collected words. What it will actually do is:
  - Gather the tokens from one batch.
  - Find the recent tweets that use at least k of them.
  - For starters, printing out the Tweet texts is good enough. 

# Things to Do/Fix
- We start the recomputation of frequency data after a certain number of file writes. But this isn't necessary when you have just read in the stored counts.  It wastes a lot of time waiting for half a window to go by when it could create the frequency data structures immediately.
-

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

 
 
