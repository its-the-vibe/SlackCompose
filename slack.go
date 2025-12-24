package main

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"
)

// SlackClient wraps the Slack API client
type SlackClient struct {
	client *slack.Client
}

// NewSlackClient creates a new Slack client
func NewSlackClient(token string) *SlackClient {
	return &SlackClient{
		client: slack.New(token),
	}
}

// GetMessage retrieves a message from Slack with metadata
func (s *SlackClient) GetMessage(ctx context.Context, channel, timestamp string) (*SlackMessage, error) {
	// Get conversation history with the specific message
	params := &slack.GetConversationHistoryParameters{
		ChannelID:          channel,
		Latest:             timestamp,
		Limit:              1,
		Inclusive:          true,
		IncludeAllMetadata: true,
	}

	history, err := s.client.GetConversationHistoryContext(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	if len(history.Messages) == 0 {
		return nil, fmt.Errorf("message not found")
	}

	msg := history.Messages[0]

	// Convert Slack message to our format
	slackMsg := &SlackMessage{
		Type:      msg.Type,
		Text:      msg.Text,
		Timestamp: msg.Timestamp,
	}

	// Parse metadata if present
	if msg.Metadata.EventType != "" {
		slackMsg.Metadata.EventType = msg.Metadata.EventType
		slackMsg.Metadata.EventPayload = make(map[string]interface{})

		// Copy event payload
		for k, v := range msg.Metadata.EventPayload {
			slackMsg.Metadata.EventPayload[k] = v
		}
	}

	return slackMsg, nil
}
