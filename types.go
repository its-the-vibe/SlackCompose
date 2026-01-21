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
	Type  string             `json:"type"`
	Event SlackReactionEvent `json:"event"`
}

// SlackReactionEvent contains the reaction event details
type SlackReactionEvent struct {
	Type     string            `json:"type"`
	User     string            `json:"user"`
	Reaction string            `json:"reaction"`
	Item     SlackReactionItem `json:"item"`
}

// SlackReactionItem contains the message item that was reacted to
type SlackReactionItem struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	TS      string `json:"ts"`
}

// PoppitPayload is the payload sent to Poppit service
type PoppitPayload struct {
	Repo     string                 `json:"repo"`
	Branch   string                 `json:"branch"`
	Type     string                 `json:"type"`
	Dir      string                 `json:"dir"`
	Commands []string               `json:"commands"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SlackLinerPayload is the payload sent to SlackLiner service
type SlackLinerPayload struct {
	Channel  string        `json:"channel"`
	Text     string        `json:"text,omitempty"`
	Blocks   interface{}   `json:"blocks,omitempty"` // Block Kit blocks
	Metadata SlackMetadata `json:"metadata,omitempty"`
	TTL      int           `json:"ttl,omitempty"`       // Time to live in seconds
	ThreadTS string        `json:"thread_ts,omitempty"` // Thread timestamp for posting replies
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
	Type     string                 `json:"type"`
	Command  string                 `json:"command"`
	Output   string                 `json:"output"`
	Stderr   string                 `json:"stderr"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SlackBlockAction represents a block action event from SlackRelay
type SlackBlockAction struct {
	Type    string               `json:"type"`
	Actions []BlockActionElement `json:"actions"`
	State   BlockActionState     `json:"state"`
	Message BlockActionMessage   `json:"message,omitempty"`
	Channel BlockActionChannel   `json:"channel,omitempty"`
}

// BlockActionElement represents an individual action element
type BlockActionElement struct {
	ActionID string `json:"action_id"`
	BlockID  string `json:"block_id"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

// BlockActionState represents the state of form inputs
type BlockActionState struct {
	Values map[string]map[string]BlockActionValue `json:"values"`
}

// BlockActionValue represents a value from a block action
type BlockActionValue struct {
	Type           string             `json:"type"`
	SelectedOption *BlockActionOption `json:"selected_option"`
}

// BlockActionOption represents a selected option
type BlockActionOption struct {
	Text  BlockActionText `json:"text"`
	Value string          `json:"value"`
}

// BlockActionText represents text in a block action
type BlockActionText struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

// BlockActionMessage represents the message containing the block action
type BlockActionMessage struct {
	TS string `json:"ts"`
}

// BlockActionChannel represents the channel where the action occurred
type BlockActionChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
