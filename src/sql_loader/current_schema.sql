--
-- PostgreSQL database dump
--

-- Dumped from database version 14.6 (Homebrew)
-- Dumped by pg_dump version 14.6 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: ai_analysis_results; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.ai_analysis_results (
    result_id integer NOT NULL,
    session_id integer NOT NULL,
    cluster_id integer NOT NULL,
    prompt_text text NOT NULL,
    response_text text NOT NULL,
    response_metadata jsonb,
    analysis_metadata jsonb,
    created_at timestamp with time zone DEFAULT now(),
    processing_time_ms integer
);


ALTER TABLE public.ai_analysis_results OWNER TO petercoates;

--
-- Name: TABLE ai_analysis_results; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.ai_analysis_results IS 'Individual AI analysis requests and responses for clusters';


--
-- Name: COLUMN ai_analysis_results.response_metadata; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.ai_analysis_results.response_metadata IS 'JSON metadata from AI response (tokens, timing, etc.)';


--
-- Name: COLUMN ai_analysis_results.analysis_metadata; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.ai_analysis_results.analysis_metadata IS 'Structured data extracted from AI response';


--
-- Name: ai_analysis_results_result_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.ai_analysis_results_result_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.ai_analysis_results_result_id_seq OWNER TO petercoates;

--
-- Name: ai_analysis_results_result_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.ai_analysis_results_result_id_seq OWNED BY public.ai_analysis_results.result_id;


--
-- Name: ai_analysis_sessions; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.ai_analysis_sessions (
    session_id integer NOT NULL,
    run_id integer NOT NULL,
    session_name text NOT NULL,
    ai_model character varying(100) NOT NULL,
    ai_endpoint character varying(255) NOT NULL,
    prompt_template text NOT NULL,
    analysis_type character varying(50) NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    completed_at timestamp with time zone,
    status character varying(20) DEFAULT 'running'::character varying,
    total_clusters integer DEFAULT 0,
    processed_clusters integer DEFAULT 0,
    failed_clusters integer DEFAULT 0
);


ALTER TABLE public.ai_analysis_sessions OWNER TO petercoates;

--
-- Name: TABLE ai_analysis_sessions; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.ai_analysis_sessions IS 'Tracks AI analysis sessions for experiment runs';


--
-- Name: COLUMN ai_analysis_sessions.ai_model; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.ai_analysis_sessions.ai_model IS 'The AI model used for analysis (e.g., llama3.1:8b)';


--
-- Name: COLUMN ai_analysis_sessions.prompt_template; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.ai_analysis_sessions.prompt_template IS 'Template used to generate prompts for clusters';


--
-- Name: ai_analysis_sessions_session_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.ai_analysis_sessions_session_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.ai_analysis_sessions_session_id_seq OWNER TO petercoates;

--
-- Name: ai_analysis_sessions_session_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.ai_analysis_sessions_session_id_seq OWNED BY public.ai_analysis_sessions.session_id;


