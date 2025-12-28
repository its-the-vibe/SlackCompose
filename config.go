package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds all configuration for the service
type Config struct {
	// Redis configuration
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Service configuration
	SlackCommandChannel      string // Redis channel to listen for Slack commands
	SlackReactionChannel     string // Redis channel to listen for Slack reactions
	SlackBlockActionsChannel string // Redis channel to listen for Slack block actions
	PoppitListName           string // Redis list name for Poppit notifications
	PoppitOutputChannel      string // Redis channel to listen for Poppit command output
	SlackLinerListName       string // Redis list name for SlackLiner messages
	SlackToken               string // Slack API token
	SlackChannel             string // Slack channel to post to (e.g., #slack-compose)

	// Project configuration file path
	ProjectConfigPath string

	// Project mappings (loaded from config file)
	Projects map[string]ProjectConfig
}

// ProjectConfig maps a project name to its working directory
type ProjectConfig struct {
	Name       string `json:"name"`
	WorkingDir string `json:"working_dir"`
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		RedisAddr:                getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:            getEnv("REDIS_PASSWORD", ""),
		RedisDB:                  getEnvInt("REDIS_DB", 0),
		SlackCommandChannel:      getEnv("SLACK_COMMAND_CHANNEL", "slack-commands"),
		SlackReactionChannel:     getEnv("SLACK_REACTION_CHANNEL", "slack-reactions"),
		SlackBlockActionsChannel: getEnv("SLACK_BLOCK_ACTIONS_CHANNEL", "slack-relay-block-actions"),
		PoppitListName:           getEnv("POPPIT_LIST_NAME", "poppit:notifications"),
		PoppitOutputChannel:      getEnv("POPPIT_OUTPUT_CHANNEL", "poppit:command-output"),
		SlackLinerListName:       getEnv("SLACKLINER_LIST_NAME", "slack_messages"),
		SlackToken:               getEnv("SLACK_BOT_TOKEN", ""),
		SlackChannel:             getEnv("SLACK_CHANNEL", "#slack-compose"),
		ProjectConfigPath:        getEnv("PROJECT_CONFIG_PATH", "projects.json"),
	}

	// Load project configuration
	if err := config.loadProjectConfig(); err != nil {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

	// Validate required fields
	if config.SlackToken == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN is required")
	}

	return config, nil
}

// loadProjectConfig loads project mappings from JSON file
func (c *Config) loadProjectConfig() error {
	// If file doesn't exist, initialize with empty map
	if _, err := os.Stat(c.ProjectConfigPath); os.IsNotExist(err) {
		c.Projects = make(map[string]ProjectConfig)
		return nil
	}

	data, err := os.ReadFile(c.ProjectConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read project config file: %w", err)
	}

	var projects []ProjectConfig
	if err := json.Unmarshal(data, &projects); err != nil {
		return fmt.Errorf("failed to parse project config: %w", err)
	}

	c.Projects = make(map[string]ProjectConfig)
	for _, p := range projects {
		c.Projects[p.Name] = p
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}
