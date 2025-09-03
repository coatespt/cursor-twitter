# Project Description and Notes

This document covers
- Goals of the project
- What the big parts do
- Notes on how it works

# Project Goal
The software describe here consumes the X/Twitter firehose data and identifies and analyses the new subjects as they appear in the data stream.  New is the operative word, because the firehose contains so many subjects that presenting them all would be an overwhelming flow of information.  Limiting it to new subjects produces a very manageable flow of data that human can understand and explore.

There are two big pieces, each with its own display component. 

## Finding the Subjects: Z-Filters
The first piece identifies the new subjects in the firehose. This is the Z-filters piece. It has no natural language processing (NLP) or AI components.

It operates on batches (thousands to tens of thousands) of Tweets, identifying the new subjects and clustering together the Tweets on those subjects.  The output of this process is JSON, a series of batches composed of clusters, which are composed of Tweets. A typical incoming batch might be 25,000 Tweets, with the result being one output batch with 0 to a dozen or so clusters that are the new subjects. Each subject might be from a handful to a few hundred Tweets. A maximum number to output is specified in config.yaml.  

Thus, a typical output batch might be less than 1% of the incoming volume, but most of this is the repetitive Tweets on the subject. If you look at just the subjects, 25,000 tweets typically cooks down to just a handful of new subjects that can be summarized in one line--a tiny fraction of 1%.

Most subjects are small--just a few Tweets, but occasionally the get much larger before they cease to be regarded by the heuristic as new.

### Display Web Service
A Web server can read the JSON and display it in a variety of ways, emphasizing various properties.



## Semantic Analysis: Ollama
The results of the first step are sets of Tweets on recognizable subjects. There is a cap specified in config.yaml on the number of Tweets that are actually output for a subject. 

### Loading the Z-Filters Output into Postgres
The first components of the Semantic Analysis piece reads the JSON output of the Z-filters component (batches, clusters, and Tweets) and inserts the data into a Postgres relational database.  

### Submitting the Clusters to the LLM
The second component reads the data from Postgres, formulates AI prompts containing (for instance) a cluster, sends the prompt to Ollama via HTTP, and receives back a response, which it inserts into Postgrest. This makes all of the data on the batch as well as Ollama's take on it, available for browsing.

### Display AI Service
The last major component of the Semantic Analysis piece is a web service that supports browser access. A user tells the web service what it wants to view and the web service queries Postgres for the data.

### Limitations
By far, the biggest time consumer in the process is the AI portion. With Ollama running on an ordinary Linux laptop, each cluster analysis takes about 15 seconds. With a 25,000 Tweet batch, and an average of 3.5 clusters per batch, it works out to about one and a half decahoses.   

Note, however, that a laptop is a slow platform for AI--an inexpensive AI appliance could be expected to process prompts fast enough to easily perform semantic analysis on clusters at the full firehose rate.
Any number could be ganged up as required.

# The Data Set

The input available for development purposes is a bit more than two weeks of the decahose (about 500 Tweets/second, about 6.5 billion Tweets.) It consists of JSON-formatted Tweets in files that have the file order encoded in the filenames as Unix start and end times. The files are about five minutes of Tweets at about 500 Tweets/second. 

The original files are unpacked into uncompressed CSV files.  

In production the input could be either files (for historical processing) or the actual live Tweet stream (for real time processing).  

For live processing, the system is entirely bottlenecked by parsing the JSON Tweets. When reading Tweets from disk, it is still I/O bound, but less so.

Using files is not a "cheat" because any system would typicaly receive Tweets and feed them somehow. The actual development feed is from a process running on the same meachine and sending the Tweets in via RabbitMQ.

The non-graphical output is a series of clustered sets of Tweets that are about new subjects. There is not yet a graphical front end.
 

# The Major Pieces
The following are the main components of the software.
Instruction for running them can be found in the USER_MANUAL.md

