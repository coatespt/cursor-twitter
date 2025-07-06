# Session Notes

## Project Architecture & Workflow

### Goal
The project is a way to read the Twitter firehose and find new subjects appearing in the 
tweet stream. The underlying insight is that it would be essentially useless to 
analyse all the subjects that are active at one time because this number is in the 
tens of thousands--at least 100x too much for a human to grasp.  

However, new subjects actually arrive at a manageable rate. Depending upon exactly how you
define "subject," new subjects arrive perhaps every few seconds. 
By exposing only the stream of new subjects (and the tweets they are in) you see 
a useful, evolving view of what people are talking about. With reasonable parameterization
of the definition of "subject" you get something about like the Times Square News Ticker.

A subject is easiest to identify not by using the most frequently used words, but by
using words that are anomalously frequently used at the moment. As the overwhelming 
majority of words are very rarely used (on the order of zero to a handful of times a day)
in practice new subjects are characterised by the appearance of two or three (or more)
words that are suddenly being used with unusual frequency.

For this to be helpful, the anomalously frequently used words (called busy words) must be detected
very quickly because it must be practical to analyse the current window of tweets for
clusterings of busy words. 

Ordinary word frequency computation would be very hard to do sufficiently quickly. Therefore
a probabilistic heuristic is used that only does occasional global frequency computation (in order
to keep up with word usage changes with time of day, day of week, etc.)

Instead, the heuristic detects the leading edge of surges in frequency. As it is only sensitive to 
the leading edge, it automatically forgets words that surge in usage and then stay common.

The heuristic is extremely fast--potentially considerably faster than the firehose rate. This means
it is practical not only to do real time processing, but to process historical 
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

Word usage has a Zipf distribution, which means that 99% of words are very rarely used. 
"Rarely" in this context typically means perhaps zero, once, or twice a day.

Because most words are very rare, literally one-in-a-million in a text stream, if two or 
three busy words appear in a few tweets, it's almost certainly a new subject.
 
A word being busy is not about its absolute frequency. It is about it's relative 
change in frequency of use. 

If a hot subject comes along, i.e. people are tweeting and retweeting text that contains
some subset of three or four busy words, it will tend to be forgotten after a while because
the rate of use that caused them to be marked as busy words slowly becomes the norm for them.

However, in practice, a major subject will keep getting augmented with other unusual but related
words, so it keeps the subject fresh beyond the expiration of the original busy words.

The key to this working is that the sliding window has to be small, e.g., fiver or ten thousand 
tweets or so to keep the computation manageable. The size of the window determines the "resolution" of the view. 

So sum up, the algorithm 
- Detects all the currently surging words.
- Finds the tweets in the current window that use a proper subset of these words
- Discards all the other tweets
- Clusters the busy-word tweets by the busy words they use.  This isn't necessarily a unique clustering.

### How It Works

The trick is, you can't use an ordinary frequency calculation. Frequency calculation
could be modified to show frequency relative to the previous frequencies, but you'd have to do that massive calculation every few seconds in order to keep the Tweet window small.
 
It would be difficult to execute such a large computation on millions of words every few seconds. 
You would need to 
- Count the frequencies of all words in the current window of time.
- When the limit of the window is reached, compute the relative frequencies for the words.
- Compare them to the relative frequencies of of the current window to those of previous window
- Derive a list of those with changes too large to be random chance.

In addition to this, words differ radically in how often they are used. The most common dozen 
or so words in a window of say, a million tweets, get about as much total usage as the least
used 100,000.

## How the heuristic works.

### Word Frequency Computations

We use a periodic computation of global word frequency to divide the universe of words into F frequency classes, with the most frequently used words in the first class, and the least frequent words in the last class. This frequency analysis is done outside of the processing of the stream of inbound tweets. The purpose is to provide a background look at what normal word frequencies are 
at the particular time of day, day of week, etc.

The global computation is done at a very coarse grain, e.g., say, between fifteen minutes and an hour.The word freqencies are computed and the words are divided into equivalence classes according to
frequency. The words in each class will account for approximately equal usage.

The word frequency classes are used to construct a series of filters that allow an incoming word's 
frequency to be identified. The first few classes contain only a few words, and the last more than all the others put together. Any incoming word that doesn't match to any frequency class is necessarily rare, so it is treated as being in the least frequent class.

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

---

## Things Known To Be Wrong
* Mode is in configuration but not needed. Remove--we will always use MQ for now
* maxRebuildTime should be removed from Configuation.  It is also used at the beginning
of the main loop. That code should be removed.
 

