# Session Notes

## Project Architecture & Workflow

###
Absolute priority number one directive to Cursor AI. DO NOT IMPLEMENT ANYTHING, NOT ONE SINGLE LINE, WITHOUT EXPLICITLY ASKING ME AND GETTING AN AFFIRMATIVE RESPONSE. LITERALLY HALF THE TIME I HAVE USED YOU HAS BEEN SPENT UNDOING CRAZY SHIT YOU DID WITHOUT ME ASKING YOU TO. THE POLICY IS ONE STEP AT A TIME AND ASK FIRST.

### Goal
The project is a way to read the Twitter firehose and find new subjects appearing in the 
tweet stream. The underlying insight is that it would be essentially useless to 
analyse all the subjects that are active at one time because this number is in the 
tens of thousands--at least 100x too much for a human to grasp.  

Fortunately, new subjects actually arrive at a manageable rate. Depending upon exactly how you define "subject," new subjects arrive perhaps every few seconds. By exposing only the stream of new subjects (and the tweets they are in) you see a useful, evolving view of what people are talking about. With reasonable parameterization of the definition of "subject" you get something about like the Times Square News Ticker.

To charcterize the semantics of thousands of tweets per second, and then group them together based upon subject likenss would be a daunting task computationally. Certainly it would be extremely difficult to do in real time. Fortunately, hower groups meaningful words used together are an excellent proxy for semantics. 

Interestingly, a subject is easiest to identify not by grouping based upon the most frequently used words, but by using words that being used are anomalously frequently at the moment. 

As the overwhelming majority of words are very rarely used (on the order of zero to a handful of times a day), in practice new subjects are typically characterised by the appearance together of two or three (or more) words that are suddenly being used with unusual frequency.

For this to be helpful, the anomalously frequently used words (called here busy words) must be detected very quickly because it must be practical to analyse the current window of tweets for clusterings of busy words. To get a fine-grained view, the granularity of the analyisis should be on the order of a few seconds.

Ordinary word frequency computation would be very hard to do sufficiently quickly, but is a poor method for finding words that are suddenly being used unusually often for fundamental reasons. Even at 5000 Tweets/second, it takes quite a while to get enough words to get a meaningful frequency analysis. By the time you have enough words to confidently assign a frequency to each, the window is too large to find rapidly developing subject in.

Therefore we use a probabilistic heuristic that identifies surges in word frequency without doing frequency analysis inline. A global frequency computation is done periodically off line in order to keep up with word usage changes with time of day, day of week, etc., as well as with changes due to the emergence of new subjects, but the main line of analysis does not need do the global computations necessary for relative frequency.

The heuristic detects only the leading edge of surges in frequency. As it is only sensitive to the leading edge of a surge, it automatically forgets words that surge in usage and then stay at an elevated level.

The heuristic is extremely fast--potentially considerably faster than the firehose rate. Thus it is practical not only to do real time processing, but to process historical 
data at reasonable speed.

### Input and Output

The input for development purposes is JSON-formatted tweets in files that have the file order encoded
in the filenames. The files are GZIP'ed.  In production the input could be either similar files 
(for historical processing) or the actual firehose (for real time processing).

The non-graphical output is a series of clustered set of tweets that are about new subjects. 
The output is primarily the original CSV tweets, but the format for indicating clustering 
is still to be decided.

Computing the non-graphical output is the "hard" part. We will ultimately have a front end 
that displays the output in graphical form. We are not yet concerned with that.

### The Approach

The fundamental principle is that "subjects" are best identified by sets of words that
are suddenly appearing with unusual frequency.

Word usage famously has a Zipf distribution, which means that 99% of words are very rarely used. "Rarely" in this context typically means perhaps zero, once, or twice a day. A word that is suddenly appearing unusually frequently we call a "busy word."

Because most words are very rare, literally one-in-a-million in a text stream, if two or 
three busy words appear together in a few tweets, it's almost certainly a new subject.
 
