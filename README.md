# SlackCompose

Run Docker Compose commands from Slack

## Overview

SlackCompose is a Go service that enables running Docker Compose commands from Slack. It integrates with:
- **SlackCommandRelay** - receives `/docker-compose` commands from Slack
- **Poppit** - executes docker compose commands
- **SlackLiner** - posts command output to Slack channels
- **SlackRelay** - receives emoji reaction events from Slack
- **Redis** - for pub/sub messaging between services

## Features

- Execute `docker compose ps` via Slack command `/docker-compose <project-name>`
- Control projects via emoji reactions:
  - ⬆️ (up_arrow) - runs `docker compose up -d`
  - ⬇️ (down_arrow) - runs `docker compose down`
  - 🔄 (arrows_counterclockwise) - runs `docker compose restart`
- Project configuration via JSON file
- Built with scratch Docker image for minimal size

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REDIS_ADDR` | Redis server address | `localhost:6379` |
| `REDIS_PASSWORD` | Redis password | (empty) |
| `REDIS_DB` | Redis database number | `0` |
| `SLACK_COMMAND_CHANNEL` | Redis channel for Slack commands | `slack-commands` |
| `SLACK_REACTION_CHANNEL` | Redis channel for Slack reactions | `slack-reactions` |
| `POPPIT_URL` | URL of Poppit service | `http://localhost:8080` |
| `SLACKLINER_URL` | URL of SlackLiner service | `http://localhost:8081` |
| `SLACK_TOKEN` | Slack API token (required) | - |
| `SLACK_CHANNEL` | Slack channel to post to | `#slack-compose` |
| `PROJECT_CONFIG_PATH` | Path to projects configuration file | `projects.json` |

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

### Local Build

```bash
go build -o slackcompose
```

### Docker Build

```bash
docker build -t slackcompose .
```

## Running

### Locally

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
   docker-compose up -d
   ```

## Usage

### Via Slack Command

In Slack, use the `/docker-compose` command:

```
/docker-compose my-project
```

This will execute `docker compose ps` for the specified project and post the output to the configured Slack channel.

### Via Emoji Reactions

Once the status is posted to Slack, you can control the project by reacting to the message:

- React with ⬆️ to run `docker compose up -d`
- React with ⬇️ to run `docker compose down`  
- React with 🔄 to run `docker compose restart`

## Integration Details

### Poppit Payload

When sending commands to Poppit, the following payload is used:

```json
{
  "repo": "<project name>",
  "branch": "refs/heads/main",
  "type": "slack-compose",
  "dir": "<working directory>",
  "commands": [
    "docker compose ps"
  ],
  "taskId": "task-12345"
}
```

### SlackLiner Payload

When posting to Slack via SlackLiner:

```json
{
  "channel": "#slack-compose",
  "text": "<output of docker compose ps command>",
  "metadata": {
    "event_type": "slack-compose",
    "event_payload": {
      "taskId": "task-12345",
      "project": "<project name>"
    }
  }
}
```

## Requirements

- Go 1.24 or later
- Redis server (locally hosted, not included in docker-compose)
- Slack workspace with appropriate permissions
- SlackCommandRelay, Poppit, SlackLiner, and SlackRelay services

## License

MIT
