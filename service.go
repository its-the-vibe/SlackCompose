package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

const (
	EmojiUpArrow                = "arrow_up"
	EmojiDownArrow              = "arrow_down"
	EmojiArrowsCounterClockwise = "arrows_counterclockwise"
	EmojiPageFacingUp           = "page_facing_up"
)

// emojiToCommand maps supported emoji reactions to their docker compose commands
var emojiToCommand = map[string]string{
	EmojiUpArrow:                "docker compose up -d",
	EmojiDownArrow:              "docker compose down",
	EmojiArrowsCounterClockwise: "docker compose restart",
	EmojiPageFacingUp:           "docker compose logs",
}

// Service is the main service handler
type Service struct {
	config      *Config
	redisClient *RedisClient
	slackClient *SlackClient
	wg          sync.WaitGroup
}

// NewService creates a new service instance
func NewService(config *Config, redisClient *RedisClient) *Service {
	return &Service{
		config:      config,
		redisClient: redisClient,
		slackClient: NewSlackClient(config.SlackToken),
	}
}

// Start starts the service
func (s *Service) Start(ctx context.Context) error {
	slog.Info("Service starting...")

	// Start listening for Slack commands
	s.wg.Add(1)
	go s.listenForCommands(ctx)

	// Start listening for Slack reactions
	s.wg.Add(1)
	go s.listenForReactions(ctx)

	// Start listening for Poppit command output
	s.wg.Add(1)
	go s.listenForPoppitOutput(ctx)

	slog.Info("Service started successfully")
	return nil
}

// listenForCommands listens for Slack commands from SlackCommandRelay
func (s *Service) listenForCommands(ctx context.Context) {
	defer s.wg.Done()

	pubsub := s.redisClient.Subscribe(ctx, s.config.SlackCommandChannel)
	defer pubsub.Close()

	slog.Info("Listening for commands", "channel", s.config.SlackCommandChannel)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}
			s.handleCommand(ctx, msg.Payload)
		}
	}
}

// handleCommand processes incoming Slack commands
func (s *Service) handleCommand(ctx context.Context, payload string) {
	var cmd SlackCommand
	if err := json.Unmarshal([]byte(payload), &cmd); err != nil {
		slog.Error("Failed to parse command", "error", err)
		return
	}

	// Only handle /slack-compose commands
	if cmd.Command != "/slack-compose" {
		return
	}

	slog.Info("Received /slack-compose command", "text", cmd.Text)

	// Extract project name from command text
	projectName := strings.TrimSpace(cmd.Text)
	if projectName == "" {
		slog.Warn("No project name provided in command")
		return
	}

	// Check if project exists in config
	project, exists := s.config.Projects[projectName]
	if !exists {
		slog.Warn("Unknown project requested", "project", projectName)
		return
	}

	// Send docker compose ps command to Poppit
	poppitPayload := PoppitPayload{
		Repo:     projectName,
		Branch:   "refs/heads/main",
		Type:     "slack-compose",
		Dir:      project.WorkingDir,
		Commands: []string{"docker compose ps"},
		Metadata: map[string]interface{}{
			"project": projectName,
		},
	}

	if err := s.sendToPoppit(ctx, poppitPayload); err != nil {
		slog.Error("Failed to send to Poppit", "error", err, "project", projectName)
		return
	}

	slog.Info("Sent docker compose ps command", "project", projectName)
}

// listenForPoppitOutput listens for command output from Poppit
func (s *Service) listenForPoppitOutput(ctx context.Context) {
	defer s.wg.Done()

	pubsub := s.redisClient.Subscribe(ctx, s.config.PoppitOutputChannel)
	defer pubsub.Close()

	slog.Info("Listening for Poppit output", "channel", s.config.PoppitOutputChannel)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				slog.Warn("Received nil message from Poppit output channel, possible connection issue")
				continue
			}
			s.handlePoppitOutput(ctx, msg.Payload)
		}
	}
}

