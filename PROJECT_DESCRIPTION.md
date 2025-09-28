# Project Description and Notes

This document covers
- Goals of the project
- What the big parts do
- Notes on how it works



# Project Goal
The software describe here consumes the X/Twitter firehose data and identifies and analyses the new subjects as they appear in the data stream.  New is the operative word, because the firehose contains so many subjects that presenting them all would be an overwhelming flow of information.  Limiting it to new subjects produces a very manageable flow of data that human can understand and explore.

There are two big pieces, each with its own display component. 

## Diagram
See ./block_diagram.pdf for a graphical overview of the components.


Note that many of the components show disk access as a data source. Any of the disk sources can be replaced with messaging or other systems and some, such as the main component of Z-filters have that option.  However, using messaging for development and demoing is unnecessary and complicated deployment.  It also only applies to live deployment. Messaging is unnecessary for historical data applications.


## Finding the Subjects: Z-Filters
The central piece identifies the new subjects in the firehose. This is Z-filters piece. It has no natural language processing (NLP) or AI components.

It operates on batches (typically thousands to tens of thousands) of Tweets, identifying the new subjects and clustering together the Tweets on those subjects.  

The output of this process is JSON, a series of batch metadata followed by a set of clusters, which are composed of Tweets. A typical incoming batch might be 25,000 Tweets, with the result being one output batch with 0 to a dozen or so clusters that are the new subjects. Each subject might be from a handful to a few hundred Tweets The number of Tweets in a new subject is usually a single digit or low two-digit number. A maximum number to output is specified in config.yaml.  

Thus, a typical output batch might be less than 1% of the incoming volume, but most of this is the repetitive Tweets on the subject. If you look at just the subjects, 25,000 tweets typically cooks down to just a handful of new subjects that can be summarized in one line--a small fraction of 1%.


### Display Web Service
This component is a Web server that can read the Z-Filters output JSON and display it in a variety of ways, emphasizing various properties.
It defaults to running from a file, but it can run from messaging.


## Semantic Analysis: Ollama
The main component of the results of the Z-Filters are sets Tweets on recognizable subjects. There is a cap specified in config.yaml on the number of Tweets that are actually includedfor a single subject (cluster.) 

This data is useful as-is, as seen in the Z-Filters display service.

The semantic analysis component uses an LLM to analyse the subjects and generate a description of what they are about, the entities they refer to, etc. 

This is done by generating a prompt for the LLM (in the demo, Ollama) and sending it off via HTTP.
The result is stored in Postgres.

### Loading the Z-Filters Output into Postgres

The first components of the Semantic Analysis piece reads the JSON output of the Z-filters component (batches, clusters, and Tweets) and inserts the data into a Postgres relational database.  

This database is the source for the data in the prompts sent to Olllama, and remains available for any destired post-processing of the Ollama results, e.g., recovering the original Tweets, etc. 

### Submitting the Clusters to the LLM

The second component reads the data from Postgres, formulates AI prompts containing (for instance) a cluster, sends the prompt to Ollama via HTTP, and receives back a response, which it inserts into Postgrest. This makes all of the data on the batch as well as Ollama's take on it, available for browsing.

The demo uses a single prompt, i.e., it only has one way to ask Ollama to analyse a cluster.

Other prompts are possible. For instance, a time series of Ollama's responses could be sent back to look for trends over time, etc.

### Display AI Service

The last major component of the Semantic Analysis piece is a web service that supports browser access to the Ollama responses. A user tells the web service what it wants to view and the web service queries Postgres for the data.

The browse only has one basic function implemented, which is browsing forward and backward in time through the results.  However, all of the backing data is in Postgres, so it is available fo more complex displays.

### Limitations

By far, the biggest time consumer in the process is the AI portion. With Ollama running on an ordinary Linux laptop, each cluster analysis takes about 15 seconds. With a 25,000 Tweet batch, and an average of 3.5 clusters per batch, this works out to about one and a half decahoses.   

Note, however, that a laptop is a slow platform for AI--an inexpensive AI appliance could be expected to process prompts fast enough to easily perform semantic analysis on clusters at the full firehose rate. Any number could be ganged up as required.

A batch (with the demo settings) is about five seconds of the firehose.  While Ollama on the laptop takes about 15 seconds to process five seconds of the firehose, the Z-Filters component, which finds the subjects, can do five seconds of the firehose in about one half a second.

Thus, for Olamma to keep up, you'd need about 150x as much LLM capacity. This would be straightforward to obtain with one or more dedicated appliances.

# The Data Set

The input available for development purposes is a bit more than two weeks of the decahose (about 500 Tweets/second, about 6.5 billion Tweets.) It consists of JSON-formatted Tweets in files that have the file order encoded in the filenames as Unix start and end times. The files are about five minutes of Tweets at about 500 Tweets/second. 

The original files are unpacked in advance into uncompressed CSV files.  

In production the input could be either files (for historical processing) or the actual live Tweet stream (for real time processing).  

For live processing, Z-Filters is entirely bottlenecked by parsing the JSON Tweets. When reading Tweets from disk, it is still I/O bound, but less so.

