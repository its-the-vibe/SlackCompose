package main

// SlackCommand represents a command received from SlackCommandRelay
type SlackCommand struct {
	Command     string `json:"command"`
	Text        string `json:"text"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
}

// SlackReaction represents an emoji reaction event from SlackRelay
type SlackReaction struct {
	Type      string `json:"type"`
	Reaction  string `json:"reaction"`
	UserID    string `json:"user_id"`
	Channel   string `json:"channel"`
	MessageTS string `json:"message_ts"`
}

// PoppitPayload is the payload sent to Poppit service
type PoppitPayload struct {
	Repo     string   `json:"repo"`
	Branch   string   `json:"branch"`
	Type     string   `json:"type"`
	Dir      string   `json:"dir"`
	Commands []string `json:"commands"`
	TaskID   string   `json:"taskId"`
}

// SlackLinerPayload is the payload sent to SlackLiner service
type SlackLinerPayload struct {
	Channel  string        `json:"channel"`
	Text     string        `json:"text"`
	Metadata SlackMetadata `json:"metadata"`
}

// SlackMetadata contains metadata for Slack messages
type SlackMetadata struct {
	EventType    string                 `json:"event_type"`
	EventPayload map[string]interface{} `json:"event_payload"`
}

// SlackMessage represents a message retrieved from Slack API
type SlackMessage struct {
	Type      string        `json:"type"`
	Text      string        `json:"text"`
	Timestamp string        `json:"ts"`
	Metadata  SlackMetadata `json:"metadata"`
}

// PoppitCommandOutput represents output from Poppit command execution
type PoppitCommandOutput struct {
	TaskID  string `json:"taskId"`
	Type    string `json:"type"`
	Command string `json:"command"`
	Output  string `json:"output"`
}
