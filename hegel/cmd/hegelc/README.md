## hegelc

Potentially the home for a more featured `hegel`/metadata client.

For now, a simple binary that connects to `hegel`, subscribes to changes, and prints out line delimited JSON which can be piped to `jq` or other tools.

Would be nice if diffs could be parsed across each line of json...

### Usage

```
$ hegelc -help
Usage of hegelc
  -port int
        The port of the Hegel service [HEGEL_PORT] (default 50060)
  -server string
        The hostname or address of the Hegel service [HEGEL_SERVER] (default "metadata")
```