---

## Useful Commands

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

---

## TODO / Next Steps

- [x] Clean up complex code and keep only essential components
- [x] Create simple, blocking Go receiver
- [x] Test the basic pipeline: JSON → CSV → RabbitMQ → Go receiver
- [x] **SUCCESS: Pipeline working at high throughput**
- [ ] **Next: Add config file support** (YAML config, command-line flags)
- [ ] **Next: Add sliding window** to buffer tweets for processing
- [ ] **Next: Implement Bloom filter frequency classification**
- [ ] **Next: Add three-part key generation**
- [ ] **Next: Build Z-score anomaly detection**
- [ ] **Next: Add clustering for new subjects**
- [ ] Document any new architectural decisions here
- [ ] Consider feature request to Cursor for persistent session memory

---

## Notes
- If you start a new session, paste relevant parts of this file into the chat to restore context for the AI assistant.
- Always check that files created in Cursor are saved to disk (visible in Finder) to avoid data loss.
- We're building incrementally now - simple foundation first, then add complexity step by step.
- **Current status: SOLID FOUNDATION WORKING** - ready to add processing logic incrementally. 

-- Volume Issues
Volume decahose = 500/sec
500/sec = 30k tweets/min = 450,000 tweets/15min  = 1.8m tweets/hour
Say, an average of 10 words/tweet = 4,400,000 words/15 min or 18m words/hour

761,674 in 3m55sec  = about 3242 tweets/second via MQ
The firehose is about 5700/second. So about 0.57 firehoses.
This could probably be speeded up with sending in batch, but
who cares? This is a demo.  We're doing more than six firehoses.

This means a 15 minute frequency filter update is only looking
at a small part of the universe of words, which is 20m+ distinct words in a day. But so what? If you havent encountered a word in 4.4 million, it's rare, right?

TTD

-- Structure the code directory for multipart pipeline.

-- Add some code to track the processing rate i.e., msgs/second processed.

-- Receiving Tweets
---    Parse and tokenize the text of each tweet, (which is a field of the CSV)
          attaching a multi-set of word tokens to each tweet struct. (multiset not a set because the count matters.)
       There should be at least a place holder for a filter function that
            Normalizes tokens, e.g., all lower case, omit apostrophes, etc.
            Rejects unacceptable words (The n-word is amazingly popular)
            Etc
       If an accepted word/token doesn't already map to a 3pk, compute one 
          and put it in the global two-way lookup table token->3pK and 3pk->token
--- The tokens need to be globally counted in a rolling word-counter that computes
         word frequencies at a coarse grain of m minutes worth. 
          This is the data used to compute the frequency classes. 
          The value of m is configurable, but probably something like 15 minutes
      The tokens need to be rolled out of the global-counter too, so all the
          tweets with their token multi-sets for the last m minutes need to be 
          retained, in an ordered list. 
          It might make sense to maintain the normalized, accepted tokens in the 
            tweet object.
      The value m is chosen short enough to keep frequency data structures fluidly 
          updated as the day goes on, but long enough that it is not a burdensome
          computation. I'm thinking 15 minutes or so.
--- When m minutes of tweets have been processed, the frequency count is executed.
      words are partitioned into sets such that each set has the same total number
        of usages. I.e. the common words set will be small because they are used in 
        huge numbers. 
      The least common word set will be large because each one is
        very rarely used. 
      This algorithm for partition size may or may not be right
        so we need to make it pluggable.
      The F frequency sets are inserted into their own Bloom filters
--- The F Bloom filters are used in a function that takes each incoming word and
      puts it on the correct frequency pipeline.
--- Every time m minutes pass, the Bloom filters are updated and thus the frequency
      partitioning keeps up with the time-of-day changes as well as the ongoing 
      changes from the contents new subjects.  

Getting to this point, where the parsing and tokenizing are happening, the BF's are 
built, and it can run indefinitely without memory growth is a big milestone.


The test for the above should be that 


  
-- Write a function to create 3pk for every word incoming.
-- Use it to build
-- Also write to an SQL database?
== Define data structure for tweet. e.g. unique id, csv text, 3pk list

Steps
--  Build word to 3pk two way lookup
==  component to maintain ongoing word frequency computation
--    This could be a separate component that takes the first n files and 
      does freq analyais.
      Then we have a function that computes F frequency BF's for the current 
      frequency table.
