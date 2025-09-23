module cursor-twitter/sql_loader

go 1.24.4

require (
	github.com/lib/pq v1.10.9
	gopkg.in/yaml.v3 v3.0.1
)

require cursor-twitter/json_parser v0.0.0

require cursor-twitter-display v0.0.0

replace cursor-twitter/json_parser => ../json_parser

replace cursor-twitter-display => ../../display