--
-- Name: ai_insights; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.ai_insights (
    insight_id integer NOT NULL,
    result_id integer NOT NULL,
    insight_type character varying(50) NOT NULL,
    insight_value text NOT NULL,
    confidence_score numeric(3,2),
    created_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.ai_insights OWNER TO petercoates;

--
-- Name: TABLE ai_insights; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.ai_insights IS 'Structured insights extracted from AI analysis responses';


--
-- Name: COLUMN ai_insights.confidence_score; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.ai_insights.confidence_score IS 'AI confidence in this insight (0.0-1.0)';


--
-- Name: ai_insights_insight_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.ai_insights_insight_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.ai_insights_insight_id_seq OWNER TO petercoates;

--
-- Name: ai_insights_insight_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.ai_insights_insight_id_seq OWNED BY public.ai_insights.insight_id;


--
-- Name: new_batches; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.new_batches (
    id integer NOT NULL,
    run_id integer NOT NULL,
    batch_number integer NOT NULL,
    batch_time timestamp with time zone NOT NULL,
    method character varying(50) NOT NULL,
    total_tweets integer NOT NULL,
    total_clusters integer NOT NULL,
    clusters_above_min_size integer NOT NULL,
    created_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.new_batches OWNER TO petercoates;

--
-- Name: TABLE new_batches; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.new_batches IS 'Metadata for each batch processed by the pipeline';


--
-- Name: new_batches_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.new_batches_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.new_batches_id_seq OWNER TO petercoates;

--
-- Name: new_batches_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.new_batches_id_seq OWNED BY public.new_batches.id;


--
-- Name: new_busy_words; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.new_busy_words (
    id integer NOT NULL,
    cluster_id integer NOT NULL,
    word text NOT NULL,
    word_order integer NOT NULL,
    frequency_class integer NOT NULL,
    z_score numeric(10,6),
    count integer,
    mean numeric(10,6),
    created_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.new_busy_words OWNER TO petercoates;

--
-- Name: TABLE new_busy_words; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.new_busy_words IS 'Busy words identified in each cluster with their frequency classes';


--
-- Name: COLUMN new_busy_words.word; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.new_busy_words.word IS 'Busy word identified in the cluster';


--
-- Name: COLUMN new_busy_words.frequency_class; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.new_busy_words.frequency_class IS 'Frequency class of this word in this batch';


--
-- Name: new_busy_words_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.new_busy_words_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.new_busy_words_id_seq OWNER TO petercoates;

--
-- Name: new_busy_words_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.new_busy_words_id_seq OWNED BY public.new_busy_words.id;


--
-- Name: new_clusters; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.new_clusters (
    id integer NOT NULL,
    batch_id integer NOT NULL,
    cluster_id integer NOT NULL,
    size integer NOT NULL,
    quality_score numeric(5,4),
    created_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.new_clusters OWNER TO petercoates;

--
-- Name: TABLE new_clusters; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.new_clusters IS 'Cluster information extracted from each batch';


--
-- Name: COLUMN new_clusters.quality_score; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.new_clusters.quality_score IS 'Computed quality score for the cluster';


--
-- Name: new_clusters_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.new_clusters_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.new_clusters_id_seq OWNER TO petercoates;

--
-- Name: new_clusters_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.new_clusters_id_seq OWNED BY public.new_clusters.id;


--
-- Name: new_experiment_runs; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.new_experiment_runs (
    run_id integer NOT NULL,
    run_name text NOT NULL,
    run_date_time timestamp with time zone DEFAULT now(),
    created_at timestamp with time zone DEFAULT now(),
    window_size integer,
    batch_size integer,
    freq_classes integer,
    min_jaccard_similarity numeric(3,2),
    bw_array_len integer,
    z_scores text,
    min_busy_words_per_tweet integer,
    duplicate_similarity_threshold numeric(3,2),
    language_filter character varying(10),
    use_medoid_similarity boolean,
    use_busy_word_similarity boolean,
    medoid_similarity_threshold numeric(3,2),
    busy_word_similarity_threshold numeric(3,2),
    min_token_len integer
);


ALTER TABLE public.new_experiment_runs OWNER TO petercoates;

--
-- Name: TABLE new_experiment_runs; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.new_experiment_runs IS 'Experimental run configuration and metadata';


--
-- Name: new_experiment_runs_run_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.new_experiment_runs_run_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.new_experiment_runs_run_id_seq OWNER TO petercoates;

--
-- Name: new_experiment_runs_run_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.new_experiment_runs_run_id_seq OWNED BY public.new_experiment_runs.run_id;


--
-- Name: new_tweet_clusters; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.new_tweet_clusters (
    id integer NOT NULL,
    tweet_id integer NOT NULL,
    cluster_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.new_tweet_clusters OWNER TO petercoates;

--
-- Name: TABLE new_tweet_clusters; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.new_tweet_clusters IS 'Many-to-many relationship between tweets and clusters';


--
-- Name: new_tweet_clusters_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.new_tweet_clusters_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.new_tweet_clusters_id_seq OWNER TO petercoates;

--
-- Name: new_tweet_clusters_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.new_tweet_clusters_id_seq OWNED BY public.new_tweet_clusters.id;


--
-- Name: new_tweets; Type: TABLE; Schema: public; Owner: petercoates
--

CREATE TABLE public.new_tweets (
    id integer NOT NULL,
    cluster_id integer NOT NULL,
    tweet_id_str text NOT NULL,
    unix_timestamp bigint NOT NULL,
    created_at_tweet timestamp with time zone,
    user_id_str text,
    tweet_text text NOT NULL,
    retweeted boolean DEFAULT false,
    retweet_count integer DEFAULT 0,
    lang character varying(10),
    batch_id integer,
    tweet_order integer NOT NULL,
    is_medoid boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.new_tweets OWNER TO petercoates;

--
-- Name: TABLE new_tweets; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON TABLE public.new_tweets IS 'Individual tweets within each cluster';


--
-- Name: COLUMN new_tweets.is_medoid; Type: COMMENT; Schema: public; Owner: petercoates
--

COMMENT ON COLUMN public.new_tweets.is_medoid IS 'Marks this tweet as the cluster medoid';


--
-- Name: new_tweets_id_seq; Type: SEQUENCE; Schema: public; Owner: petercoates
--

CREATE SEQUENCE public.new_tweets_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.new_tweets_id_seq OWNER TO petercoates;

--
-- Name: new_tweets_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: petercoates
--

ALTER SEQUENCE public.new_tweets_id_seq OWNED BY public.new_tweets.id;


--
-- Name: ai_analysis_results result_id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_results ALTER COLUMN result_id SET DEFAULT nextval('public.ai_analysis_results_result_id_seq'::regclass);


--
-- Name: ai_analysis_sessions session_id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_sessions ALTER COLUMN session_id SET DEFAULT nextval('public.ai_analysis_sessions_session_id_seq'::regclass);


--
-- Name: ai_insights insight_id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_insights ALTER COLUMN insight_id SET DEFAULT nextval('public.ai_insights_insight_id_seq'::regclass);


--
-- Name: new_batches id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_batches ALTER COLUMN id SET DEFAULT nextval('public.new_batches_id_seq'::regclass);


--
-- Name: new_busy_words id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_busy_words ALTER COLUMN id SET DEFAULT nextval('public.new_busy_words_id_seq'::regclass);


--
-- Name: new_clusters id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_clusters ALTER COLUMN id SET DEFAULT nextval('public.new_clusters_id_seq'::regclass);


--
-- Name: new_experiment_runs run_id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_experiment_runs ALTER COLUMN run_id SET DEFAULT nextval('public.new_experiment_runs_run_id_seq'::regclass);


--
-- Name: new_tweet_clusters id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweet_clusters ALTER COLUMN id SET DEFAULT nextval('public.new_tweet_clusters_id_seq'::regclass);


--
-- Name: new_tweets id; Type: DEFAULT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweets ALTER COLUMN id SET DEFAULT nextval('public.new_tweets_id_seq'::regclass);


--
-- Name: ai_analysis_results ai_analysis_results_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_results
    ADD CONSTRAINT ai_analysis_results_pkey PRIMARY KEY (result_id);


--
-- Name: ai_analysis_results ai_analysis_results_session_id_cluster_id_key; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_results
    ADD CONSTRAINT ai_analysis_results_session_id_cluster_id_key UNIQUE (session_id, cluster_id);


--
-- Name: ai_analysis_sessions ai_analysis_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_sessions
    ADD CONSTRAINT ai_analysis_sessions_pkey PRIMARY KEY (session_id);


--
-- Name: ai_analysis_sessions ai_analysis_sessions_run_id_session_name_key; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_sessions
    ADD CONSTRAINT ai_analysis_sessions_run_id_session_name_key UNIQUE (run_id, session_name);


--
-- Name: ai_insights ai_insights_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_insights
    ADD CONSTRAINT ai_insights_pkey PRIMARY KEY (insight_id);


--
-- Name: new_batches new_batches_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_batches
    ADD CONSTRAINT new_batches_pkey PRIMARY KEY (id);


--
-- Name: new_batches new_batches_run_id_batch_number_key; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_batches
    ADD CONSTRAINT new_batches_run_id_batch_number_key UNIQUE (run_id, batch_number);


--
-- Name: new_busy_words new_busy_words_cluster_id_word_order_key; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_busy_words
    ADD CONSTRAINT new_busy_words_cluster_id_word_order_key UNIQUE (cluster_id, word_order);


--
-- Name: new_busy_words new_busy_words_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_busy_words
    ADD CONSTRAINT new_busy_words_pkey PRIMARY KEY (id);


--
-- Name: new_clusters new_clusters_batch_id_cluster_id_key; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_clusters
    ADD CONSTRAINT new_clusters_batch_id_cluster_id_key UNIQUE (batch_id, cluster_id);


--
-- Name: new_clusters new_clusters_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_clusters
    ADD CONSTRAINT new_clusters_pkey PRIMARY KEY (id);


--
-- Name: new_experiment_runs new_experiment_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_experiment_runs
    ADD CONSTRAINT new_experiment_runs_pkey PRIMARY KEY (run_id);


--
-- Name: new_experiment_runs new_experiment_runs_run_name_key; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_experiment_runs
    ADD CONSTRAINT new_experiment_runs_run_name_key UNIQUE (run_name);


--
-- Name: new_tweet_clusters new_tweet_clusters_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweet_clusters
    ADD CONSTRAINT new_tweet_clusters_pkey PRIMARY KEY (id);


--
-- Name: new_tweet_clusters new_tweet_clusters_tweet_id_cluster_id_key; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweet_clusters
    ADD CONSTRAINT new_tweet_clusters_tweet_id_cluster_id_key UNIQUE (tweet_id, cluster_id);


--
-- Name: new_tweets new_tweets_cluster_id_tweet_order_key; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweets
    ADD CONSTRAINT new_tweets_cluster_id_tweet_order_key UNIQUE (cluster_id, tweet_order);


--
-- Name: new_tweets new_tweets_pkey; Type: CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweets
    ADD CONSTRAINT new_tweets_pkey PRIMARY KEY (id);


--
-- Name: idx_ai_analysis_results_cluster_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_ai_analysis_results_cluster_id ON public.ai_analysis_results USING btree (cluster_id);


--
-- Name: idx_ai_analysis_results_session_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_ai_analysis_results_session_id ON public.ai_analysis_results USING btree (session_id);


--
-- Name: idx_ai_analysis_sessions_run_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_ai_analysis_sessions_run_id ON public.ai_analysis_sessions USING btree (run_id);


--
-- Name: idx_new_batches_batch_number; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_batches_batch_number ON public.new_batches USING btree (batch_number);


--
-- Name: idx_new_batches_batch_time; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_batches_batch_time ON public.new_batches USING btree (batch_time);


--
-- Name: idx_new_batches_run_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_batches_run_id ON public.new_batches USING btree (run_id);


--
-- Name: idx_new_busy_words_cluster_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_busy_words_cluster_id ON public.new_busy_words USING btree (cluster_id);


--
-- Name: idx_new_busy_words_frequency_class; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_busy_words_frequency_class ON public.new_busy_words USING btree (frequency_class);


--
-- Name: idx_new_busy_words_word; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_busy_words_word ON public.new_busy_words USING btree (word);


--
-- Name: idx_new_clusters_batch_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_clusters_batch_id ON public.new_clusters USING btree (batch_id);


--
-- Name: idx_new_clusters_cluster_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_clusters_cluster_id ON public.new_clusters USING btree (cluster_id);


--
-- Name: idx_new_tweet_clusters_cluster_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_tweet_clusters_cluster_id ON public.new_tweet_clusters USING btree (cluster_id);


--
-- Name: idx_new_tweet_clusters_tweet_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_tweet_clusters_tweet_id ON public.new_tweet_clusters USING btree (tweet_id);


--
-- Name: idx_new_tweets_cluster_id; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE INDEX idx_new_tweets_cluster_id ON public.new_tweets USING btree (cluster_id);


--
-- Name: idx_new_tweets_one_medoid_per_cluster; Type: INDEX; Schema: public; Owner: petercoates
--

CREATE UNIQUE INDEX idx_new_tweets_one_medoid_per_cluster ON public.new_tweets USING btree (cluster_id) WHERE (is_medoid = true);


--
-- Name: ai_analysis_results ai_analysis_results_cluster_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_results
    ADD CONSTRAINT ai_analysis_results_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES public.new_clusters(id) ON DELETE CASCADE;


--
-- Name: ai_analysis_results ai_analysis_results_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_results
    ADD CONSTRAINT ai_analysis_results_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.ai_analysis_sessions(session_id) ON DELETE CASCADE;


--
-- Name: ai_analysis_sessions ai_analysis_sessions_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_analysis_sessions
    ADD CONSTRAINT ai_analysis_sessions_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.new_experiment_runs(run_id) ON DELETE CASCADE;


--
-- Name: ai_insights ai_insights_result_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.ai_insights
    ADD CONSTRAINT ai_insights_result_id_fkey FOREIGN KEY (result_id) REFERENCES public.ai_analysis_results(result_id) ON DELETE CASCADE;


--
-- Name: new_batches new_batches_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_batches
    ADD CONSTRAINT new_batches_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.new_experiment_runs(run_id) ON DELETE CASCADE;


--
-- Name: new_busy_words new_busy_words_cluster_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_busy_words
    ADD CONSTRAINT new_busy_words_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES public.new_clusters(id) ON DELETE CASCADE;


--
-- Name: new_clusters new_clusters_batch_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_clusters
    ADD CONSTRAINT new_clusters_batch_id_fkey FOREIGN KEY (batch_id) REFERENCES public.new_batches(id) ON DELETE CASCADE;


--
-- Name: new_tweet_clusters new_tweet_clusters_cluster_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweet_clusters
    ADD CONSTRAINT new_tweet_clusters_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES public.new_clusters(id) ON DELETE CASCADE;


--
-- Name: new_tweet_clusters new_tweet_clusters_tweet_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweet_clusters
    ADD CONSTRAINT new_tweet_clusters_tweet_id_fkey FOREIGN KEY (tweet_id) REFERENCES public.new_tweets(id) ON DELETE CASCADE;


--
-- Name: new_tweets new_tweets_cluster_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: petercoates
--

ALTER TABLE ONLY public.new_tweets
    ADD CONSTRAINT new_tweets_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES public.new_clusters(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