==  
--  Create tweet data structure with 3pk's for the text
--  

Major Components

Let's make a practice of saying what each module does
at the top of the file.  Comment as if I'm an idiot.

--- CSV Sender        send_csv_to_mq.py                 DONE
    This was rewritten in go because there were apparently hidden character problems
    that Go was choking on.

--- Main              main.go                           Shell exists
    Main()  receives the tweets
            builds the Tweet objects
            operates the token counter
            Creates and updates the token frequency partitioning

            We ran into a problem where a second thread was doing the frequency 
            calculation and ran into a concurrency proble with adding to the Map
            while it was being accessed.

            This seems to have been fixed, but now there is something fishy with 
            the addition of tokens because some of the cycles add very few tokens.

            It was switched to a separate thread because the frequency calculations
            took a majority of the time, but I suspect the design is just wrong.

            It may be that a second map needs to be created to accept tweets while the
            main one is being processed.

            There would also need to be a provision for removing old tweets in an orderly way.


                           
--- Tweet data structure
    Struct holds the CSV fields as variables, also holds a multiset of 3pk's, i.e. words. An important field is the unique orderd id. Every incoming tweet is turned into one of these.
    The eccentric Twitter date string is converted to a real Time object that is used for ordering.
    This is important because files are just test source and aren't coming in necessarily at a
    realistic rate.

--- Part of this is the tokenizing process which normalizes the words and  SIMPLE IMPL
    perhaps deletes some trash, e.g., racial slurs, etc.

--- Another part is constructing the 3pk's
                            
--- There is a global mapping of 3pk's to words and words to 3pk's. Nothing ages out of this

--- We will be an entire frequency window of, tweets in a
    queue for several purposes: (1) We need to roll their contents out of the global frequency counter after m minutes. (2) We need the recent tweets available for clustering on busy words, (3) the tweets are the main component of the clusters delivered in the output, etc.

    These tweets are what feeds the mechanism that rolls old tokens out of the counters

--- Global token occurrence counter 

--- There were to be F frequency class Bloom filters, but it's more economical to use Bloom
    filters only for the least frequently used words, maybe one or two of the partitions.
    The rest of the partitions can get by with a HashedSet which would be much cheaper to access.

--  JSON to CSV was in Java but it's been ported to Go and lives in ./parser  Compilation and
    running instructions below.

Next big milestone is to be able to read mulitple windows tweets. We should see 
    The queue fill up,
    The tweets that fall off the back used to update the counter
    The frequency class test data structures should be built in a separate thread.

--- Building and running the main program
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


# Development Plan

## Some Concerns to Look Out For
 

* If the current interval of W has been reached, flag is set so the
  the FCT thread knows it is time to do frequency analysis
* When the frequency analysis is done, a flag set by the FTC to tell the main thread that the that the new frequency filters are ready. There may be a better way to do this.


## Counts and frequencies are computed by a FrequencyComputationThread FCT
 
  - The FCT runs in a loop that 
      - Takes a token off the InboundTokenQueue queue and 
          Increments its count in the CountMap
      - Takes a token (if available) from the OldTokenQueue and decrements the   
          corresponding counts in the CountMap
      - Checks a flag set by the main thread. If the flag is set, the FCT stops   
          taking tokens off the queues and does the frequency calculations and constructs the frequency 
        class filters.
      - When the filters are built and made available, it resets the flag
          and resumes the token processing loop loop.

- Meanwhile, the main thread keeps putting values on the InboundTweetQueue and  
    the OldTweetQueue. There is no conflict as the queues are thread safe, and the FCT will drain those queues when it finishes constructing the current frequency filters.

- When the filters are complete it makes them available for asynchronous pickup by the main thread. This might be made thread safe without a flag by using mutexes around the block
of code that accesses the filters? Not sure the best way to do it. 

- When the global frequency calculations are done, it resumes draining the ITQ and the OTC, one token at a time each. This is easily accomplished by putting the entire  process in a function call that is in the loop, but only runs when the flag is set.

- There is a potential issue here if the frequency computations can't keep up with the pipeline is processing tweets. If the global calculations are temporarily off by a cycle of two or three off, it's not a big problem, but if the pipline is consistently faster
  - The size of the unprocessed backlog on the InboundTokenQueue and OldTokenQueue grow without bound.
  - The creation of fresh frequencyfilters will get more and more out of sync with the tweet stream. 
  - The former is a memory consumption problem. The latter will degrade quality, as words that have recently become more frequent pollute the streams of lower-frequency 
  frequency classes. Possible solutions include
 

