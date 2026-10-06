# Golang Search Engine

A search engine built from scratch in Go: a concurrent web crawler, an indexer that builds an inverted index in PostgreSQL, and a CLI query engine that ranks results by term frequency.

> **Status:** v1.0 complete. v2 is in progress.

## Features

**Crawler**
- Breadth-first traversal with a URL frontier and visited set
- `robots.txt` compliance
- Per-host rate limiting
- URL deduplication
- Concurrent workers using goroutines, channels, and `sync.WaitGroup`
- HTML parsing and link extraction from `href` attributes

**Indexer**
- Tokenizes crawled pages
- Builds an inverted index stored in PostgreSQL

**Query engine**
- CLI-based search
- Retrieves matching pages and ranks them by term frequency

## Architecture

```
            ┌───────────┐     ┌───────────┐     ┌──────────────┐
 seed URLs →│  Crawler  │ →   │  Indexer  │ →   │  PostgreSQL  │
            │ (BFS, 10  │     │ (tokenize,│     │ pages        │
            │  workers) │     │  inverted │     │ inverted_    │
            └───────────┘     │  index)   │     │ index        │
                              └───────────┘     └──────┬───────┘
                                                       │
                                               ┌───────▼───────┐
                                  query  →     │ Query engine  │ → ranked results
                                               │ (TF ranking)  │
                                               └───────────────┘
```

## Project structure

| Directory  | Purpose                                      |
|------------|----------------------------------------------|
| `cmd/`     | Crawler entry point (`main.go`) and search CLI (`search/main.go`) |
| `crawler/` | BFS crawler, frontier, rate limiter, robots  |
| `indexer/` | Tokenization and inverted index construction |
| `search/`  | Query processing and ranking                 |
| `db/`      | PostgreSQL connection and queries            |
| `models/`  | Shared data types                            |

## Getting started

### Prerequisites
- Go 1.21+ (check `go.mod` for the exact version)
- PostgreSQL

### Setup

```bash
git clone https://github.com/mukul-work/golang-search-engine.git
cd golang-search-engine
go mod download
```

Create a database and configure the connection:

```bash
createdb search_engine
# TODO: set your connection string / env var, e.g.
# export DATABASE_URL="postgres://user:password@localhost:5432/search_engine?sslmode=disable"
```

### Usage

**Crawl** (fetches pages and builds the index):

```bash
go run cmd/main.go
```

**Search:**

```bash
go run cmd/search/main.go -n 10 google
```

- `-n` sets the number of results to return
- the last argument is the search query

## Design notes

- **Concurrency:** a fixed pool of workers pulls URLs from the frontier over channels; a `WaitGroup` coordinates shutdown.
- **Politeness:** the crawler respects `robots.txt` and rate-limits requests per host.
- **Storage:** the `pages` table holds crawled documents; `inverted_index` maps terms to pages with frequency data.
- **Ranking (v1):** results are scored by term frequency.

