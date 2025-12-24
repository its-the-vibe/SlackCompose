package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// sendToPoppit sends a payload to the Poppit service via Redis list
func (s *Service) sendToPoppit(ctx context.Context, payload PoppitPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	if err := s.redisClient.RPush(ctx, s.config.PoppitListName, data); err != nil {
		return fmt.Errorf("failed to push to Redis list: %w", err)
	}

	return nil
}

// sendToSlackLiner sends a payload to the SlackLiner service via Redis list
func (s *Service) sendToSlackLiner(ctx context.Context, payload SlackLinerPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	if err := s.redisClient.RPush(ctx, s.config.SlackLinerListName, data); err != nil {
		return fmt.Errorf("failed to push to Redis list: %w", err)
	}

	return nil
}
