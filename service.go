package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const (
	EmojiUpArrow                = "arrow_up"
	EmojiDownArrow              = "arrow_down"
	EmojiArrowsCounterClockwise = "arrows_counterclockwise"
	EmojiPageWithCurl           = "page_with_curl"
)

// Service is the main service handler
type Service struct {
	config      *Config
	redisClient *RedisClient
	slackClient *SlackClient
	taskMap     map[string]string // maps taskID to projectName
	taskMapMu   sync.RWMutex
	wg          sync.WaitGroup
}

// NewService creates a new service instance
func NewService(config *Config, redisClient *RedisClient) *Service {
	return &Service{
		config:      config,
		redisClient: redisClient,
		slackClient: NewSlackClient(config.SlackToken),
		taskMap:     make(map[string]string),
	}
}

// Start starts the service
func (s *Service) Start(ctx context.Context) error {
	log.Println("Service starting...")

	// Start listening for Slack commands
	s.wg.Add(1)
	go s.listenForCommands(ctx)

	// Start listening for Slack reactions
	s.wg.Add(1)
	go s.listenForReactions(ctx)

	// Start listening for Poppit command output
	s.wg.Add(1)
	go s.listenForPoppitOutput(ctx)

	log.Println("Service started successfully")
	return nil
}

// listenForCommands listens for Slack commands from SlackCommandRelay
func (s *Service) listenForCommands(ctx context.Context) {
	defer s.wg.Done()

	pubsub := s.redisClient.Subscribe(ctx, s.config.SlackCommandChannel)
	defer pubsub.Close()

	log.Printf("Listening for commands on channel: %s", s.config.SlackCommandChannel)

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
		log.Printf("Failed to parse command: %v", err)
		return
	}

	// Only handle /slack-compose commands
	if cmd.Command != "/slack-compose" {
		return
	}

	log.Printf("Received /slack-compose command with text: %s", cmd.Text)

	// Extract project name from command text
	projectName := strings.TrimSpace(cmd.Text)
	if projectName == "" {
		log.Println("No project name provided")
		return
	}

	// Check if project exists in config
	project, exists := s.config.Projects[projectName]
	if !exists {
		log.Printf("Unknown project: %s", projectName)
		return
	}

	// Generate task ID
	taskID := fmt.Sprintf("task-%s", uuid.New().String())

	// Store task-to-project mapping
	s.taskMapMu.Lock()
	s.taskMap[taskID] = projectName
	s.taskMapMu.Unlock()

	// Send docker compose ps command to Poppit
	poppitPayload := PoppitPayload{
		Repo:     projectName,
		Branch:   "refs/heads/main",
		Type:     "slack-compose",
		Dir:      project.WorkingDir,
		Commands: []string{"docker compose ps"},
		TaskID:   taskID,
	}

	if err := s.sendToPoppit(ctx, poppitPayload); err != nil {
		log.Printf("Failed to send to Poppit: %v", err)
		return
	}

	log.Printf("Sent docker compose ps command for project %s (task: %s)", projectName, taskID)
}

// listenForPoppitOutput listens for command output from Poppit
func (s *Service) listenForPoppitOutput(ctx context.Context) {
	defer s.wg.Done()

	pubsub := s.redisClient.Subscribe(ctx, s.config.PoppitOutputChannel)
	defer pubsub.Close()

	log.Printf("Listening for Poppit output on channel: %s", s.config.PoppitOutputChannel)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				log.Printf("Received nil message from Poppit output channel, possible connection issue")
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
		log.Printf("Failed to parse Poppit output: %v", err)
		return
	}

	log.Printf("Received output for task %s: command=%s", cmdOutput.TaskID, cmdOutput.Command)

	// Only handle output for slack-compose type
	if cmdOutput.Type != "slack-compose" {
		return
	}

	// Retrieve project name from task map
	s.taskMapMu.RLock()
	projectName, exists := s.taskMap[cmdOutput.TaskID]
	s.taskMapMu.RUnlock()

	// Clean up task map regardless of success or failure
	defer func() {
		s.taskMapMu.Lock()
		delete(s.taskMap, cmdOutput.TaskID)
		s.taskMapMu.Unlock()
	}()

	if !exists {
		log.Printf("Warning: Task %s not found in task map, project name will not be included in metadata", cmdOutput.TaskID)
	}

	// Build metadata with project name if available
	eventPayload := map[string]interface{}{
		"taskId":  cmdOutput.TaskID,
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
	}

	if err := s.sendToSlackLiner(ctx, slackLinerPayload); err != nil {
		log.Printf("Failed to send to SlackLiner: %v", err)
		return
	}

	log.Printf("Sent output to SlackLiner for task %s", cmdOutput.TaskID)
}

// listenForReactions listens for emoji reactions from SlackRelay
func (s *Service) listenForReactions(ctx context.Context) {
	defer s.wg.Done()

	pubsub := s.redisClient.Subscribe(ctx, s.config.SlackReactionChannel)
	defer pubsub.Close()

	log.Printf("Listening for reactions on channel: %s", s.config.SlackReactionChannel)

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
		log.Printf("Failed to parse reaction: %v", err)
		return
	}

	log.Printf("Received reaction: %s on message %s in channel %s", reaction.Event.Reaction, reaction.Event.Item.TS, reaction.Event.Item.Channel)

	// Only handle specific reactions
	if reaction.Event.Reaction != EmojiUpArrow && reaction.Event.Reaction != EmojiDownArrow && reaction.Event.Reaction != EmojiArrowsCounterClockwise && reaction.Event.Reaction != EmojiPageWithCurl {
		return
	}

	// Retrieve message from Slack to get metadata
	message, err := s.slackClient.GetMessage(ctx, reaction.Event.Item.Channel, reaction.Event.Item.TS)
	if err != nil {
		log.Printf("Failed to retrieve message: %v", err)
		return
	}

	// Parse metadata
	if message.Metadata.EventType != "slack-compose" {
		log.Printf("Message is not a slack-compose event, ignoring")
		return
	}

	projectName, ok := message.Metadata.EventPayload["project"].(string)
	if !ok || projectName == "" {
		log.Printf("No project name in metadata")
		return
	}

	// Check if project exists
	project, exists := s.config.Projects[projectName]
	if !exists {
		log.Printf("Unknown project in metadata: %s", projectName)
		return
	}

	// Determine command based on reaction
	var command string
	switch reaction.Event.Reaction {
	case EmojiUpArrow:
		command = "docker compose up -d"
	case EmojiDownArrow:
		command = "docker compose down"
	case EmojiArrowsCounterClockwise:
		command = "docker compose restart"
	case EmojiPageWithCurl:
		command = "docker compose logs"
	}

	log.Printf("Executing %s for project %s", command, projectName)

	// Generate new task ID
	taskID := fmt.Sprintf("task-%s", uuid.New().String())

	// Send command to Poppit
	poppitPayload := PoppitPayload{
		Repo:     projectName,
		Branch:   "refs/heads/main",
		Type:     "slack-compose",
		Dir:      project.WorkingDir,
		Commands: []string{command},
		TaskID:   taskID,
	}

	if err := s.sendToPoppit(ctx, poppitPayload); err != nil {
		log.Printf("Failed to send to Poppit: %v", err)
		return
	}

	log.Printf("Sent %s command for project %s (task: %s)", command, projectName, taskID)
}

// Wait waits for all goroutines to finish
func (s *Service) Wait() {
	s.wg.Wait()
}
