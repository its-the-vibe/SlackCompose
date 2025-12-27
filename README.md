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

### With Project Name
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

### Without Project Name (Block Kit Dialog)
```
User in Slack → /slack-compose (no project)
       ↓
SlackCommandRelay → Redis Pub/Sub (slack-commands)
       ↓
SlackCompose → Redis List RPUSH (slack_messages)
       ↓
SlackLiner posts Block Kit dialog to Slack
       ↓
User selects project & clicks action button
       ↓
SlackRelay → Redis Pub/Sub (slack-relay-block-actions)
       ↓
SlackCompose → Redis List RPUSH (poppit:notifications)
       ↓
Poppit executes: docker compose command
       ↓
Poppit → Redis Pub/Sub (poppit:command-output)
       ↓
SlackCompose → Redis List RPUSH (slack_messages)
       ↓
SlackLiner posts output as thread reply
```

## Features

- Execute `docker compose ps` via Slack command `/slack-compose <project-name>`
- Interactive Block Kit dialog when no project specified with `/slack-compose`
  - Project selection via external select dropdown
  - Action buttons for docker compose commands (up, down, restart, ps, logs)
  - Commands execute as thread replies to the dialog message
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
| `SLACK_BLOCK_ACTIONS_CHANNEL` | Redis Pub/Sub channel for Slack block actions | `slack-relay-block-actions` |
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

**With a project name:**
```
/slack-compose my-project
```

This will execute `docker compose ps` for the specified project and post the output to the configured Slack channel.

**Without a project name (Block Kit Dialog):**
```
/slack-compose
```

This displays an interactive Block Kit dialog where you can:
1. Select a project from the external select dropdown
2. Click action buttons to execute docker compose commands:
   - **⬆️ Up** - runs `docker compose up -d`
   - **🔄 Restart** - runs `docker compose restart`
   - **⬇️ Down** - runs `docker compose down`
   - **📊 Process Status** - runs `docker compose ps`
   - **📄 View Logs** - runs `docker compose logs`

Command outputs are posted as thread replies to the dialog message.

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

**Text messages:**
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
  },
  "ttl": 86400
}
```

**Block Kit messages:**
```json
{
  "channel": "#slack-compose",
  "blocks": [
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "Select a project..."
      }
    },
    {
      "type": "input",
      "block_id": "project_block",
      "element": {
        "type": "external_select",
        "action_id": "SlackCompose",
        "placeholder": {
          "type": "plain_text",
          "text": "Search projects..."
        }
      }
    }
  ],
  "ttl": 86400
}
```

The metadata allows emoji reactions on messages to be linked back to the original project for executing follow-up commands. Messages have a default TTL of 24 hours (86400 seconds).

### SlackRelay Integration

SlackRelay publishes block action events to a Redis Pub/Sub channel (default: `slack-relay-block-actions`):

```json
{
  "type": "block_actions",
  "actions": [
    {
      "action_id": "docker_logs",
      "block_id": "m8VaW",
      "type": "button",
      "value": "logs"
    }
  ],
  "state": {
    "values": {
      "project_block": {
        "SlackCompose": {
          "type": "external_select",
          "selected_option": {
            "text": {
              "type": "plain_text",
              "text": "InnerGate"
            },
            "value": "InnerGate"
          }
        }
      }
    }
  },
  "message": {
    "ts": "1234567890.123456"
  },
  "channel": {
    "id": "C1234567890",
    "name": "slack-compose"
  }
}
```

SlackCompose listens for these events and:
1. Extracts the selected project from the `state.values.project_block.SlackCompose.selected_option.value` field
2. Identifies the action from the `actions[].action_id` field (must be type "button")
3. Executes the corresponding docker compose command via Poppit
4. Posts output as a thread reply to the original message (using `message.ts` as `thread_ts`)

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
