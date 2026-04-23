package main

import (
	"encoding/json"
	"testing"
)

func TestPoppitPayload_JSONRoundTrip(t *testing.T) {
	payload := PoppitPayload{
		Repo:     "my-project",
		Branch:   "refs/heads/main",
		Type:     "slack-compose",
		Dir:      "/path/to/project",
		Commands: []string{"docker compose ps", "docker compose up -d"},
		Metadata: map[string]interface{}{
			"project":   "my-project",
			"thread_ts": "1234567890.123456",
			"channel":   "C1234567890",
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got PoppitPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.Repo != payload.Repo {
		t.Errorf("Repo = %q, want %q", got.Repo, payload.Repo)
	}
	if got.Branch != payload.Branch {
		t.Errorf("Branch = %q, want %q", got.Branch, payload.Branch)
	}
	if got.Type != payload.Type {
		t.Errorf("Type = %q, want %q", got.Type, payload.Type)
	}
	if got.Dir != payload.Dir {
		t.Errorf("Dir = %q, want %q", got.Dir, payload.Dir)
	}
	if len(got.Commands) != 2 || got.Commands[0] != "docker compose ps" {
		t.Errorf("Commands = %v, want %v", got.Commands, payload.Commands)
	}
	if project, ok := got.Metadata["project"].(string); !ok || project != "my-project" {
		t.Errorf("Metadata project = %v, want %q", got.Metadata["project"], "my-project")
	}
}

func TestSlackLinerPayload_TextMessage(t *testing.T) {
	payload := SlackLinerPayload{
		Channel: "#slack-compose",
		Text:    "Hello from SlackCompose",
		Metadata: SlackMetadata{
			EventType: "slack-compose",
			EventPayload: map[string]interface{}{
				"project": "test-project",
				"command": "docker compose ps",
			},
		},
		TTL: DefaultTTLSeconds,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got SlackLinerPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.Channel != payload.Channel {
		t.Errorf("Channel = %q, want %q", got.Channel, payload.Channel)
	}
	if got.Text != payload.Text {
		t.Errorf("Text = %q, want %q", got.Text, payload.Text)
	}
	if got.TTL != DefaultTTLSeconds {
		t.Errorf("TTL = %d, want %d", got.TTL, DefaultTTLSeconds)
	}
	if got.Metadata.EventType != "slack-compose" {
		t.Errorf("EventType = %q, want %q", got.Metadata.EventType, "slack-compose")
	}
}

func TestSlackLinerPayload_OmitEmpty(t *testing.T) {
	payload := SlackLinerPayload{
		Channel: "#slack-compose",
		Text:    "some text",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Blocks and ThreadTS should be omitted when empty
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if _, ok := raw["blocks"]; ok {
		t.Error("blocks field should be omitted when nil")
	}
	if _, ok := raw["thread_ts"]; ok {
		t.Error("thread_ts field should be omitted when empty")
	}
}

func TestSlackCommand_JSONUnmarshal(t *testing.T) {
	raw := `{
		"command": "/slack-compose",
		"text": "my-project",
		"user_id": "U123",
		"user_name": "alice",
		"channel_id": "C456",
		"channel_name": "general"
	}`

	var cmd SlackCommand
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if cmd.Command != "/slack-compose" {
		t.Errorf("Command = %q, want %q", cmd.Command, "/slack-compose")
	}
	if cmd.Text != "my-project" {
		t.Errorf("Text = %q, want %q", cmd.Text, "my-project")
	}
	if cmd.UserID != "U123" {
		t.Errorf("UserID = %q, want %q", cmd.UserID, "U123")
	}
	if cmd.ChannelID != "C456" {
		t.Errorf("ChannelID = %q, want %q", cmd.ChannelID, "C456")
	}
}

func TestPoppitCommandOutput_JSONUnmarshal(t *testing.T) {
	raw := `{
		"type": "slack-compose",
		"command": "docker compose ps",
		"output": "container1 running",
		"stderr": "",
		"metadata": {"project": "my-project", "thread_ts": "123.456"}
	}`

	var out PoppitCommandOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if out.Type != "slack-compose" {
		t.Errorf("Type = %q, want %q", out.Type, "slack-compose")
	}
	if out.Command != "docker compose ps" {
		t.Errorf("Command = %q, want %q", out.Command, "docker compose ps")
	}
	if out.Output != "container1 running" {
		t.Errorf("Output = %q, want %q", out.Output, "container1 running")
	}
	if proj, ok := out.Metadata["project"].(string); !ok || proj != "my-project" {
		t.Errorf("Metadata project = %v, want %q", out.Metadata["project"], "my-project")
	}
}

func TestSlackBlockAction_JSONUnmarshal(t *testing.T) {
	raw := `{
		"type": "block_actions",
		"actions": [{"action_id": "docker_up", "block_id": "b1", "type": "button", "value": "up"}],
		"state": {
			"values": {
				"project_block": {
					"SlackCompose": {
						"type": "external_select",
						"selected_option": {
							"text": {"type": "plain_text", "text": "my-project"},
							"value": "my-project"
						}
					}
				}
			}
		},
		"message": {"ts": "1234567890.123456"},
		"channel": {"id": "C1234567890", "name": "slack-compose"}
	}`

	var action SlackBlockAction
	if err := json.Unmarshal([]byte(raw), &action); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if action.Type != "block_actions" {
		t.Errorf("Type = %q, want %q", action.Type, "block_actions")
	}
	if len(action.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(action.Actions))
	}
	if action.Actions[0].ActionID != "docker_up" {
		t.Errorf("ActionID = %q, want %q", action.Actions[0].ActionID, "docker_up")
	}
	if action.Channel.ID != "C1234567890" {
		t.Errorf("Channel.ID = %q, want %q", action.Channel.ID, "C1234567890")
	}
	if action.Message.TS != "1234567890.123456" {
		t.Errorf("Message.TS = %q, want %q", action.Message.TS, "1234567890.123456")
	}

	// Check state extraction
	proj := action.State.Values[BlockIDProjectBlock][ActionIDSlackCompose].SelectedOption.Value
	if proj != "my-project" {
		t.Errorf("SelectedOption.Value = %q, want %q", proj, "my-project")
	}
}