## Z-Fiilters: The subjects
### JSON Tweet to CSV Tweet Parser
This component parses the original compressed JSON files into a more compact and easily parsed CSV representation.

This process is not included in speed numbers because every processing method confronts the same problem of unpacking the JSON, and this can be done as fast as required by applying multiple machines to the job. Therefore, we do it offline and work from equivalent CSV Tweets. 

### Language Utility
The Twitter language field (lang:) is worthless. It apparently only reflects what the user's environment says is the user's language, so it is more often than not either empty or wrong, and people of many languages leave the default of English.

The optional language utility is a Go program that reads all the CSV files and submits the text to an NLP library that figures out the language. It's quite accurate, but quite slow, so it exists as a separate program.

The resulting Tweet CSV files are just like the originals, except for the language field being corrected and/or populated. You can select whether or not to use the language info in config.yaml regardless of whether the language adujstment has been made.

The config.yaml has a parameter for whether you want to use all languages, or just en, es, et.

### RabbitMQ/MQ Feeder
The main Z-filters processor can take its input Tweets either directly from disk files or from RabbitMQ. The choice is stated in the config.yaml file. Instructions for starting RabbitMQ are in the USER_MANUAL.md
Use of the Feeder is optional.

The RabbitMQ Feeder reads the files (there are about 5700 of them) in time-order, and puts the CSV on RabbitMQ.  It keeps a notation on disk of the last file it processed, so if it is interrupted, it will pick up more or less where it left off.

### Main Processing
The main processor either reads the Tweet CSV from disk or picks it up line by line from RabbitMQ. It takes the input in batches of several thousand Tweets (config.yaml) figures out the unusually busy words, finds the recent Tweets that use at least M of them, (this is a very small percentage of all Tweets), runs a clustering algorithm on those Tweets to identify the clusters of usage, and writes the resulting clusters of Tweets, as well as metadata about the clusters and the bathces, to standard out as JSON.

This is the heart of the system because it identifies the subjects. It is very fast, operating on up to 50,000 Tweets/second even on a laptop.

#### Main Processing Viewer
A Web service provides interactive browser access to several different views on the subjects, the words that characterize them, etc.  This web service is available on port 8080 wherever it is running.

## AI Semantics
The optional AI portion uses the Ollama LLM running locally to describe the meaning of each cluster/subject. The nature of the AI prompt can be changed programmatically. An upcoming enhancement will allow the format of the prompt to be supplied by the user.

### JSON Main Output to Postgres
A free-standing utility reads the JSON output from main and inserts it into several tables on Postgress. One of the tables contains the name of the particular run, as well as the values of key parameters that it extracts from the config file.  

This allows the output of any number of runs to exist side by side in Postgres.

### AI/Ollama Service
A second utility program reads the batch/cluster/tweet information from Postgres cluster-by-cluster, formualtes it into prompts, and sends it to Ollama via HTTP. Ollama runs locally on its own machine.

The response to the HTTP Post is inserted in Postgres, where it can be queried along with the Tweets and metadata of the cluster/subject.

### AI Web Service and Browser
A web service exists to display the AI results in a browser. The browser is used to select and control what what the web service extracts for display.


# Speed of Computation and Significant Events

## Z-Filters Portion
The Z-Filters processing is very fast. A laptop can process a stream of about 8000 to 9000 Tweets/second with the Tweets coming in from Rabbit MQ, and about 45 to 50k if it reads the CSV directly, i.e., as much as ten firehoses.
 
The two-weed Tweet sample was recorded from two weeks of a Decahose. However, the Tweets can be consumed at any speed. 
 
Each CSV file is about five minutes of the decahose, i.e., about 30,000 Tweets per file, or 33 files per million Tweets, which is about 2.7 logical hours of the decahose. 
We are actually processing 2.7 logical hours in about 10 minutes.
 