A word being busy is not about its absolute frequency. It is about it's relative 
change in frequency of use. Very common words, like "the" and "a" contain almost no information.  But surprisingly, somewhat less common words, such as people and place names, often don't carry much either. "Trump", "Beyonce", or "Swift" are used in so many conversations and subjects that they mean almost nothing. 

It is the clusters of less common people's names, place names, and unusual words that tend to identify a new subject. The super-common names tend to get dragged along.

If a hot subject comes along, i.e. people are tweeting and retweeting text that contains
some subset of three or four busy words, the subject will tend to be forgotten after a while because the rate of use that caused them to be marked as busy words slowly becomes the norm for them.

However, in practice, a major subject will keep getting augmented with other unusual but related words, so it keeps the subject fresh beyond the expiration of the original busy words. This is in keeping with our intuition. As a conversation evolves, the original words hang around, but they are elaborated with more and more varied vocabulary.

The key to this working is that the sliding window has to be small, e.g., typically in the range five or ten thousand tweets. Too wide a window and the granularity of the view erodes. The number of tweets that words must appear in to be noticed become larger and the resolution goes down. 

So sum up, the algorithm 
- Detects all the currently surging words.
- Finds the tweets in the current window that use a proper subset of these words
- Discards all the other tweets
- Clusters the busy-word tweets by the busy words they use.  This isn't necessarily a unique clustering.

## What's Wrong with Using Frequency Calculations

The trick is, you can't use an ordinary frequency calculation. Frequency calculation
could be modified to show frequency relative to the previous frequencies, but you'd have to do that massive calculation every few seconds in order to keep the Tweet window small. To get meaningful results, you would need to work on a large window--many minutes of tweets.
 
It would be difficult to execute such a large computation on millions of words every few seconds. You would need to 
- Count the frequencies of all words in the current window of time.
- When the limit of the window is reached, compute the relative frequencies for the words.
- Compare them to the relative frequencies of of the current window to those of previous window
- Derive a list of those words with changes too large to be random chance.

The other problems is that the span of Tweets necessary to get the word statistics is so large that the resolution of "new" subjects is extremely coarse. You can't find subjects until they are quite far from being new.

## The Heuristic

### Word Frequency Computations

We use a periodic computation of global word frequency to divide the universe of words into F frequency classes, with the most frequently used words in the first class, and the least frequent words in the last class. This frequency analysis is done outside of the processing of the stream of inbound tweets and covers a much larger time span than the seconds required for the heuristic itself. The purpose is to provide a background look at what normal word frequencies are at the particular time of day, day of week, etc.

The global computation is done at a very coarse grain, e.g., say, between fifteen minutes and an hour.The word freqencies are computed and the words are divided into equivalence classes according to frequency. The words in each class will account for approximately equal usage.

The frequency classes are used to construct a series of filters that allow an incoming word's background frequency to be identified. The first few classes contain only a few words, and the last class contains considerably more than all the other classes put together. Any incoming word that doesn't match to any frequency class is necessarily rare, so it is treated as being in the least frequent class.

The word counts, frequency class computation, etc. are done on a separate thread. Behind the scenese it swaps in new frequency filters to be used by the main processing pipeline.

