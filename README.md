# How to Listen to a Million Voices

This project implements a heuristic for discovering new subjects in the X/Twitter firehose in near real time. 
The heuristic presented here is capable of extracting the new subjects people are Tweeting about at the volume
of the full X/Twitter firehose--upward of 5000 Tweets/second--with a latency of a few seconds, using a 
minimal single-CPU platform.

Our definition of subject is intuitive--a cluster of Tweets that a person would immediately recognize as 
being about the same thing.

We define "real time" as identification of subjects with a latency that is comparable to the time scale 
required to read and respond to a Tweet. So, something in the range of single digit seconds. 

The idea is to 
- Be able to present a "TV-Guide" for X, sidestepping the need to know about hashtags or similar a-priori mechanism.
- To discover things people are Tweeting much faster than they can appear on conventional news.
- To be able to process historical data fast enough and cheaply enough to catch up to the present day.
- To reduce the flow to subjects that can be processed with AI with a reasonable investment in resources.
 
The problem is that by any reasonable definition of "subject" there are thousands, perhaps tens of thousands at 
any one time, with tens of millions of people send messages every day. Simply identifying all the current subjects
is uselss--it's orders of magnitude too much data.

The solution isn't difficult to grasp: present only the new subjects as they appear
  - The flow of new subjects turns out to have a data rate similar to that of the Times Square news ticker.
  - The new ones are the most important to almost everyone.
  - By exposing only new subjects, in the limit, one gets all subjects.
  - If you only see the new subjects, you automatically bypass the old.

It turns out that what we recognize as a subject is well captured by the sudden appearance togtether of words that are 
normally unusual. It's almost the opposite of our intuition. Neither "Trump" nor "Taylor Swift" indicate a subject,
let alone a new subject. These words appear every day in countless Tweets on many subjects. On the contrary, it is the 
appearance together of clusters words that are normally rare that characterize something different people are talking
about. Those clusters drag the very common words with them, not the other way around.

Fortunately, most words are very rare indeed. The most common 250 words are used more often than the least common 
10,000,000 combined.  

If you can identify the suddenly not-so-rare words fast enough, you can easily discover the clusters of Tweets published in 
the last few seconds that use several of them. It's a simple principle a cluster of Tweets suddenly useing the same set 
of several one-in-millions words almost certainly constitues a subject people are talking about.

It's simple in theory, yet not trivial to do fast enough.  

To make it work, you have to identify surges in frequency of rare words within seconds. It's almost impossible to do a
fresh analysis of relative frequency fast enough with a flow of several thousand Tweets per second and a universe of 
ten to twenty million words.

The trick is to identify surges in frequency without actually doing frequency analysis on millions of words. 
A novel heuristic for doing that is the core of the processing.


   