## Main processing pipeline.

### Using the Frequency Class Filters
-  The F frequency class filters described in the previous section are used to determine what class word is in.
    -- The word is fed to each, starting from most frequent to least frequent.
    -- If kth filter recognizes the token, the token is passed to pipeline k
    -- If no filter recognizes the token, it is treated as being in the least frequent class
-  It's 3pK's are identified from a global lookup table. If a token doesn't have a 3pk registered, it is created and added to the global table.
-- the 3pk is passed into the appropriate kth frequency class pipeline.

We had formerly been planning to use Bloom filters for the frequency class
registers, but why not just use some kind of hashed set? It should be faster and it's not that much space.

### The Core Token Processing Structures
The heart of a processing pipeline are the 3pK stream processors. There is one 
for each freqency class.

Each 3pk processor is centered around a data structure with three arrays of counters. The arrays are all of the same size. 

The size (3pkCountArraySize) is configurable, but experience suggests about 1000 is a good balance. The for reasons why will be made clear below.

The 3pkProcessor reads 3pk's from a queue. (The 3pk's are put on the queue by the main processing thread.)

Each of the three elements of the 3pk is used to choose the index of a counter to increment, by taking the integer value modulo 3pkCountArraySize. That counter in the corresponding array is bumped.

After a window of width Batch, (Batch is typically a small fraction of WindowSize, perhaps a few seconds) its counters will be have been incremented in total approximately the average number of tokens in a tweet multiplied by Batch and divided by the number of frequency classes.

When Batch number of tweets have been processed by the main thread and their 
3pk's sent to the 3pk processors, the main thread sends a special
3pk to all 3pK processors to tell them to stop reading 3pk's and process the batch. E.g. a 3pk with all three values equal to -1?  

The reason for this, and not a flag, is to keep them synchonized, as the signal 3pk's would be injected into the queues at the same time. Therefore each processor would keep draining its queue until it hits the signal.

* The counter arrays are copied and then cleared.
* The copies are posted to be picked up by computation threads.  Possibly put 
        on queues to allow for asynchronous processing?
* Processing is resumed.

### Computing the Busy Words
While ingestion of Tweets, background frequency processing continues, and populating of the 3pk counter arrays continues, the the analysis of the three arrays 
from each processing queue proceeds as follows
* The assumption is that if all the words behind the 3pk's in a frequency class
    had approximately the same average frequency, the counters would have fairly 
    similar values with a Gaussian (i.e. normal) distribution.
* Busy words, however, will give higher values to the counters they map to.
* A Z score is computed for each counter of each array. Z measures how unlikely
      the deviation of a given counter value is from what would be expected by random.
* We only care about the counters with sufficiently high z. Z is set in configuration, and is much higher than the -4 to 4 range normally encountered in social statistics.

Given three sets of indices of counters that pass the Z test
* We take the Cartesian product of the three sets 
* Most of these will not corrrespond to existing 3pk's. 
** If you actually had K busy words producing high index values in each array, you'd have k^3 combinations, and all but about k/k^3 of them would be
spurious. For example if there are 25 legit busy words, that's 15,625 combination of which approximately 15,600 are spurious.
* Note that if the size of the counter arrays is 1000, there are a billion possible 3pk's, but probably only about 20 million distict tokens appear in a 
day of tweets, which means about 1 in 50 random 3pk's would correspond to a 
real token. 
* 1 in 50 sounds high, but most of them will never actually appear even
once in any of the few thousand Tweets considered in an interval. And when one does, so what? It's one spurious busy word that is again, unlikely to appear in 
the same Tweets as other busy words. Therefore, very little harm comes from a reasonable spurious busy-word rate. 
  
### What Happens with the busy words.
The busy words produced by all the 3pk processing pipelines are grouped together. 
The set of Batch Tweets are examined, and all Tweets that don't contain tokens
matching the 3pk's produced by the pipelines are discarded.

Depending upon configured values, 1, 2 or more busy words might be required. The 
best value will be determined empirically.

A clustering algorithm is applied to the Tweets, grouping them together based upon 
the subset of busy words they contain. Details TBD.


## Some Sample Code for PTC's Edification
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
git commit -m "Sjprt ;oved change"

### Switch Back to Main and Merge the Feature Branch
git checkout main
git merge my-feature

This will fast-forward merge if no other changes have been made to main.