Using files is not a "cheat" because any system would must receive Tweets and feed them somehow. In a non-demo environment, they could be unpacked as fast as required by dedicated a dedicated server or servers. The actual development feed is from a process running on the same meachine and sending the Tweets in via RabbitMQ. The Z-filters componenent can also read the CSV files directly, for much higher throughput.

# The Major Pieces
The following are the main components of the software.
Instruction for running them can be found in the USER_MANUAL.md
There is a link to a diagram at the top of this file.

## Z-Fiilters: The subjects
This is the key piece that finds new subjects in the firehose.

### JSON Tweet to CSV Tweet Parser
This component parses the original compressed JSON files into a more compact and easily parsed CSV representation.

### Language Utility
The Twitter language field (lang:) is worthless. It apparently only reflects what the user's environment says is the user's language, so it is more often than not either empty or wrong, and users often leave the default English regardless of their actual language.

The optional language utility is a Go program that reads all the CSV files and submits the text to an NLP library that figures out the language. It is quite accurate but very slow, so it exists as a optional separate program.

The resulting Tweet CSV files are just like the originals, except for the language field being corrected and/or populated. You can select whether or not to use the language info in config.yaml regardless of whether the language adujstment has been made.

The config.yaml has a parameter for whether you want to use all languages, or just en, es, et.

### RabbitMQ/MQ Feeder

The main Z-filters processor can take its input Tweets either directly from disk files or from RabbitMQ. The choice is stated in the config.yaml file. Instructions for starting RabbitMQ are in the USER_MANUAL.md
Use of the Feeder is optional.

The RabbitMQ Feeder reads the files (there are about 5700 of them) in time-order, and puts the CSV on RabbitMQ.  It keeps a notation on disk of the last file it processed, so if it is interrupted, it will pick up more or less where it left off.

### Main Processing

The main processor works the same way regardless of whether it reads the Tweet CSV from disk or picks it up line by line from RabbitMQ. It takes the input in batches of several thousand Tweets (config.yaml) figures out the unusually busy words, and finds the recent Tweets that use at least M of them, (this is a very small percentage of all Tweets). It then runs a clustering algorithm on those Tweets to identify the clusters of usage, and writes the resulting clusters of Tweets, as well as metadata about the clusters and the bathces, to standard out as JSON.

This is the heart of the system because it identifies the subjects. It is very fast, operating on up to 50,000 Tweets/second even on a laptop.

This makes the core algorithm is by far the fastest part of the process capable of handling multiples of the firehose, but not easily parallelized except for historical data, which can be parallelized by handing large intervals of time to multiple instances.

#### Persistence of Subjects

In addition to computing the new subjects, the system can give information about how long those new subjects have existed in terms of the number of recent batches the subject or similar subjects appeared in. This is given in the form of a list of the previous N batches that also match the new subject.

Persistence of subjects is a bigger consumer of resources than the cost as the clustering itself.  It can be turned on and off, but the heuristic as a whole is a minor consumer of CPU compared to the unpacking of the input and maintaining the background frequency data structures. Therefore, it is not clear how far back one would have to check for this to be a significant consumer of resources. 

#### Main Processing Viewer
A Web service provides interactive browser access to several different views on the subjects, the words that characterize them, etc.  This web service is available on port 8080 wherever it is running.

Additional viewing modes are easy to add, as they all run on the same basic data structure.

## AI for Subject Semantics

The optional AI portion uses the Ollama LLM running locally to describe the meaning of each cluster/subject. The nature of the AI prompt can be changed programmatically. An upcoming enhancement will allow the format of the prompt to be supplied by the user.

The AI portion is currently set to use a utility to feed the output of the Z-Filters portion to Postgres.  This can easily be switched to feed as the batches are processed.
Unless true real-time analysis is required, it isn't really necessary.

### JSON Main Output Into Postgres

A free-standing utility reads the JSON output from main and inserts it into several tables on Postgress. One of the tables contains the name of the particular run, as well as the values of key parameters that it extracts from the config file.  

This allows the output of any number of runs to exist side by side in Postgres. 
My conventions are:
- Have an override config file with whatever makes the run distinctive and give it a name like ptc_config_sept_7.yaml

- Name output something like sept_7.json

- Call the run something like "sept_7" in Postgres 

This way you have documentation for the run and a way to pull up the corresponding views in the ai_display and the regular display.

The runs don't produce all that much data. A typical JSON output for the two weeks of the decahose is about 25 to 100 MB depending on settins. It could be more.

The amount of Postrgess data space is similar.

So you're probably going to get about four full analysis runs per GB of storage.


### AI/Ollama Service
A second utility program reads the batch/cluster/tweet information from Postgres cluster-by-cluster, formualtes it into prompts, and sends it to Ollama via HTTP POST. Ollama runs locally on its own machine.

The response to the HTTP POST is inserted in Postgres, where it can be queried along with the Tweets and metadata of the cluster/subject.


### AI Web Service and Browser

