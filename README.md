# go-nate
CLI tool to dump, index and search bookmarks

## Motivation

The idea behind this tool is to make browser's bookmarks searchable by their category, title, and content. It could be done in 3 steps: `dump`, `index` and then run a webserver with available UI to search across the bookmarks.

## Installation

Currently, the CLI tool could be installed using `go install` or docker capabilities.

### Local

Clone the repository:
```bash
git clone https://github.com/Neurostep/go-nate.git && cd go-nate/
```

Run the following command:
```bash
go install github.com/Neurostep/go-nate
```

`go-nate` binary will be installed in `$GOPATH/bin` or `$GOBIN` if specified

### Docker

To pull docker image run the following:

```bash
docker pull neurostep/go-nate:latest
```

`go-nate` while running stores logs and bookmark data (content, index information etc) locally. In order to make it
persistent and accessible from the host we need to mount local directories to docker container, for example:

```bash
docker run -it -v "${HOME}/.gonate/index:/root/index" -v "${HOME}/.gonate/db:/root/db" -v "${HOME}/.gonate/index:/root/index" -v "${HOME}/.gonate/log:/root/log" -v "${HOME}/Library/Application Support/Google/Chrome:/root/bookmarks" neurostep/go-nate:latest go-nate dump -f bookmarks
```

## Usage

By default, `go-nate` stores all auxiliary data in "${HOME}/.gonate/" directory.
This can be changed by specifying environment variable `GONATE_HOME`.

```bash
go-nate --help

Usage: go-nate [flags] <command> [<args>]

Commands:
    dump      Saves bookmarks for the specified browser to the local DB. If bookmark URL is provided, it will dump that one only
    index     Indexes bookmarks from DB. If 'bookmark url' is provided, it will index only that bookmark
    watch     Runs a background check for the bookmark file change
    server    Runs HTTP server on provided port
    repl      Starts the go-nate REPL

Flags:
  --d  Turn on debug mode
  --i  Path to directory containing search index
  --l  Path to directory containing logs
  --s  Path to directory containing database files
```

### Repl

```bash
go-nate repl --help

USAGE
  go-nate repl
```

`go-nate repl` will start the very simple REPL with prompt `go-nate> `.
REPL supports following commands:

1. `search <here goes search string>` - will run the [Query Search](https://blevesearch.com/docs/Query-String-Query/)
    against the index. Index should be present.
2. `set search searchResultSize <number>` - specifies the number of result for search command. Default number is `10`

Examples:

```
go-nate repl
go-nate> search golang
go-nate> set search searchResultSize 2
```
### Dump

```bash
go-nate dump --help

USAGE
  go-nate dump [-f path] [-b browser] [-p profile] [-c concurrency] [-F force to dump] [bookmark url] [bookmark folder] [bookmark title]

FLAGS
  -F false                                                       If provided, then bookmark will be dumped even if it already exists
  -b chrome                                                      Browser for which bookmarks are being dumped
  -c 100                                                         Number of concurrent workers to dump the bookmarks
  -f ${HOME}/Library/Application Support/Google/Chrome           The path to local browser profile
  -p default                                                     The profile name of the browser
```

`go-nate dump` command does the following:

1. reads provided browser's bookmark file
1. tries to retrieve data for the bookmark using Go `http` library first
1. if it failed to retrieve content of the bookmark using Go `http` library, it will try to do that using Chrome web browser
1. then it stores content locally using [Badger DB](https://github.com/dgraph-io/badger)

### Index

```bash
go-nate index --help

USAGE
  go-nate index [bookmark url]
```

For the `index` command it's required that `dump` step previously done. `go-nate index` will go over dumped data and will
index that data using [Bleve search engine](http://blevesearch.com/)

### Server

```bash
go-nate server --help

USAGE
  go-nate server [-p port]

FLAGS
  -p 8080  Number represents the port server will listen to
```

Will spin-up the server. Navigate to `http://localhost:8080/search/syntax/` and try search your bookmarks!

### Watch

```bash
go-nate watch --help

USAGE
  go-nate watch [-i interval] [-f path] [-b browser] [-p profile]

FLAGS
  -b chrome                                                      Browser for which bookmarks are being watched and dumped
  -f ${HOME}/Library/Application Support/Google/Chrome           The path to local browser profile
  -i 30s                                                         The interval in which watch will perform the bookmark file check
  -p default                                                     The profile name of the browser
```

This command runs a background job which will be checking the provided bookmarks file for the update and run `dump` and
`index` automatically.

## Requirements

To run `go-nate` locally there are following requirements:

 - Go Lang version 1.16+
 - NodeJS version 16.3+
 - Chrome Browser (version 91 tested)

There is also possibility to run `go-nate` using docker.