If you are starting and stopping frequently for development purposes, the system can optionally read in the existing frequency data, for an instant start.  Beware that if you are starting at some random point, the saved frequency data will be wrong to an unpredictable degree. This means you need to either start fresh or disregard the results until you have run for long enough to completely update the background statistics.

## AI Portion

## JSON to Postgres
The JSON to Postgres step is reasonably fast, being dominated by the time to unpack the JSON. Tweets on new subjects are almost never as much as 1% of the full volume. A batch might represent five seconds of the full firehose and result in 100 to 200 rows, so moving the data into Postgres is not a heavy load on the database server. 

The AI piece is bottlenecked by access to Ollama, which takes about fifteen seconds per cluster.
This works out to about 1.5 of the decahose rate, whereas the initial load into Postgres could handle several firehose loads.  

However, this can easily be scaled to take advantage of almost any amount of available LLM capacity. For instance, an Nvidia Jetson Orin Nano, which costs about $275 can run the LLM about 10x as fast as the Linux laptop.

As many such appliance as necessary could be used, so it is not clear where the next bottleneck would appear. 

# Miscellaneous Issues and Details
This section describes various processing details.

## Persistence of Subjects

In addition to computing the new subjects, the system can give information about how long those new subjects have existed in terms of the number of batches. This is given in the form of a list of the previous N batches that also match the new subject.

Persistence of subjects is a significant consumer of CPU, bigger than the cost as the clustering itself.  It can be turned on and off, but the effectiveness of turning it off is still to be determined. It's not clear that it affects throughput, which may be bottlenecked elsewhere.

## The Big Token Window

Word distributions change thoughout the day and week. People don't Tweet the same things at breakfast on Monday that they do at 1:00 AM on Saturday.  Also, the world turns, and while New Yorkers are getting up in the morning, people in Bejing are out for the evening.

Equally importantly, surges in usage continually adjust the current background frequencies.

Note, these issues are best thought of in terms of the logical time in the Tweets, independently of the actual time to process. In a live field, these are the same, but with stored data, processing can be considerably faster than logical time.

### Word Distribution

See "Zipf Distribution" in any reference for details.

Consider that 

- "Dog" is about the 1000th most common word, yet only 1/10,000 of words will be dog.
- "Mirror" is the 3000th most common word, and only 3 words out of 100,000 will be mirror.  
- "Edge" is the 5000th most common word, and it appears only once in 100,000 words
- "Shocking", "critical", "stripper", and "java" are all about rank 10,000 and appear somewhat less than once in a million words. 
- The most common 250 words in the corpus occur as much as the least common 11 million.

Because even fairly ordinary words are literally one-in-a-million or fewer, you need a lot of words (millions) to get even a reasonably accurate estimate of a word's background frequency. The decahose is 500 Tweets/second or about 5,000 words/second, which is about 3.3 minutes of data. So a window of five million words is about 16.6 logical minutes.

So why not have a window of fifty or a hundred million?  Because there is a trade off between the quality of the frequency statistics and having a short enough window to both keep up with the time of day and to allow surges of frequency associated with subjects to age out reasonably quickly.  Three million tokens is about 300,000 Tweets, which is about ten files of five logical minutes each. Fifty minutes, of the decahose give or take. 

This is an interesting point because with the decahose, the time it takes to get enough tokens to accumulate a good picture of word frequency is long enough that time of day comes into play and it multiplies the benchmark of what is "new" times ten. Playing the Tweets ten times as fast isn't the same as the full firehose because the decahose requires ten times as much logical time for the same number of tweets.

Because the window has to be large, the tokens underlying the token counters are written to disk in batches as they are read in. After the number of token batch files reaches its defined limit, each time a new batch is written out, the oldest batch is read in (and deleted from disk). The contents of the old batch are then used to decrement the global token counts.

You can use whatever batch size suits, but fifty files for three million tokens seems to work well. That means you're aging out the old tokens out in approximately one minute increments over a window of fifty logical minutes. 

