# SlackCompose

Run Docker Compose commands from Slack

## Overview

SlackCompose is a Go service that enables running Docker Compose commands from Slack. It integrates with:
- **SlackCommandRelay** - receives `/slack-compose` commands from Slack
- **Poppit** - executes docker compose commands
- **SlackLiner** - posts command output to Slack channels
- **SlackRelay** - receives emoji reaction events from Slack
- **Redis** - for pub/sub messaging and list-based queues between services

## Architecture Flow

```
User in Slack → /slack-compose <project>
       ↓
SlackCommandRelay → Redis Pub/Sub (slack-commands)
       ↓
SlackCompose → Redis List RPUSH (poppit:notifications)
       ↓
Poppit executes: docker compose ps
       ↓
Poppit → Redis Pub/Sub (poppit:command-output)
       ↓
SlackCompose → Redis List RPUSH (slack_messages)
       ↓
SlackLiner posts to Slack #slack-compose channel

User reacts with ⬆️/⬇️/🔄/📄
       ↓
SlackRelay → Redis Pub/Sub (slack-reactions)
       ↓
SlackCompose → Slack API (fetch message metadata)
       ↓
SlackCompose → Redis List RPUSH (poppit:notifications)
       ↓
Poppit executes: docker compose up/down/restart/logs
```

## Features

- Execute `docker compose ps` via Slack command `/slack-compose <project-name>`
- Control projects via emoji reactions:
  - ⬆️ (up_arrow) - runs `docker compose up -d`
  - ⬇️ (down_arrow) - runs `docker compose down`
  - 🔄 (arrows_counterclockwise) - runs `docker compose restart`
  - 📄 (page_facing_up) - runs `docker compose logs`
- Project configuration via JSON file
- Built with scratch Docker image for minimal size

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REDIS_ADDR` | Redis server address | `localhost:6379` |
| `REDIS_PASSWORD` | Redis password | (empty) |
| `REDIS_DB` | Redis database number | `0` |
| `SLACK_COMMAND_CHANNEL` | Redis Pub/Sub channel for Slack commands | `slack-commands` |
| `SLACK_REACTION_CHANNEL` | Redis Pub/Sub channel for Slack reactions | `slack-reactions` |
| `POPPIT_LIST_NAME` | Redis list name for Poppit notifications | `poppit:notifications` |
| `POPPIT_OUTPUT_CHANNEL` | Redis Pub/Sub channel for Poppit command output | `poppit:command-output` |
| `SLACKLINER_LIST_NAME` | Redis list name for SlackLiner messages | `slack_messages` |
| `SLACK_TOKEN` | Slack API token (required) | - |
| `SLACK_CHANNEL` | Slack channel to post to | `#slack-compose` |
| `PROJECT_CONFIG_PATH` | Path to projects configuration file | `projects.json` |
| `LOG_LEVEL` | Logging level: `DEBUG`, `INFO`, `WARN`, `ERROR` | `INFO` |

### Project Configuration

Create a `projects.json` file to map project names to their working directories:

```json
[
  {
    "name": "my-project",
    "working_dir": "/path/to/my-project"
  },
  {
    "name": "another-project",
    "working_dir": "/path/to/another-project"
  }
]
```

See `projects.json.example` for a sample configuration.

## Building

### Using Make

The project includes a Makefile for common tasks:

```bash
make build        # Build the application
make test         # Run tests
make lint         # Format code and run checks
make docker-build # Build Docker image
make help         # Show all available targets
```

### Manual Build

Local build:
```bash
go build -o slackcompose
```

Docker build:
```bash
docker build -t slackcompose .
```

## Running

### Locally

Using Make:
```bash
make run
```

Or manually:
```bash
export SLACK_TOKEN=xoxb-your-slack-token
export REDIS_ADDR=localhost:6379
export REDIS_PASSWORD=your-redis-password
./slackcompose
```

### Docker Compose

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` with your configuration

3. Start the service:
   ```bash
   make docker-run
   # OR
   docker-compose up -d
   ```

## Usage

### Via Slack Command

In Slack, use the `/slack-compose` command:

```
/slack-compose my-project
```

This will execute `docker compose ps` for the specified project and post the output to the configured Slack channel.

### Via Emoji Reactions

Once the status is posted to Slack, you can control the project by reacting to the message:

- React with ⬆️ to run `docker compose up -d`
- React with ⬇️ to run `docker compose down`  
- React with 🔄 to run `docker compose restart`
- React with 📄 to run `docker compose logs`

## Integration Details

### Poppit Integration

SlackCompose sends commands to Poppit by pushing JSON payloads to a Redis list (default: `poppit:notifications`):

```json
{
  "repo": "<project name>",
  "branch": "refs/heads/main",
  "type": "slack-compose",
  "dir": "<working directory>",
  "commands": [
    "docker compose ps"
  ],
  "metadata": {
    "project": "<project name>"
  }
}
```

Poppit executes the commands and publishes output to a Redis Pub/Sub channel (default: `poppit:command-output`):

```json
{
  "type": "slack-compose",
  "command": "docker compose ps",
  "output": "<command output>",
  "metadata": {
    "project": "<project name>"
  }
}
```

### SlackLiner Integration

SlackCompose sends messages to SlackLiner by pushing JSON payloads to a Redis list (default: `slack_messages`):

```json
{
  "channel": "#slack-compose",
  "text": "<output of docker compose ps command>",
  "metadata": {
    "event_type": "slack-compose",
    "event_payload": {
      "project": "<project name>",
      "command": "docker compose ps"
    }
  }
}
```

The metadata allows emoji reactions on messages to be linked back to the original project for executing follow-up commands.

## Implementation Notes

### Service Architecture

The service is organized into the following components:

- **main.go** - Application entry point with graceful shutdown handling
- **config.go** - Configuration management from environment variables and project config file
- **redis.go** - Redis client wrapper for pub/sub operations
- **service.go** - Main service logic with command and reaction handlers
- **slack.go** - Slack API client for retrieving messages with metadata
- **clients.go** - HTTP clients for Poppit and SlackLiner integration
- **types.go** - Data structures for all payloads and messages

### Key Design Decisions

1. **Redis Architecture**: 
   - Pub/Sub channels for event notifications (commands, reactions, output)
   - Redis lists (RPUSH) for queuing messages to Poppit and SlackLiner
   - Decoupled communication between all services
2. **Metadata Tracking**: Project information is passed through Poppit's metadata field, making the service stateless
3. **Slack Message Metadata**: Slack message metadata links reactions to original projects
4. **Scratch Image**: Final Docker image uses scratch for minimal size (~11MB binary)
5. **Graceful Shutdown**: Context-based cancellation for clean service shutdown

### Project Configuration

Projects are mapped to working directories via a JSON configuration file. This allows:
- Dynamic project management without code changes
- Security by limiting which directories can be accessed
- Clear mapping between Slack-friendly names and filesystem paths

## Requirements

- Go 1.24 or later
- Redis server (locally hosted, not included in docker-compose)
- Slack workspace with appropriate permissions
- SlackCommandRelay, Poppit, SlackLiner, and SlackRelay services

## License

MIT
