# Go Bookmarks CLI

Ultra-fast bookmark manager with LLM capabilities built in Go.

## Features

- **Fast Import**: Import bookmarks from JSON files with progress tracking
- **Interactive Search**: Search through bookmarks with various filters
- **Redis Backend**: Uses Redis for ultra-fast storage and retrieval
- **Duplicate Detection**: Automatically prevents duplicate bookmarks
- **Tag Support**: Organize bookmarks with tags
- **Date Filtering**: Filter bookmarks by creation date

## Prerequisites

- Go 1.19 or later
- Redis server (configured to use production Redis at `192.168.1.100:6379`)

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd Go-Bookmarks-cli
```

2. Install dependencies:
```bash
go mod tidy
```

3. Build the application:
```bash
./scripts/build.sh
```

## Configuration

The application uses environment variables for configuration. Update the `.env` file:

```env
REDIS_ADDR=192.168.1.100:6379
REDIS_DB=1
REDIS_PASSWORD=
LLM_API_KEY=your_openai_key_here
LLM_MODEL=gpt-4
DEBUG=true
```

## Usage

### Import Bookmarks

Import bookmarks from a JSON file:

```bash
./bin/bookmark import test-bookmarks.json
```

### Search Bookmarks

Start interactive search mode:

```bash
./bin/bookmark search
```

Search shortcuts:
- `/query` - Text search
- `#tag` - Filter by tags
- `@date` - Filter by date range
- `!llm` - Enable LLM processing

Examples:
```bash
> /golang programming
> #database #redis
> @2023-01-01 @2023-12-31
```

### Clean Duplicates

Remove duplicate bookmarks:

```bash
./bin/bookmark clean
```

## Development

Run in development mode:

```bash
./scripts/run-dev.sh
```

## Project Structure

```
bookmark-cli/
├── cmd/bookmark/main.go          # Main CLI application
├── internal/
│   ├── models/bookmark.go        # Bookmark data model
│   ├── redis/client.go           # Redis client configuration
│   ├── importer/importer.go      # Bookmark import functionality
│   └── searcher/searcher.go      # Search functionality
├── configs/config.yaml           # Configuration file
├── scripts/
│   ├── build.sh                  # Build script
│   └── run-dev.sh                # Development run script
└── test-bookmarks.json           # Sample bookmark data
```

## Testing

Test the application with sample data:

```bash
# Import test bookmarks
./bin/bookmark import test-bookmarks.json

# Search for specific bookmarks
echo "golang" | ./bin/bookmark search
```

## License

MIT License