// handlePoppitOutput handles output from Poppit and sends it to SlackLiner
func (s *Service) handlePoppitOutput(ctx context.Context, payload string) {
	var cmdOutput PoppitCommandOutput
	if err := json.Unmarshal([]byte(payload), &cmdOutput); err != nil {
		slog.Error("Failed to parse Poppit output", "error", err)
		return
	}

	slog.Debug("Received output", "command", cmdOutput.Command)

	// Only handle output for slack-compose type
	if cmdOutput.Type != "slack-compose" {
		return
	}

	// Extract project name from metadata
	projectName := ""
	if cmdOutput.Metadata != nil {
		if proj, ok := cmdOutput.Metadata["project"].(string); ok {
			projectName = proj
		}
	}

	if projectName == "" {
		slog.Warn("No project name in metadata")
	}

	// Build metadata for SlackLiner
	eventPayload := map[string]interface{}{
		"command": cmdOutput.Command,
	}
	if projectName != "" {
		eventPayload["project"] = projectName
	}

	slackLinerPayload := SlackLinerPayload{
		Channel: s.config.SlackChannel,
		Text:    fmt.Sprintf("```\n%s\n```", cmdOutput.Output),
		Metadata: SlackMetadata{
			EventType:    "slack-compose",
			EventPayload: eventPayload,
		},
		TTL: 86400, // 24 hours in seconds
	}

	if err := s.sendToSlackLiner(ctx, slackLinerPayload); err != nil {
		slog.Error("Failed to send to SlackLiner", "error", err)
		return
	}

	slog.Info("Sent output to SlackLiner", "project", projectName)
}

// listenForReactions listens for emoji reactions from SlackRelay
func (s *Service) listenForReactions(ctx context.Context) {
	defer s.wg.Done()

	pubsub := s.redisClient.Subscribe(ctx, s.config.SlackReactionChannel)
	defer pubsub.Close()

	slog.Info("Listening for reactions", "channel", s.config.SlackReactionChannel)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}
			s.handleReaction(ctx, msg.Payload)
		}
	}
}

// handleReaction processes emoji reactions
func (s *Service) handleReaction(ctx context.Context, payload string) {
	var reaction SlackReaction
	if err := json.Unmarshal([]byte(payload), &reaction); err != nil {
		slog.Error("Failed to parse reaction", "error", err)
		return
	}

	slog.Debug("Received reaction", "emoji", reaction.Event.Reaction, "message", reaction.Event.Item.TS, "channel", reaction.Event.Item.Channel)

	// Check if this is a supported reaction
	// Unsupported reactions are logged at DEBUG level to avoid cluttering logs with reactions we don't care about
	command, supported := emojiToCommand[reaction.Event.Reaction]
	if !supported {
		slog.Debug("Unsupported reaction, ignoring", "emoji", reaction.Event.Reaction)
		return
	}

	// Retrieve message from Slack to get metadata
	message, err := s.slackClient.GetMessage(ctx, reaction.Event.Item.Channel, reaction.Event.Item.TS)
	if err != nil {
		slog.Error("Failed to retrieve message", "error", err)
		return
	}

	// Parse metadata
	if message.Metadata.EventType != "slack-compose" {
		slog.Debug("Message is not a slack-compose event, ignoring")
		return
	}

	projectName, ok := message.Metadata.EventPayload["project"].(string)
	if !ok || projectName == "" {
		slog.Warn("No project name in metadata")
		return
	}

	// Check if project exists
	project, exists := s.config.Projects[projectName]
	if !exists {
		slog.Warn("Unknown project in metadata", "project", projectName)
		return
	}

	slog.Info("Executing command for project", "command", command, "project", projectName)

	// Send command to Poppit
	poppitPayload := PoppitPayload{
		Repo:     projectName,
		Branch:   "refs/heads/main",
		Type:     "slack-compose",
		Dir:      project.WorkingDir,
		Commands: []string{command},
		Metadata: map[string]interface{}{
			"project": projectName,
		},
	}

	if err := s.sendToPoppit(ctx, poppitPayload); err != nil {
		slog.Error("Failed to send to Poppit", "error", err, "project", projectName)
		return
	}

	slog.Info("Sent command to Poppit", "command", command, "project", projectName)
}

// Wait waits for all goroutines to finish
func (s *Service) Wait() {
	s.wg.Wait()
}
