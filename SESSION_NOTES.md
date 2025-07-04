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
  -- Detects all the currently surging words.
  -- Finds the tweets in the current window that use a proper subset of these words
  -- Discards all the other tweets
  -- Clusters the busy-word tweets by the busy words they use.  This isn't necessarily
     a unique clustering.

### How It Works

The trick is, you can't use an ordinary frequency calculation. Frequency calculation
could be modified to show frequency relative to the previous frequencies, but you'd have to do that massive calculation every few seconds in order to keep the window small.
 
It would be difficult to execute such a large computation on millions of words every few seconds. 
You would need to 
- count the frequencies of all words in the current window of time.
- When the limit of the window is reached, compute the relative frequencies for the words.
- Compare them to the relative frequencies of of the current window to those of previous window
- Derive a list of those with changes too large to be random chance.

In addition to this, words differ radically in how often they are used. The most common dozen 
or so words in a window of say, a million tweets, get about as much total usage as the least
used 100,000.

--- How the heuristic works.

--- Word frequency computations.
We use a periodic computation of global frequency to divide the universe of words into F frequency classes, with the most frequently used words in the first class, and the least frequent words in the last class. This frequency analysis is done outside of the processing of the stream of inbound tweets.

The global computation is done at a very coarse grain, e.g., say, between fifteen minutes and an hour.  The word freqencies are computed and the words are divided into equivalence classes according to
frequency.  If you have F frequency classes, the words in each class will in the aggregate have
been used approximately equally.  

The word frequency classes are used to construct a series of filters that allow an incoming word's 
frequency to be identified. The first few classes are quite small, so hashed sets are appropriate.
The bigger classes use Bloom filters.  Any word that doesn't match at all obviously rare, so it is 
treated as being in the least frequent class.

The word counts, frequency class computation, etc. is done on a separate thread, and the main tweet 
processing is periodically suspended while the frequency filters are updated.

-- Processing the stream

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

## Key Decisions

- **Message format:** CSV row as a string (not JSON) for simplicity and generic compatibility
- **Queue name:** `tweet_in`
- **RabbitMQ:** Running locally on default port (`localhost:5672`), default credentials
- **Consumer:** Blocking consumer (no polling, no "no message available" warnings)
- **Current approach:** Minimal, working foundation first, then add complexity incrementally
- **Persistence:** All important code, architecture, and decisions should be saved in project files (like this one) due to stateless AI sessions

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
   
Running Notes

The code is in ~/python-work/cursor-twitter
A few things might still be in ~/python-work/twitprocess.

The twitprocess directory should be retired as soon as it's uselessness is confirmed.
There is also a ton of stuff in ~/python/src most of which looks obsolete. Be careful deleting though.

--- Sending Tweets
Running the actual software assumes you have a directory of CSV files of tweets, in this case msg_output.

-- The CSV is created as follows. This reads GZ files from msg_input and writes them as  CSV files in msg_output

I THING THIS IS OBSOLETE 
cd /Users/petercoates/python-work/twitprocess 
## Obsolete python ./src/send_csv_to_mq.py ../twits/msg_output
 ./process  -config ../config.yaml -print-tweets=false

-- LOGS SEEM TO BE IN THE WRONG PLACE. THE LOCATION IS CONFIGURABLE 
Logs in ./logs


--- THE CURRENT WAY TO RUN THE JSON->CSV PREPROCESSOR
This was formerly done in Python and created parsing problems in Go 
because the parsing was sloppy.

cd /Users/petercoates/python-work/cursor-twitter/parser
go build -o tweetparser parser.go 
./tweetparser -inputdir ../../twits/msg_input  -outputdir ../../twits/msg_output 

--- THE CURRENT TWAY TO BUILD AND RUN THE MAIN PROCESSING
cd /Users/petercoates/python-work/cursor-twitter/src
go build -o process main.go
./process  -config ../config.yaml -print-tweets=false


--- DEVELOPMENT PLAN
-- A big concern is to eliminate problems with concurrent access of counting data structures.

