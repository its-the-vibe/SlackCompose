# Copilot Instructions for SlackCompose

## Project Overview

SlackCompose is a Go service that enables running Docker Compose commands from Slack. It integrates with multiple services via Redis pub/sub:
- **SlackCommandRelay** - receives `/slack-compose` commands from Slack
- **Poppit** - executes docker compose commands
- **SlackLiner** - posts command output to Slack channels
- **SlackRelay** - receives emoji reaction events from Slack
- **Redis** - for pub/sub messaging and list-based queues between services

## Code Style and Conventions

### Go Style
- Use Go 1.25.5 or later
- Follow standard Go formatting with `gofmt`
- Use structured logging with `log/slog` package
- Always use `log/slog.Info`, `log/slog.Debug`, `log/slog.Error`, etc. for logging
- Use constants for magic strings and values
- Keep functions focused and single-purpose

### Naming Conventions
- Use PascalCase for exported types, functions, and constants
- Use camelCase for unexported functions and variables
- Use ALL_CAPS for environment variable names in documentation
- Prefix Redis channel/list names with service name (e.g., `poppit:notifications`)
- Use descriptive names for emoji constants (e.g., `EmojiUpArrow` not `UP`)

### Error Handling
- Always wrap errors with context using `fmt.Errorf("message: %w", err)`
- Log errors before returning them
- Use structured logging with key-value pairs for error context
- Never ignore errors without good reason

### JSON Structures
- Always use struct tags for JSON marshaling/unmarshaling
- Use `omitempty` for optional fields
- Use `interface{}` for dynamic/unknown JSON structures
- Map action IDs and emojis to commands using maps defined as constants

## Project Structure

### Key Files
- **main.go** - Application entry point with graceful shutdown handling
- **config.go** - Configuration management from environment variables and project config file
- **redis.go** - Redis client wrapper for pub/sub operations
- **service.go** - Main service logic with command and reaction handlers
- **slack.go** - Slack API client for retrieving messages with metadata
- **clients.go** - HTTP clients for Poppit and SlackLiner integration
- **types.go** - Data structures for all payloads and messages

### Configuration
- All configuration comes from environment variables
- Use `getEnv()` helper for string values with defaults
- Use `getEnvInt()` helper for integer values with defaults
- Project mappings are loaded from `projects.json` file
- Required fields (like `SLACK_BOT_TOKEN`) must be validated in `LoadConfig()`

## Redis Integration Patterns

### Pub/Sub Channels
- Subscribe to channels using `redisClient.Subscribe(ctx, channel)`
- Publish messages using `redisClient.Publish(ctx, channel, message)`
- Always unmarshal JSON payloads into strongly-typed structs
- Use separate goroutines for each channel subscription

### Redis Lists (Queues)
- Push to lists using `redisClient.RPush(ctx, listName, payload)`
- Always marshal payloads to JSON before pushing
- Lists are used for one-way communication (fire-and-forget)

### Metadata Pattern
- Include metadata in all payloads to maintain statelessness
- Project information is passed through Poppit's metadata field
- Slack message metadata links reactions to original projects

## Slack Integration

### Message Metadata
- Fetch messages using `slackClient.GetMessage(channel, timestamp)`
- Extract project from metadata: `message.Metadata.EventPayload["project"]`
- Use metadata to track which project a message relates to

### Block Kit
- Use Slack Block Kit for interactive messages
- External select dropdowns for project selection
- Action buttons for docker compose commands
- Post outputs as thread replies using `thread_ts`

### Action IDs
- Use descriptive action IDs: `docker_up`, `docker_down`, `docker_restart`, `docker_ps`, `docker_logs`
- Map action IDs to commands using `actionIDToCommand` map
- Block ID for project selection is `project_block`
- Action ID for project select is `SlackCompose`

## Docker Compose Commands

