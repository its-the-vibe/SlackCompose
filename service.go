package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/slack-go/slack"
)

const (
	EmojiUpArrow                = "arrow_up"
	EmojiDownArrow              = "arrow_down"
	EmojiArrowsCounterClockwise = "arrows_counterclockwise"
	EmojiPageFacingUp           = "page_facing_up"

	// Docker compose action IDs
	ActionDockerUp      = "docker_up"
	ActionDockerDown    = "docker_down"
	ActionDockerRestart = "docker_restart"
	ActionDockerPS      = "docker_ps"
	ActionDockerLogs    = "docker_logs"

	// Block Kit element IDs
	BlockIDProjectBlock  = "project_block"
	ActionIDSlackCompose = "SlackCompose"

	// Git branch reference
	DefaultGitBranch = "refs/heads/main"

	// DefaultTTLSeconds is the default time-to-live for SlackLiner messages (24 hours)
	DefaultTTLSeconds = 86400
)

// emojiToCommand maps supported emoji reactions to their docker compose commands
var emojiToCommand = map[string]string{
	EmojiUpArrow:                "docker compose up -d",
	EmojiDownArrow:              "docker compose down",
	EmojiArrowsCounterClockwise: "docker compose restart",
	EmojiPageFacingUp:           "docker compose logs",
}

// actionIDToCommand maps block action IDs to their docker compose commands
var actionIDToCommand = map[string]string{
	ActionDockerUp:      "docker compose up -d",
	ActionDockerDown:    "docker compose down",
	ActionDockerRestart: "docker compose restart",
	ActionDockerPS:      "docker compose ps",
	ActionDockerLogs:    "docker compose logs",
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

// getCommandForEmoji returns the docker compose command for a given emoji reaction
func (s *Service) getCommandForEmoji(emoji string) (string, bool) {
	baseCmd, ok := emojiToCommand[emoji]
	if !ok {
		return "", false
	}
	return s.expandCommand(baseCmd), true
}

// getCommandForActionID returns the docker compose command for a given action ID
func (s *Service) getCommandForActionID(actionID string) (string, bool) {
	baseCmd, ok := actionIDToCommand[actionID]
	if !ok {
		return "", false
	}
	return s.expandCommand(baseCmd), true
}

// expandCommand expands docker compose commands with config values
func (s *Service) expandCommand(cmd string) string {
	if cmd == "docker compose logs" {
		return fmt.Sprintf("docker compose logs -n %d", s.config.DockerLogsLineLimit)
	}
	return cmd
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

	// Start listening for Slack block actions
	s.wg.Add(1)
	go s.listenForBlockActions(ctx)

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

	// Check if project is empty or invalid - display block kit dialog
	if projectName == "" {
		slog.Info("No project name provided, showing block kit dialog")
		s.sendBlockKitDialog(ctx, cmd.ChannelID)
		return
	}

	// Check if project exists in config
	project, exists := s.config.Projects[projectName]
	if !exists {
		slog.Warn("Unknown project requested, showing block kit dialog", "project", projectName)
		s.sendBlockKitDialog(ctx, cmd.ChannelID)
		return
	}

	// Send docker compose ps command to Poppit
	poppitPayload := PoppitPayload{
		Repo:     projectName,
		Branch:   DefaultGitBranch,
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
	threadTS := ""
	channel := ""
	if cmdOutput.Metadata != nil {
		if proj, ok := cmdOutput.Metadata["project"].(string); ok {
			projectName = proj
		}
		if ts, ok := cmdOutput.Metadata["thread_ts"].(string); ok {
			threadTS = ts
		}
		if ch, ok := cmdOutput.Metadata["channel"].(string); ok {
			channel = ch
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

	// Use the channel from metadata if available, otherwise use default
	targetChannel := s.config.SlackChannel
	if channel != "" {
		targetChannel = channel
	}

	slackLinerPayload := SlackLinerPayload{
		Channel: targetChannel,
		Text:    fmt.Sprintf("*Project:* %s\n*Command:* `%s`\n```\n%s\n```", projectName, cmdOutput.Command, cmdOutput.Output),
		Metadata: SlackMetadata{
			EventType:    "slack-compose",
			EventPayload: eventPayload,
		},
		TTL:      DefaultTTLSeconds,
		ThreadTS: threadTS,
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
	command, supported := s.getCommandForEmoji(reaction.Event.Reaction)
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
	// Include thread_ts and channel metadata to enable posting command output as thread replies in the correct channel
	poppitPayload := PoppitPayload{
		Repo:     projectName,
		Branch:   DefaultGitBranch,
		Type:     "slack-compose",
		Dir:      project.WorkingDir,
		Commands: []string{command},
		Metadata: map[string]interface{}{
			"project":   projectName,
			"thread_ts": reaction.Event.Item.TS,
			"channel":   reaction.Event.Item.Channel,
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

// sendBlockKitDialog sends a block kit dialog to the user
func (s *Service) sendBlockKitDialog(ctx context.Context, channel string) {
	// Create block kit blocks using slack-go/slack types
	minQueryLength := 0
	externalSelect := slack.NewOptionsSelectBlockElement(
		slack.OptTypeExternal,
		slack.NewTextBlockObject(slack.PlainTextType, "Search projects...", false, false),
		ActionIDSlackCompose,
	)
	externalSelect.MinQueryLength = &minQueryLength

	blocks := []slack.Block{
		// Section with instructions
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, "Select a project from your GitHub repositories to manage its containers.", false, false),
			nil,
			nil,
		),
		// Input block with external select for project selection
		slack.NewInputBlock(
			BlockIDProjectBlock,
			slack.NewTextBlockObject(slack.PlainTextType, "Project / Repository", false, false),
			nil,
			externalSelect,
		),
		// Divider
		slack.NewDividerBlock(),
		// Lifecycle actions section header
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, "*Lifecycle Actions*", false, false),
			nil,
			nil,
		),
		// Lifecycle action buttons
		slack.NewActionBlock(
			"",
			slack.NewButtonBlockElement(
				ActionDockerUp,
				"up",
				slack.NewTextBlockObject(slack.PlainTextType, ":arrow_up: Up", false, false),
			).WithStyle(slack.StylePrimary),
			slack.NewButtonBlockElement(
				ActionDockerRestart,
				"restart",
				slack.NewTextBlockObject(slack.PlainTextType, ":arrows_counterclockwise: Restart", false, false),
			),
			slack.NewButtonBlockElement(
				ActionDockerDown,
				"down",
				slack.NewTextBlockObject(slack.PlainTextType, ":arrow_down: Down", false, false),
			).WithStyle(slack.StyleDanger),
		),
		// Observation section header
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, "*Observation*", false, false),
			nil,
			nil,
		),
		// Observation action buttons
		slack.NewActionBlock(
			"",
			slack.NewButtonBlockElement(
				ActionDockerPS,
				"ps",
				slack.NewTextBlockObject(slack.PlainTextType, ":chart_with_upwards_trend: Process Status", false, false),
			),
			slack.NewButtonBlockElement(
				ActionDockerLogs,
				"logs",
				slack.NewTextBlockObject(slack.PlainTextType, ":page_facing_up: View Logs", false, false),
			),
		),
	}

	slackLinerPayload := SlackLinerPayload{
		Channel: channel,
		Blocks:  blocks,
		TTL:     DefaultTTLSeconds,
		Metadata: SlackMetadata{
			EventType:    "slack-compose-dialog",
			EventPayload: map[string]interface{}{},
		},
	}

	if err := s.sendToSlackLiner(ctx, slackLinerPayload); err != nil {
		slog.Error("Failed to send block kit dialog to SlackLiner", "error", err)
		return
	}

	slog.Info("Sent block kit dialog", "channel", channel)
}

// listenForBlockActions listens for Slack block actions from SlackRelay
func (s *Service) listenForBlockActions(ctx context.Context) {
	defer s.wg.Done()

	pubsub := s.redisClient.Subscribe(ctx, s.config.SlackBlockActionsChannel)
	defer pubsub.Close()

	slog.Info("Listening for block actions", "channel", s.config.SlackBlockActionsChannel)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}
			s.handleBlockAction(ctx, msg.Payload)
		}
	}
}