Note that frequency calculations would typically happen multiple times in the span of time represented by an entire token window.  Window size and frequency of recalculation are controlled by separate parameters. Say you have a five million word window, i.e., 16 or 17 minutes, you might recompute the frequency filters every couple or three minutes.

## The Small Window of Tweets

The big token window is only of concern only to the off-line frequency calculating thread.  The main thread keeps short window of Tweets (not just tokens) in memory for use in the clustering algorithm, which is in the main processing line.

This is a conventional queue that keeps a configured number of Tweets, ageing out the old ones as new ones are added.  This would be configured to hold a few processing batches of Tweets. As a batch is typically in the range of one thousand to a few thousands, it would be a small multiple of that size.  

Clustering happens with respect to the latest batch.  However, the persistence of clusters is tracked across previous batches. You can see in this way whether a subject just popped up, or has it been around a while.

The size of a batch, the number of batches kept in memory, and the number of batches used to compute a clustering are all configurable.
 

## Confirmation of the Z-score Principle
The data technically violates the assumptions of Z because a Gaussian applies to a continuum of real number values, not integer counts. There is an argument to be made for Poission based scoring which is designed for discrete values.

Also, we pre-suppose that only the underlying frequencies are approximately Gaussian. We are assuming that the busywords impose an explicitly non-Gaussian distribution on top of the main distribution of pseudo random values.

Nevertheless, use of Z in this way is fairly common for anomalie detectio for anomaly detection, which is the case here. We are only identifying anomalies and don't really need to know exactly how anomalous they are.

The following are excerpted from the logs. Note that the low Z scores are all quite small negative numbers most of which are between -1 and 0, as you'd expect.  
The high z scores are up in the double digits. This says it's spotting anomalies effetively.

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
Most distinct tokens are, for practical purposes, unique on the time scale of a day. At least half of the tokens don't appear more than once in two weeks. Permanently storing the 3pk values for super-rare tokens incurs two very significant costs:
- It wastes a ton of memory.
  - With a token window size of three million, and fifty token files on disk, each time we age out a cached file of tokens, about 20k tokens that are in the counter map go to zero.
  - The FCT thread removes these zero count tokens from the counter map to prevent bloat.
  - However, the main thread, which keeps a 3pk <-> token map does not know about the token counter map in the FCT and has no independent way to know which are meaningless. 
  - Those useless mappings accumulate a cost about 100MB of memory per hour to store permanently.  
- The bloated 3pk<->token map also increase the probability of collisions. 
  - There are something like 135M distinct tokens in the two-week data set.
  - The 3pk mapping space is only one or two billion. If you kept them all, 3pK collisions would be constant.

We solve this bloat problem by having the FCT put tokens on a queue when their counts in the counter map go down to zero. 

The main thread nibbles away at these entries on the queue, taking a bunch of them every few hundred Tweets, and deleting the corresponding entries in the 3pk<->token mapping.  

This is mostly harmless, because if the token ever shows up again, the main thread will automatically recreate it anyway. It is possible, but unlikely, that a mapping will be removed after a set of the corresponding 3pk has been handed off to the busyword processors. This would not happen in steady-state, but can happen after a restart from stored state. If the analysis thread doesn't get back a token for a 3pk, it simply discards it. (If it has any significance, it will show up in the next batch anyway.)

   
# The analyze_tokens Program Output

The **analyze_tokens** program reads CSV files and accumulates the number of distinct tokens that it has encountered.

Running on about 1200 CSV files, which is at least a couple of days of data, shows that the percentage of new tokens rises quickly at first, and then settles down to a pretty stead rate of about one new token for every 3.15 Tweets, or roughly every 31.5 tokens, which is surprisingly high and unwavering.

It is unknown if the full Firehose would behave differently. It could be that most of the diversity at any given moment is contained in a fraction of the Tweets.


 