### Receiving Tweets
The main routine reads inbound Tweets in CSV format.
It maintains a queue of the latest W Tweets. (W might be fifteen minutes to an hour's worth of Tweets.)

It parses them, identifies the tokens in the text, and puts the tokens on a queue for the off-line frequency calculations to use.  When Tweets age out of the queue, it puts these on another queue for the the off-line frequency calculations to use to age them out of the frequency counts.

The tokens from the tweets are divided into the F frequency classes and handed off to F queues for the busy word processors to work on. They are not handed off as Tokens but as three-part-keys 3pk's.

The busy word processors work or shorter windows of Tweets--a few thousand in a window. When a window of tokens have been handed to the busy word processors, th

### Processing the Stream of 

There is a separate pipeline for each of the frequency classes. 

Tweets are processed in a sliding window of width N.  

The text of each incoming tweet is tokenized into words, numbers, etc.

Each word in each tweet is tested for which frequency class it is in and handed off the one of
F parallel computations as follow.

Each word is hashed three different ways to produce a "three-part-key", i.e., an 
ordered triple of integer hashes. 

The hashes are used to index into three arrays of C counters. (Array size is configurable, but
it would be something like 1000 to 5000 for reasons that will be clear later.)

If every word had the same probability, the counters would be approximately equal.  However, we
are assuming that some are anomalously busy.  Therefore, some of the counters will have exceptionally
high counts.

When N tweets have been fed in, the following happens with of the parallel computations.

A Z score is computed for each counter. The Z reflects the improbability of a count having been
reached at random.

An arbitrary z is set in configuration. This z says how unlikely the count must be to be considered
freaky high.

The indexes of the high Z score counters are collected for each of the counter arrays.

The Cartesian product of the index sets is computed. This gives you a set of three-part keys,
some of which correspond to actual words, and the others are just junk.  

Check each against the legitimate three part keys that is maintained (as described above)

The keys that correspond to actual words give you the busy words for the current window.

Only one window worth of tweets is maintained, so hereafter, for every incoming tweet, the oldest tweet must be discarded.

This means that all it's busy word indexes must be removed from the F counter arrays to keep them current.  

So no matter how many windows you process, the counts are always correct.

### Details

In addition to the pipleline of tweets being broken into words and three part keys, some other tasks
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

1. **Parser (Go)** - JSON → CSV conversion (scaffolding)
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
cd to cursor-tweet
python3 parser/parser.py /Users/petercoates/python-work/twits/msg_input ./test_output


- **Build the Go receiver:**
  ```bash
  cd cursor-twitter/src
  go build -o process main.go
  ```
- **Run the receiver:**
  ```bash
  ./process
  ```
- **Run the sender:**
  ```bash
  python src/send_csv_to_mq.py /path/to/csv/files
  ```
- **Install pika (Python RabbitMQ library):**
  ```bash
  pip install pika
  ```



## TODO / Next Steps
- Clean up the excessive logging and print outs
- Add the offensive word detection. Read in a file of words to ignore. Niggah, niggaaazz, etc.
- Find the right data structure to output the set of busy words too. It need the words by frequency class and the ID's of the tweets.  Or some way to identify the tweets that the batch of words pertains to.
- Fix the feeder. It doesn't seem to be working on the big set of files.  We need a jumbo set of tweets to run on.
- Find the tweets that the set of busy words applies to and do the clustering.




## Notes
- If you start a new session, paste relevant parts of this file into the chat to restore context for the AI assistant.
- Always check that files created in Cursor are saved to disk (visible in Finder) to avoid data loss.
- We're building incrementally now - simple foundation first, then add complexity step by step.
- **Current status: SOLID FOUNDATION WORKING** - ready to add processing logic incrementally. 
- Commit to a branch and merge the branch back to main periodically

- Volume Issues
-- Volume decahose = 500/sec
-- 500/sec = 30k tweets/min = 450,000 tweets/15min  = 1.8m tweets/hour
-- Say, an average of 10 words/tweet = 4,400,000 words/15 min or 18m words/hour

- We're doing about 761,674 in 3m55sec  = about 3242 tweets/second INTO MQ
- We're processing them about 2500 a second (Which is freakin' fast! Half a fire hose.)
- The firehose is about 5700/second.  
- Google says a modern mac could probably do about 10x as fast. So, multiple fire hoses.

 

# TTD
 
## Long Window
The window is so small that many relatively common words don't get a chance to appear and thus get misinterpreted as busy.

- We need a test routine to find out how the number of distinct tokens grows with the number of tweets read in. There are about 10 tokens per tweet, but we have no idea how fast the number of distinct tokens grows.

I think we are having a problem in that the number of tweets represented in the counter array is too small. Even relatively common words haven't had time to show up, and so get classified as being in the wrong frequency category.