// handleBlockAction processes block action events
func (s *Service) handleBlockAction(ctx context.Context, payload string) {
	var action SlackBlockAction
	if err := json.Unmarshal([]byte(payload), &action); err != nil {
		slog.Error("Failed to parse block action", "error", err)
		return
	}

	slog.Debug("Received block action", "actions", len(action.Actions))

	// Extract the selected project from state
	projectName := ""
	if state, ok := action.State.Values[BlockIDProjectBlock]; ok {
		if slackCompose, ok := state[ActionIDSlackCompose]; ok {
			if slackCompose.SelectedOption != nil {
				projectName = slackCompose.SelectedOption.Value
				slog.Debug("Extracted project from state", "project", projectName)
			}
		}
	}

	// If no project selected, ignore the action
	if projectName == "" {
		slog.Debug("No project selected, ignoring block action")
		return
	}

	// Check if project exists
	project, exists := s.config.Projects[projectName]
	if !exists {
		slog.Warn("Unknown project in block action", "project", projectName)
		return
	}

	// Process each action
	for _, act := range action.Actions {
		// Only process button actions
		if act.Type != "button" {
			slog.Debug("Ignoring non-button action", "type", act.Type)
			continue
		}

		// Check if this is a known action
		command, known := s.getCommandForActionID(act.ActionID)
		if !known {
			slog.Debug("Unknown action_id, ignoring", "action_id", act.ActionID)
			continue
		}

		slog.Info("Executing command for project", "command", command, "project", projectName, "action_id", act.ActionID)

		// Determine channel and thread_ts
		channel := ""
		threadTS := ""
		if action.Channel.ID != "" {
			channel = action.Channel.ID
		}
		if action.Message.TS != "" {
			threadTS = action.Message.TS
		}

		// Send command to Poppit
		poppitPayload := PoppitPayload{
			Repo:     projectName,
			Branch:   DefaultGitBranch,
			Type:     "slack-compose",
			Dir:      project.WorkingDir,
			Commands: []string{command},
			Metadata: map[string]interface{}{
				"project":   projectName,
				"thread_ts": threadTS,
				"channel":   channel,
			},
		}

		if err := s.sendToPoppit(ctx, poppitPayload); err != nil {
			slog.Error("Failed to send to Poppit", "error", err, "project", projectName)
			continue
		}

		slog.Info("Sent command to Poppit", "command", command, "project", projectName)
	}
}
