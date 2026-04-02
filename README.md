# Gator - RSS Feed Aggregator

A command-line RSS feed aggregator written in Go. Gator fetches posts from RSS feeds, stores them in a PostgreSQL database, and lets you browse recent posts right from the terminal.

## Features

- **Feed Management**: Add, list, and manage RSS feeds
- **Feed Following**: Follow feeds to track their posts
- **Automatic Aggregation**: Long-running background process that continuously fetches new posts from your feeds
- **Post Browsing**: View recent posts from all your followed feeds directly in the terminal
- **Duplicate Handling**: Automatically prevents duplicate posts with unique URL constraints
- **Flexible Aggregation Intervals**: Configure how often feeds are fetched (e.g., 1m, 10s, 1h)

## Requirements

Before installing Gator, ensure you have:

- **Go 1.18+** - [Download Go](https://golang.org/doc/install)
- **PostgreSQL 12+** - [Download PostgreSQL](https://www.postgresql.org/download/)

## Installation

### Quick Install

```bash
go install github.com/ach1000/gator@latest
```

This will download, compile, and install the `gator` binary to your `$GOPATH/bin` directory (usually `~/go/bin`). Make sure this directory is in your `$PATH`.

Verify the installation:
```bash
gator help
```

### From Source

Clone the repository and build locally:

```bash
git clone https://github.com/ach1000/gator.git
cd gator
go build
./gator help
```

## Setup

### 1. Create PostgreSQL Database

```bash
createdb gator
```

### 2. Set Up Configuration File

Create a `.gatorconfig.json` file in your home directory:

```bash
touch ~/.gatorconfig.json
```

Add the following content:

```json
{
  "db_url": "postgres://postgres:postgres@localhost:5432/gator",
  "current_user_name": ""
}
```

**Important**: Replace `postgres:postgres` with your PostgreSQL username and password if different.

### 3. Run Database Migrations

```bash
goose -dir sql/schema postgres "postgres://postgres:postgres@localhost:5432/gator" up
```

Or if you've already installed gator, clone the repo to access the migration files and run that command from the gator directory.

## Usage

### Basic Commands

**Register a new user:**
```bash
gator register myusername
```

**Log in:**
```bash
gator login myusername
```

**Add a feed:**
```bash
gator addfeed "TechCrunch" "https://techcrunch.com/feed/"
```

**List all feeds:**
```bash
gator feeds
```

**Follow a feed:**
```bash
gator follow "https://techcrunch.com/feed/"
```

**View your followed feeds:**
```bash
gator following
```

**Browse recent posts (default: 2 posts):**
```bash
gator browse
```

**Browse with custom limit:**
```bash
gator browse 10
```

**Start feed aggregation (fetches every 1 minute):**
```bash
gator agg 1m
```

The aggregation loop will run continuously, fetching feeds on the specified interval. Press `Ctrl+C` to stop.

See all available commands:
```bash
gator help
```

## Aggregation

The `agg` command runs a long-lived loop that automatically fetches your feeds and stores new posts in the database. It uses smart feed rotation to prevent any single feed from starving others.

**Example usage:**

Terminal 1 - Start aggregation (runs in background):
```bash
gator agg 5m
```

Terminal 2 - Browse and interact with posts:
```bash
gator browse 5
gator login otheruser
gator addfeed ...
```

**Recommended Intervals:**
- `1m` - For frequently-updated feeds (news, tech)
- `5m` - General purpose
- `10m` - Less frequently updated feeds
- `1h` - Low-traffic feeds

## Example Feeds

Get started with these popular RSS feeds:

- **TechCrunch**: `https://techcrunch.com/feed/`
- **Hacker News**: `https://news.ycombinator.com/rss`
- **Boot.dev Blog**: `https://www.boot.dev/blog/index.xml`

## Architecture

**Data Model:**
- **Users** - Multiple users can maintain separate feeds and posts
- **Feeds** - RSS feed URLs added by users
- **Feed Follows** - User subscriptions to feeds
- **Posts** - Individual items fetched from feeds

**Database:** PostgreSQL with sqlc for type-safe SQL queries

**Aggregation Strategy:** Feeds are fetched in rotation, with unfetched feeds prioritized and then oldest-first to prevent starvation.

## Development

For development and debugging, see `PROJECT.md` in the repository for implementation details, architecture notes, and testing procedures.

## License

This project is open source and available for educational purposes.

## Contributing

Issues and pull requests welcome! For major changes, please open an issue first to discuss.

---

**Built with Go, PostgreSQL, and sqlc**
