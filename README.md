# How to Listen to a Million Voices

TEST
TEST
TEST

This project implements a heuristic for discovering new subjects in the X/Twitter firehose in near real time.  

It can far exceed the speed of the firehose on historical data. The analysis is much faster than parsing the JSON. 

For historical data, it makes sense to preprocess it all in advance. There is a multi-threaded utility for doing this. 

The heuristic presented here is capable of extracting the new subjects that people are Tweeting at several times the volume of the full X/Twitter firehose--upward of 45,000 Tweets/second--with a latency and granularity of a single digit seconds, using a minimal single-CPU platform.

## Subjects and Real Time
Our definition of subject is intuitive--a cluster of Tweets that a person would immediately recognize as being about the same thing. We single out a medioid Tweet from each as being the most typical.

We define "real time" as identification of subjects with a latency that is comparable to the time scale required to read and respond to a Tweet. So, something in the range of single digit seconds. 

## Why Bother?
The idea is to 
- Be able to discover a "TV-Guide" for X, sidestepping the need to know about hashtags or similar a-priori mechanism to find out what people are talking about.
- To discover the new subjects before thay appear in conventional news media.
- To be able to process historical data fast enough and cheaply enough to catch up to the present day.
- To reduce the flow to subjects enough that they can be processed for semantic analysis with NLP and/or AI with a reasonable investment in resources.
 
## What's Diffierent Here?
The problem is that by any reasonable definition of "subject" there are thousands, perhaps tens of thousands, at any one time. 
Simply identifying all the current subjects is useless--it is orders of magnitude too much data.

The solution isn't difficult to grasp: present only the new subjects as they appear
  - New subjects arrive at a rate similar to that of the Times Square news ticker.
  - The new ones are the most important to almost everyone.
  - By exposing only new subjects, in the limit, one gets all subjects.
  - If you only see the new subjects, you automatically bypass the old.

It turns out that what we recognize as a subject is well captured by the sudden appearance together of words that are normally unusual. 
This is almost the opposite of our first intuition. Neither "Trump" nor "Taylor Swift" indicate a subject, let alone a new subject, because these words appear every day in countless Tweets on many subjects. 

On the contrary, it is the appearance together of clusters words that are normally unusual or rare that characterizes something different people are talking about: human names, place names, unusual verbs, science terms, economic terms, etc.
Those clusters of word usage drag the very common words with them, not the other way around.

 Fortunately, most words are very rare indeed. 
 In the Tweet stream, the most common 250 words in English are used more often than the other ten or twelve million combined.  

If you can identify the suddenly not-so-rare words fast enough, you can easily discover the clusters of Tweets published in the last few seconds that use several of them.
Finding the subject if just a matter of identifying the Tweets where these words occur among the last several seconds of the stream, then grouping them according the clusters of words that appear in the same Tweets. 
If a few one-in-millions words characterise a cluster of Tweets, the cluster almost certainly constitues a subject people are talking about.

If you knew the relative frequency of every word at a given instant, this would be easy.
You could compare each word's frequency to its relative frequency of a few seconds ago, and with the words in hand, the clusters would be easy to compute.  

The problem is, to make that approach work, you'd need to maintain word counts over millions of tokens, and do a series of global operations to compute and assess changes in relative frequency over them every few thousand Tweets, which means at least every few seconds. 
With a flow of tens of thousands of words per second and a universe of millions of words, that's a tall order.

Fortunately, relative frequency is a red herring.
The only reason you care about relative frequency is so you can identify words that are being used more frequently now than they were a few seconds ago. The critical observation is that it's not the frequency that matters, but the fact that it's surging.

## The Key
It turns out that it is possible to identify words with surging frequency without actually computing relative frequency or doing any other CPU-hungry global operations--you don't even have to check individual words. When you leave out the global computations, the process can be very fast. 

A novel heuristic for doing that seemingly impossible thing is the core of the processing.

The heuristic reduces the computation from being in proportion to the cardinality of the set of words that are in use--millions--to being in proportion to something more like the cardinality of the set of words that are surging--dozens to hundreds. There's a constant, too, but it's modest.

The solution is fast and efficient. A commodity laptop can process at a rate several time that of the 5,000 Tweet/second firehose.

In fact, the job of parsing the JSON to get the fields of interest is several times as CPU intensive as the actual Tweet processing.

## Display Component

This project includes a web-based display component for visualizing the cluster analysis results in real-time. The display provides an interactive web interface for viewing Twitter subject clusters as they are discovered.

### Quick Start

```bash
# Build the display component
cd display
./build.sh

# Run with cluster data
./cursor-twitter-display ../logs/clusters_YYYYMMDD_HHMMSS.txt

# Open browser to http://localhost:8080
```

### Features

- **Real-time visualization** of Twitter subject clusters
- **Batch navigation** through processed data
- **Auto-play mode** for continuous monitoring
- **Grid view** for alternative cluster visualization
- **JSON inspection** of cluster data

For detailed information about the display component, see the [Display Documentation](display/README.md) and [User Manual](USER_MANUAL.md#display-component).
   