A web service exists to display the AI results in a browser. The browser is used to select and control what what the web service extracts for display.

Such criteria as the name of the run, the time interval of interest, etc. can be specified.

# Speed of Computation and Significant Events

## Z-Filters Portion
The Z-Filters processing is very fast. A laptop can process a stream of about 8000 to 9000 Tweets/second with the Tweets coming in from Rabbit MQ, and about 45 to 50k if it reads the CSV directly, i.e., as much as ten firehoses.
 
The two-weed Tweet sample was recorded from two weeks of a Decahose. However, the Tweets can be consumed at any speed. 
 
Each CSV file is about five minutes of the decahose, i.e., about 30,000 Tweets per file, or 33 files per million Tweets, which is about 2.7 logical hours of the decahose. 
We are actually processing 2.7 logical hours in about 10 minutes.
 
If you are starting and stopping frequently for development purposes, the system can optionally read in the existing frequency data, for an instant start.  Beware that if you are starting at some random point, the saved frequency data will be wrong to an unpredictable degree because background frequencies evolve as the day goes by and according to new subjects. This means you need to either start fresh or disregard the results until you have run for long enough to completely update the background statistics.

## AI Portion

## JSON to Postgres

The JSON to Postgres step is reasonably fast, being dominated by the time to unpack the JSON. Tweets on new subjects are almost never as much as 1% of the full volume. A batch might represent five seconds of the full firehose and result in 100 to 200 output rows, so moving the data into Postgres is not a heavy load on the database server. 

The AI piece is bottlenecked by access to Ollama, which takes about fifteen seconds per cluster.
This works out to about 1.5 of the decahose rate, whereas the initial load into Postgres could handle several firehose loads.  

However, this can easily be scaled to take advantage of almost any amount of available LLM capacity. For instance, an Nvidia Jetson Orin Nano, which costs about $275 can run the LLM about 10x as fast as the Linux laptop.

As many such appliance as necessary could be used, so it is not clear where the next bottleneck would appear. 


# Miscellaneous Issues and Details
This section describes various processing details.

## The Big Token Window

The refreshing of the background word-frequency data structre is key to how subjects age out. 

Word distributions change thoughout the day and week. People don't Tweet the same things at breakfast on Monday that they do at 1:00 AM on Saturday.  Also, the world turns, and while New Yorkers are getting up in the morning, people in Bejing are going out for the evening.

Equally importantly, surges in usage continually modify the current background frequencies.

These issues are best thought of in terms of the logical time in the Tweets, independently of the actual time to process. In a live field, these are the same, but with stored data, processing can be considerably faster than logical time.

The background frequencies would normally be calculated more frequently than the width of the window. The window might be from twenty minutes to an hour of the feed, while the refresh interval might be every five minutes.

It is not the width of the window that ages subjects out.  Consider that a sudden surge of usage of word w occurs at time t, and w is an unusual word (as is usually the case.)  The word w and some others will be noticed as busywords, and Tweets will be identified in the first batch that w was busy in.  Then the word frequency data structre gets recalculated five minutes later, and because of the surge, w in now in a higher frequency category where it no longer stands out.

Or it might--if the surge is huge, it might be that in the next batch, its frequency exceeds it's new category by a wide enough margin to be noticed again.  So as long as it's frequency continues to increase, it will keep getting noticed. If the frequency plateaus at a higher frequency class, it again drops from view.


### Word Distribution

See "Zipf Distribution" in any reference for details.

Consider that:
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

Nevertheless, use of Z in this way is fairly common for anomaly detection, which is what it is being used for here. The violations of the criteria don't really matter in part be cause we are only identifying the busy words--we don't really need to know exactly how anomalous they are.

The following are excerpted from the logs. Note that the low Z scores are all quite small negative numbers most of which are between -1 and 0, as you'd expect because we've filtered the tokens to get only words that had approximately the same frequency when the filter was created. 
On top of that, we've not added a few words with extraordinary counts, and it's the sprinkling of counters that those words land on that have exceptional counts.

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

The main thread nibbles away at these entries on the queue, removing a bunch of them every few hundred Tweets, and deleting the corresponding entries in the 3pk<->token mapping.  

This is mostly harmless, because if the token ever shows up again, the main thread will automatically recreate it anyway. It is possible, but unlikely, that a mapping will be removed after a set of the corresponding 3pk has been handed off to the busyword processors. This would not happen in steady-state, but can happen after a restart from stored state. If the analysis thread doesn't get back a token for a 3pk, it simply discards it. (If it has any significance, it will show up in the next batch anyway.)

   
# The analyze_tokens Program Output

The **analyze_tokens** program reads CSV files and accumulates the number of distinct tokens that it has encountered.

Running on about 1200 CSV files, which is at least a couple of days of data, shows that the percentage of new tokens rises quickly at first, and then settles down to a pretty stead rate of about one new token for every 3.15 Tweets, or roughly every 31.5 tokens, which is surprisingly high and unwavering.

It is unknown if the full Firehose would behave differently. It could be that most of the diversity at any given moment is contained in a fraction of the Tweets.


 