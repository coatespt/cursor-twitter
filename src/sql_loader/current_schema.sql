-- Current database schema for cursor-twitter project
-- Generated from live database on 2025-01-09

-- Experiment runs table
CREATE TABLE new_experiment_runs (
    run_id integer NOT NULL DEFAULT nextval('new_experiment_runs_run_id_seq'::regclass),
    run_name text NOT NULL,
    run_date_time timestamp with time zone DEFAULT now(),
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT new_experiment_runs_pkey PRIMARY KEY (run_id),
    CONSTRAINT new_experiment_runs_run_name_key UNIQUE (run_name)
);

-- Batches table
CREATE TABLE new_batches (
    id integer NOT NULL DEFAULT nextval('new_batches_id_seq'::regclass),
    run_id integer NOT NULL,
    batch_number integer NOT NULL,
    batch_time timestamp with time zone NOT NULL,
    method character varying(50) NOT NULL,
    total_tweets integer NOT NULL,
    total_clusters integer NOT NULL,
    clusters_above_min_size integer NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT new_batches_pkey PRIMARY KEY (id),
    CONSTRAINT new_batches_run_id_batch_number_key UNIQUE (run_id, batch_number),
    CONSTRAINT new_batches_run_id_fkey FOREIGN KEY (run_id) REFERENCES new_experiment_runs(run_id) ON DELETE CASCADE
);

-- Clusters table
CREATE TABLE new_clusters (
    cluster_id integer NOT NULL DEFAULT nextval('new_clusters_cluster_id_seq'::regclass),
    batch_id integer NOT NULL,
    size integer NOT NULL,
    quality_score numeric(5,4),
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT new_clusters_pkey PRIMARY KEY (cluster_id),
    CONSTRAINT new_clusters_batch_id_fkey FOREIGN KEY (batch_id) REFERENCES new_batches(id) ON DELETE CASCADE
);

-- Tweets table
CREATE TABLE new_tweets (
    tweet_id integer NOT NULL DEFAULT nextval('new_tweets_tweet_id_seq'::regclass),
    tweet_text text NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT new_tweets_pkey PRIMARY KEY (tweet_id)
);

-- Tweet-cluster relationship table
CREATE TABLE new_tweet_clusters (
    tweet_id integer NOT NULL,
    cluster_id integer NOT NULL,
    tweet_order integer NOT NULL,
    is_medoid boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT new_tweet_clusters_pkey PRIMARY KEY (tweet_id, cluster_id),
    CONSTRAINT new_tweet_clusters_cluster_id_tweet_order_key UNIQUE (cluster_id, tweet_order),
    CONSTRAINT new_tweet_clusters_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES new_clusters(cluster_id) ON DELETE CASCADE,
    CONSTRAINT new_tweet_clusters_tweet_id_fkey FOREIGN KEY (tweet_id) REFERENCES new_tweets(tweet_id) ON DELETE CASCADE
);

-- Busy words table
CREATE TABLE new_busy_words (
    id integer NOT NULL DEFAULT nextval('new_busy_words_id_seq'::regclass),
    cluster_id integer NOT NULL,
    word text NOT NULL,
    word_order integer NOT NULL,
    frequency_class integer NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT new_busy_words_pkey PRIMARY KEY (id),
    CONSTRAINT new_busy_words_cluster_id_word_order_key UNIQUE (cluster_id, word_order),
    CONSTRAINT new_busy_words_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES new_clusters(cluster_id) ON DELETE CASCADE
);

-- Indexes
CREATE UNIQUE INDEX idx_new_tweet_clusters_one_medoid_per_cluster ON new_tweet_clusters (cluster_id) WHERE is_medoid = true;
