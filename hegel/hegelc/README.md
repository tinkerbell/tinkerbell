### hegelc

Potentially the home for a more featured `hegel`/metadata client.
For now, a simple binary that connects to `hegel`, subscribes to changes, and prints out line delimited JSON which can be piped to `jq` or other tools.
Would be nice if diffs could be parsed across each line of json...