The universe of words in a few days of tweets is something like 20,0000 distinct words. That should be an approximate baseline against which we run.

But we don't want to read many millions of tweets in order to start up.

- We need two window sizes
-- The window should hold perhaps a tweet-time hour of tweets. At 500/second for the decahose, that's 1.8 million.  
-- The recompute time should be smaller--hundreds of thousands.
-- Every time a recomputation is done, it should write the contents of the counter array to disk, as well as the range of files from which it was derived.
-- The counter array should be read in to give a large base to start on. 
-- On startup, Tweets in the file range should be read into the window. This can be done at about 3000/second. That means 20 minutes per million. Whew, that's slow.
-- 
-- The processing should pick up with the next file after the window of Tweets represented by the 
   
## Comments
Let's make a practice of saying what each module does
at the top of the file.  Comment as if I'm an idiot.

## CSV Sender
The CSV Sender has a problem. It's choking on the files in the big repository.
 
## Trashy Words
Need a file of racial slurs, foul language, etc. that the tokenizer or other phases of processing can use to omit filth
  
 
  
# Building and running the main program
go clean -cache -modcache    (if necessary)
./process  -config ../config.yaml -print-tweets=false
   
# Running Notes

The code is in ~/python-work/cursor-twitter
 
## Sending Tweets
Running the actual software assumes you have a directory of CSV files of tweets, in this case msg_output.

 python ./send_csv_to_mq.py ../twits/msg_output

 If MQ is not running, you get a messy failure. It should be restartable with the following. It is
 possible it might need sudo prepended.
 
  brew services start rabbitmq


## Creating the CSV from GZ files
The CSV is created as follows. This reads GZ files from msg_input and writes them as  CSV files in msg_output

cd /Users/petercoates/python-work/cursor-twitter/parser
go build -o tweetparser parser.go 
./tweetparser -inputdir ../../twits/msg_input  -outputdir ../../twits/msg_output 

## Starting RabbitMQ

RabbitMQ is a service normally running  as a daemon which could be started as follows.

brew services start rabbitmq  

However, this doesn't seem to work right, so just ask Cursor to start rabbit in Docker.


##  Build and run the main program

cd /Users/petercoates/python-work/cursor-twitter/src
go build -o process main.go
./process  -config ../config.yaml -print-tweets=false

## Tests
cd /Users/petercoates/python-work/cursor-twitter
make build
make run-consumer
make run-sender
make test-verbose

## The analysis program
cd to cursor-twitter
go build -o analyze_tokens analyze_tokens.go 
./analyze_tokens -input ../twits/msg_output

# Development Plan

## Some Concerns to Look Out For
 
## Make sure Cursor doesn't lose its mind and change the architecture. Check code into git often.


## Counts and frequencies are computed by a FrequencyComputationThread FCT


## There is a potential issue here if the frequency computations can't keep up with the pipeline processing tweets. If the global calculations are temporarily off by a cycle of two or three off, it's not a big problem, but if the pipline is consistently faster
- The size of the unprocessed backlog on the InboundTokenQueue and OldTokenQueue grow without bound.
- The creation of fresh frequencyfilters will get more and more out of sync with the tweet stream. 
- The former is a memory consumption problem. The latter will degrade quality, as words that have recently become more frequent pollute the streams of lower-frequency 
  frequency classes. 
 

## Main processing pipeline.

## Using the Frequency Class Filters
-  The F frequency class filters described in the previous section are used to determine what class word is in. 
-  Its 3pK's are identified from a global lookup table. If a token doesn't have a 3pk registered, it is created and added to the global table.  Important note: The hashes in the 3pk's are written modulo the bw_word_len which is also the length of the processor arrays in the busy-word processor threads.
- the 3pk is passed into the appropriate kth frequency class pipeline.

