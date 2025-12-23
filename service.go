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
	log.Println("Service starting...")

	// Start listening for Slack commands
	s.wg.Add(1)
	go s.listenForCommands(ctx)

	// Start listening for Slack reactions
	s.wg.Add(1)
	go s.listenForReactions(ctx)

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

	// Only handle /docker-compose commands
	if cmd.Command != "/docker-compose" {
		return
	}

	log.Printf("Received /docker-compose command with text: %s", cmd.Text)

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

	// Listen for Poppit output and send to SlackLiner
	// In a real implementation, this would be done via a separate Redis channel
	// For now, we'll simulate receiving the output
	s.handlePoppitOutput(ctx, taskID, projectName, "docker compose ps output would appear here")
}

// handlePoppitOutput handles output from Poppit and sends it to SlackLiner
func (s *Service) handlePoppitOutput(ctx context.Context, taskID, projectName, output string) {
	slackLinerPayload := SlackLinerPayload{
		Channel: s.config.SlackChannel,
		Text:    output,
		Metadata: SlackMetadata{
			EventType: "slack-compose",
			EventPayload: map[string]interface{}{
				"taskId":  taskID,
				"project": projectName,
			},
		},
	}

	if err := s.sendToSlackLiner(ctx, slackLinerPayload); err != nil {
		log.Printf("Failed to send to SlackLiner: %v", err)
		return
	}

	log.Printf("Sent output to SlackLiner for task %s", taskID)
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

	log.Printf("Received reaction: %s on message %s in channel %s", reaction.Reaction, reaction.MessageTS, reaction.Channel)

	// Only handle specific reactions
	if reaction.Reaction != "up_arrow" && reaction.Reaction != "down_arrow" && reaction.Reaction != "arrows_counterclockwise" {
		return
	}

	// Retrieve message from Slack to get metadata
	message, err := s.slackClient.GetMessage(ctx, reaction.Channel, reaction.MessageTS)
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
	switch reaction.Reaction {
	case "up_arrow":
		command = "docker compose up -d"
	case "down_arrow":
		command = "docker compose down"
	case "arrows_counterclockwise":
		command = "docker compose restart"
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
