## Go Bookmarks CLI

Ultra-fast bookmark manager written in Go with Redis backend and interactive search.

### Features

- **Fast JSON Import**: Import bookmarks from JSON with progress bar
- **Browser Imports**: Import from Chrome, Firefox, Safari, Zen, Arc, or all
- **Sync & Dedupe**: Auto-import from browsers, remove duplicates, rebuild index
- **Interactive Search**: Text, tag, and date filters; quick shortcuts
- **Redis Backend**: Sorted-set index for fast retrieval
- **Duplicate Prevention**: URL set prevents re-ingest

### Prerequisites

- Go 1.21+ (tested on darwin/arm64)
- Redis server running locally (`localhost:6379` by default)

### Install

```bash
git clone <repository-url>
cd Go-Bookmarks-cli
go mod tidy
./scripts/build.sh
```

### Configuration

Environment variables (optional; sensible defaults):

```env
REDIS_ADDR=localhost:6379
REDIS_DB=0
REDIS_PASSWORD=
```

Note: LLM configs referenced in earlier drafts are not required by the current CLI.

### Usage

After build, run the CLI binary:

```bash
./bin/bookmark --help
```

Commands:

- **import**: Import bookmarks from JSON file
  - `./bin/bookmark import <file>`
- **import-html**: Import from exported bookmarks HTML
  - `./bin/bookmark import-html <file>`
- **browser**: Import from a specific browser
  - `./bin/bookmark browser chrome|firefox|safari|zen|arc|all`
- **sync**: Import from all available browsers and deduplicate
  - `./bin/bookmark sync`
- **search**: Interactive search mode
  - `./bin/bookmark search`
- **clean**: Remove duplicate bookmarks
  - `./bin/bookmark clean`

Search shortcuts inside interactive mode:

- `/query` text search in title/description/url
- `#tag` filter by tag(s)
- `@YYYY-MM-DD` date filters (from/to)

### Development

Run directly with arguments:

```bash
./scripts/run-dev.sh search
```

### Project Structure

```
bookmark-cli/
├── cmd/bookmark/main.go
├── internal/
│   ├── browser/browser.go
│   ├── importer/importer.go
│   ├── models/bookmark.go
│   ├── redis/client.go
│   └── searcher/searcher.go
├── scripts/
│   ├── build.sh
│   └── run-dev.sh
├── configs/config.yaml
└── bin/bookmark (built)
```

### Notes

- The repository intentionally excludes committed binaries; builds output to `bin/`.
- Ensure Redis is reachable; default fallback is `localhost:6379`.

### License

MIT License