-- Maintaining a window of the last W tweets in the system.
  - We assume that W tweets constitute one window that is maintained in TweetWindowQueue
  - In main() each tweet structs is computed, including the tokenization of the text. 
  - As part of tokenization, each token is checked for existence in the data structure that
    maps Tokens to 3pk's and 3pk's to tokens. If it's not there, it is added.
  - The newly constructed tweets are put on a TweetWindowQueue
  - After W tweets have been accumulated in the TweetWindowQueue, the oldest tweet will be removed
    and its tokens placed on the OldTokenQueue.

--- Accumulating word counts for the latest W tweets (off of main-pipeline processing)
- Every time a tweet is constructed and placed on the TweetWindowQueue
  - The tokens for the tweet are put on an InboundTokenQueue
  - A maximum of W tweets can be on the TweetWindowQueue
  - If more than W tweets have come in, the tweet from the tail of the 
      TweetWindowQueue is removed and the its tokens are place on an
       OldTokensQueue OTQ (see below.)
  - If the current interval of W has been reached, a flag is set so that 
      the FCT thread knows to process.
  - the InboundTokenQueue and the OldTokenQueue will buffer until the new
      data structures are built.
  - On each loop, a flag set by the FTC is set that says new frequency 
      filters are ready.

Check Go--I think it has a syntactically nice way to do mutual exclusion on 
access to the the frequency filter data structures.

--- Counts and frequencies are computed by a FrequencyComputationThread FCT
 
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

- When the filters are complete it makes them available for asynchronous pickup by the main thread.

- Then it resumes draining the ITQ and the OTC, one token at a time each.

- There is a potential issue here if the frequency computations are consistently slower than the 
  main pipeline is processing tweets. If it's temporarily a cycle of two or three off, it's not
  a big problem, but if the pipline is consistently faster, the size of the unprocessed 
  backlog on the InboundTokenQueue and OldTokenQueue grow without bound and the frequency 
  filters will get more and more out of sync with the tweet stream. This will degrade quality, as words that have become more frequent pollute the streams of lower-frequency 
  frequency classes. Possible solutions include
    - Increase the W for fewer global computations. The number of distinct tokens
      rises slowly with the number of tweets but there are fewer computations. 
    -It would be useful to do a
      performance analysis to find out where the time goes.

-- Main processing pipeline.

-  The F frequency class filters are used to determine what class word is in.
    - The word is fed to each, starting from most frequent to least frequent.
    - If kth filter recognizes the token, the token is passed to pipeline k
    - If no filter recognizes the token, it is treated as being in the least frequent class
-  It's 3pK is identified from a global lookup table.
-- the 3pk is passed into the appropriate kth frequency class pipeline.
-- The heart of a processing pipeline is a data structure with three arrays of counters. The
    size is configurable, but experience suggests about 1000 is a good balance for reasons 
    that will be made clear below.
-- Each element of the 3pk is used to choose the index of a counter to increment. 
-- After a window of width T, for T a fraction of the size of W, a lot counters will be 
    incremented. Approximately the average number of tokens in a tweet multiplied by T.
-- Ingestion is suspended.
      - The counter arrays are copied and then cleared.
      - A flag is set to tell the next stage to compute. The next stage is the only user of
        the copied counters
-- Processing is resumed.

-- Next Stage
-- While processing continues, the the analysis of the three arrays is as follows.
    - The assumption is that if all words had approximately the average frequency of their
      frequency class, the counters would be approximately equal, with a gaussian distribution.
    - Busy words, however, will give high, non-random values to the corresponding counters.
    - A Z score is computed for each counter of each array.
    - Only the counters with sufficiently high z are considered.
    - The Cartesian product of the indices of the counters is taken. 
        - Most of these will not corrrespond to existing 3pk's. 
        - If there are K busy words in the interval, there will be about k^3 combinations.
        - You are discarding about k/k^3 of them. 
        - If there are 25 legit busy words, that's 15,625 combination of which approximately
          15,600 are spurious.
    - If the size of the counter arrays is 1000, there are a billion possible 3pk's. 
    - This means that about 1/6410 of the spurious keys will match something.
        - So these will result in false positives
        - However, who cares? There aren't that many and they are unlikely to show up in the
          output.


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