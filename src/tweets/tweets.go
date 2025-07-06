package tweets

// ThreePartKey represents a three-part key for a token
type ThreePartKey struct {
	Part1 int
	Part2 int
	Part3 int
}

// Tweet represents a parsed tweet with all its components
type Tweet struct {
	IDStr        string   `json:"id_str"`
	Unix         int64    `json:"unix"`
	UserIDStr    string   `json:"user_id_str"`
	Text         string   `json:"text"`
	Tokens       []string `json:"tokens"`
	Retweeted    bool     `json:"retweeted"`
	RetweetCount int      `json:"retweet_count"`
}