### Command Patterns
- Commands are sent to Poppit via Redis lists
- All commands use `docker compose` (not `docker-compose`)
- Commands include: `up -d`, `down`, `restart`, `ps`, `logs -n <limit>`
- Use `expandCommand()` to add config values (e.g., log line limits)

### Emoji Mappings
- ⬆️ (`arrow_up`) → `docker compose up -d`
- ⬇️ (`arrow_down`) → `docker compose down`
- 🔄 (`arrows_counterclockwise`) → `docker compose restart`
- 📄 (`page_facing_up`) → `docker compose logs -n <limit>`

## Build and Test Commands

### Building
```bash
make build        # Build the application binary
make docker-build # Build Docker image
go build -o slackcompose .  # Manual build
```

### Testing
```bash
make test  # Run all tests
go test -v ./...  # Manual test execution
```

### Linting and Formatting
```bash
make lint  # Format code, run go vet, and tidy modules
make fmt   # Format code only
make vet   # Run go vet only
```

### Running
```bash
make run         # Run locally
make docker-run  # Run with docker-compose
```

## Docker

### Dockerfile
- Uses multi-stage build with `golang:1.25.5-alpine` as builder
- Final image uses `scratch` for minimal size (~11MB)
- CGO is disabled for static binary
- Includes CA certificates for HTTPS connections

### Environment Variables
- Copy `.env.example` to `.env` for docker-compose
- All configuration is via environment variables
- See README.md for complete list of variables

## Testing Guidelines

- No existing test files in the project currently
- When adding tests, follow Go test conventions
- Use table-driven tests for multiple scenarios
- Mock Redis and Slack clients for unit tests
- Test JSON marshaling/unmarshaling for all payload types

## Dependencies

- `github.com/redis/go-redis/v9` - Redis client
- `github.com/slack-go/slack` - Slack API client
- Use `go mod download` to fetch dependencies
- Run `go mod tidy` to clean up dependencies

## Common Patterns

### Graceful Shutdown
- Use context cancellation for clean shutdown
- Wait for all goroutines to finish using `sync.WaitGroup`
- Listen for SIGINT and SIGTERM signals

### Concurrent Operations
- Each Redis subscription runs in its own goroutine
- Use `wg.Add(1)` before starting goroutine
- Always defer `wg.Done()` at start of goroutine
- Pass context for cancellation support

### JSON Payload Handling
1. Define struct with JSON tags in `types.go`
2. Marshal to JSON before sending to Redis
3. Unmarshal from JSON when receiving from Redis
4. Handle errors for all marshal/unmarshal operations

## Security Considerations

- Never commit `.env` file (it's in `.gitignore`)
- Validate all user inputs from Slack commands
- Check project names against configured projects
- Don't expose internal errors to Slack users
- Use structured logging to avoid leaking sensitive data

## Common Tasks

### Adding a New Docker Compose Command
1. Add action ID constant to `service.go`
2. Add entry to `actionIDToCommand` map
3. Update Block Kit buttons if needed
4. Test with Slack integration

### Adding a New Emoji Reaction
1. Add emoji constant to `service.go`
2. Add entry to `emojiToCommand` map
3. Document in README.md
4. Test with Slack reactions

### Modifying Configuration
1. Update `Config` struct in `config.go`
2. Add environment variable with `getEnv()` or `getEnvInt()`
3. Update `.env.example`
4. Update README.md documentation

### Adding a New Redis Channel
1. Add channel name to `Config` struct
2. Subscribe in `service.go` `Start()` method
3. Create handler function following existing patterns
4. Add to goroutine with proper error handling

## Code Review Checklist

- [ ] All errors are properly handled and logged
- [ ] JSON structs have appropriate tags and `omitempty`
- [ ] New environment variables are documented in README
- [ ] Code follows Go formatting standards
- [ ] Constants are used instead of magic strings
- [ ] Logging uses structured `log/slog` package
- [ ] Context is passed and used for cancellation
- [ ] WaitGroup is used for goroutine synchronization
- [ ] Changes are minimal and focused
- [ ] No sensitive data is logged or exposed
