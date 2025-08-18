
# What This Program Does
This sub-project is a front end for viewing the data produced by the Z-filters processor.

It takes the output of the Z-filters program, which are JSON summaries of batches of clusters of Tweets that constitute new or newly surging subjects. 

The output is obtained by directing the Z-filters program into a .JSON file.
See the main project for how to do this.

# Displaying Subjects
A batch is a set of clusters of Tweets, with each cluster being Tweets on a single subject. Clusters of clusters of Tweets are also identified. A batch might typically be ten to thirty thousand consecutive Tweets. (A few seconds of the full 5000/second firehose.)

Subjects are identified as such by the appearance together of sets of typically unusualy words that are suddenly being used together anomalously often during the interval of the batch. Three or four one-in-a-million words appearing together is multiple Tweets is a strong indicator that people are Tweeting about something.  

Batches are typically of seconds duaration, as defined by the timestamps in the Tweets. Therefore, the time covered by a batch when processing a decahose rate would generally be of an order of maginitude greater duration than a firehose batch.

Each batch is characterized by:
- The time interval it covers
- The set of anomalously busy words that were identified.  
- Zero of more clusters of similar Tweets, with similarity being defined by sharing a subset of the anomalously busy words in the batch.


# Modes of Display
The primary mode of display is the grid display. 

Sets of rows show subject clusters. Each row in the cluster corresponds to a busy word. Threfore, if a cluster is around five busy words, there will be five rows.

The current busy rows are the next-to-leftmost column.

The leftmost column contains the medioid Tweet for the cluster, plus as many other Tweets as fit. So if five busy words define the subject cluster, there will be four additional example Tweets.

To the left, we relate the busy words in the current interval to their appearance or non-appearance in earlier intervals. 
If the same unusual words showd up in recent batches, you will see them appear in the relevant columns. 
A new cluster would usually have no earlier appearances of any of its busy words. A more persistent subject might show up in some or all the earlier batches.  The density of the earlier appearances suggests the significance of the subject.

On the far left are columns tha display the cluster id, the number of Tweets in the cluster, and a computed metric of the significance. Factors in this metric include:
- Over how many batches its words have persisted
- The Levenshtein distances of its mediod Tweet from the Tweets in the earlier batches
- The number of Tweets in the cluster

The metric seems to give a good rough indicator of the significance of the subject, but its a work in progress.

# Sample Data Set
A sample data set produced by the Z-Filters application is available on Google Drive. 

https://drive.google.com/file/d/1CfA6FfaDKBpFuKsk-7StKWuIVbAvFh2T/view?usp=sharing

# Running the Front End
The front end works only from files of output from the Z-filters program.
(See item above.)
It could easily be adapted to work from live data, but would need a slight modification in how it reads the data.

Note that the firehose produces results extremely fast, and Z-Filters can run much faster, so live display is of dubious value.

- Put an output file is a convenient location
- Put that location in the config.yaml file
- If you need to kill an earlier instance use: pkill -f cursor-twitter-display
- Start the Web service with ./cursor-twitter-display
- Connect via browser with either
    - http://localhost:8080/grid
    - http://localhost:8080/grid/n
- the optional argument is the batch number to start with.

Click next repeatedly to see the evolution of new subjects.
- The top Tweet in each display is the medioid Tweet, ie. the most typical Tweet in the subject

# To build cursor-twitter-display

- cd to cursor-twitter/display
- go build -o cursor-twitter-display

This should result in the executable ./cursor-twitter-display