## The Core Token Processing Structures
The heart of a processing pipeline are the busy-word threads. There is one for each freqency class.
- Each 3pk processor is centered around a data structure with three arrays of counters. The arrays are all of the same size.
- The size (3pkCountArraySize) is configurable, but experience suggests about 1000 is a good balance. As above, these arrays of of the same size as the range of the 3pk values, i.e. bw_word_len
- Each 3pkProcessor reads 3pk's from a queue fed by the main routine. (The 3pk's are put on the queue by the main processing thread.)

Each of the three elements of the 3pk is used to choose the index of a counter to increment. That counter in the corresponding array is bumped.

After a window of width batch, (batch is typically a small fraction of WindowSize, perhaps a few thousand tweets) its counters will be have been incremented by a total of approximately the average number of tokens in a tweet multiplied by batch and divided by the number of frequency classes.

When batch number of tweets have been processed by the main thread and their 3pk's sent to the 3pk processors, the main thread sends a special 3pk to all 3pK processors to tell them to stop reading 3pk's and process the batch. E.g. a 3pk with all three values equal to -1?  

The reason for this, and not a flag, is to keep them synchonized, as the signal 3pk's would be injected into the queues at the same time. Therefore each processor would keep draining its queue until it hits the signal.

To process a batch:
- The mean and standard deviation of the distribution of counts is compute and from those, a Z score is obtained for each element of the array of counts.
- The counts with Z scores above a configured Z min are are isolated for each counter array.
- The counteres can now be wiped to zero.
- The Cartesian product of the three sets is created. These are 3pk's of which the great majority will be spurious.
- The computed keys that match to known keys in the central lookup of keys to tokens represet presumed busy words. 
- The F sets of busywords are merged and passed on to the next stage of processing (clustering the Tweets.)
- As the processors complete their work, they register with a "barrier."  When all have registered, the barrier allows them to restart their 3pk reading loop.

### Notes on the Computation
Given three sets of indices of counters that pass the Z test We take the Cartesian product of the three sets 

Most of these will not corrrespond to existing 3pk's. Consider why. If you actually had K busy words producing high index values in each array, you'd have k^3 combinations, and all but about k/k^3 of them would be spurious. 

For example if there are 25 legit busy words, that's 15,625 combination of which approximately 15,600 are spurious. 
- Note that if the size of the counter arrays is 1000, there are a billion possible 3pk's, but probably only about 20 million distict tokens appear in a day of tweets, which means about 1 in 50 random 3pk's would correspond to a real token. 
- That one in fifty would indeed be spurious, but while 1 in 50 sounds high, most of them will never actually appear even once in any of the few thousand Tweets considered in an interval. This is because almost all words are rare.
- When a spurious word does appear in a Tweet or two, so what? It is unlikely to appear in 
the same Tweets as other busy words and even if it does, it is not likely to appear again. Therefore, very little harm comes from a reasonable spurious busy-word rate. 
  
##  What Happens with the busy words.

- The busy words produced by all the 3pk processing pipelines are grouped together. 
- The set of batch Tweets that produced those words are examined, and all Tweets that don't contain tokens matching the 3pk's produced by the pipelines are discarded.
- Depending upon configured values, 1, 2 or more busy words appearing together might be considered to represent a subject. The best value will be determined empirically.
- A clustering algorithm is applied to the Tweets that contain a set of busy words, grouping them together based upon the subset of busy words they contain. Details TBD.

These clusters are the output of the main processing pipeline.


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

## How to Create Short Lived Git Branches so Cursor Doesn't Trash Yo Shit

### Start From Main branch
git checkout main
git pull origin main


### Create and Switch to New Branch
git checkout -b my-feature


### Make Your Changes and Commit Them
git add .
git commit -m "Test change"

### Switch Back to Main and Merge the Feature Branch
git checkout main
git merge my-feature

This will fast-forward merge if no other changes have been made to main.


The broken code is in ./tweetparser -inputdir ../../twits/msg_input  -outputdir ../../twits/msg_output


VS Code has a launch.json. Set one up for running and debugging all the tests.
