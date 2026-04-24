package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
)

// mockPubSub is a no-op PubSubInterface used in tests that don't exercise listeners
type mockPubSub struct{}

func (m *mockPubSub) Channel(opts ...redis.ChannelOption) <-chan *redis.Message {
	ch := make(chan *redis.Message)
	close(ch)
	return ch
}
func (m *mockPubSub) Close() error { return nil }

// mockRedisClient records RPush calls and optionally injects errors
type mockRedisClient struct {
	pushed  []mockPush
	pushErr error
}

type mockPush struct {
	key   string
	value interface{}
}

func (m *mockRedisClient) Subscribe(ctx context.Context, channel string) PubSubInterface {
	return &mockPubSub{}
}

func (m *mockRedisClient) RPush(ctx context.Context, key string, value interface{}) error {
	if m.pushErr != nil {
		return m.pushErr
	}
	m.pushed = append(m.pushed, mockPush{key: key, value: value})
	return nil
}

// mockSlackClient returns configurable GetMessage results
type mockSlackClient struct {
	message *SlackMessage
	err     error
}

func (m *mockSlackClient) GetMessage(ctx context.Context, channel, timestamp string) (*SlackMessage, error) {
	return m.message, m.err
}

// newTestService creates a Service wired with mock dependencies
func newTestService(rc *mockRedisClient, sc SlackClientInterface) *Service {
	if rc == nil {
		rc = &mockRedisClient{}
	}
	cfg := &Config{
		PoppitListName:      "poppit:notifications",
		SlackLinerListName:  "slack_messages",
		SlackChannel:        "#slack-compose",
		DockerLogsLineLimit: 100,
		Projects: map[string]ProjectConfig{
			"my-project": {Name: "my-project", WorkingDir: "/srv/my-project"},
		},
	}
	svc := &Service{config: cfg, redisClient: rc, slackClient: sc}
	return svc
}

// ---- expandCommand ----

func TestExpandCommand(t *testing.T) {
	svc := &Service{config: &Config{DockerLogsLineLimit: 50}}

	tests := []struct {
		input string
		want  string
	}{
		{"docker compose logs", "docker compose logs -n 50"},
		{"docker compose up -d", "docker compose up -d"},
		{"docker compose down", "docker compose down"},
		{"docker compose restart", "docker compose restart"},
		{"docker compose ps", "docker compose ps"},
	}

	for _, tt := range tests {
		if got := svc.expandCommand(tt.input); got != tt.want {
			t.Errorf("expandCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---- getCommandForEmoji ----

func TestGetCommandForEmoji(t *testing.T) {
	svc := &Service{config: &Config{DockerLogsLineLimit: 100}}

	tests := []struct {
		emoji   string
		wantCmd string
		wantOk  bool
	}{
		{EmojiUpArrow, "docker compose up -d", true},
		{EmojiDownArrow, "docker compose down", true},
		{EmojiArrowsCounterClockwise, "docker compose restart", true},
		{EmojiPageFacingUp, "docker compose logs -n 100", true},
		{"unknown_emoji", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		cmd, ok := svc.getCommandForEmoji(tt.emoji)
		if ok != tt.wantOk {
			t.Errorf("getCommandForEmoji(%q) ok = %v, want %v", tt.emoji, ok, tt.wantOk)
		}
		if ok && cmd != tt.wantCmd {
			t.Errorf("getCommandForEmoji(%q) = %q, want %q", tt.emoji, cmd, tt.wantCmd)
		}
	}
}

// ---- getCommandForActionID ----

func TestGetCommandForActionID(t *testing.T) {
	svc := &Service{config: &Config{DockerLogsLineLimit: 100}}

	tests := []struct {
		actionID string
		wantCmd  string
		wantOk   bool
	}{
		{ActionDockerUp, "docker compose up -d", true},
		{ActionDockerDown, "docker compose down", true},
		{ActionDockerRestart, "docker compose restart", true},
		{ActionDockerPS, "docker compose ps", true},
		{ActionDockerLogs, "docker compose logs -n 100", true},
		{"unknown_action", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		cmd, ok := svc.getCommandForActionID(tt.actionID)
		if ok != tt.wantOk {
			t.Errorf("getCommandForActionID(%q) ok = %v, want %v", tt.actionID, ok, tt.wantOk)
		}
		if ok && cmd != tt.wantCmd {
			t.Errorf("getCommandForActionID(%q) = %q, want %q", tt.actionID, cmd, tt.wantCmd)
		}
	}
}

// ---- handleCommand ----

func TestHandleCommand_KnownProject(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	payload := SlackCommand{
		Command:   "/slack-compose",
		Text:      "my-project",
		ChannelID: "C123",
	}
	data, _ := json.Marshal(payload)
	svc.handleCommand(context.Background(), string(data))

	if len(rc.pushed) != 1 {
		t.Fatalf("expected 1 push to Redis, got %d", len(rc.pushed))
	}
	if rc.pushed[0].key != "poppit:notifications" {
		t.Errorf("pushed to key %q, want %q", rc.pushed[0].key, "poppit:notifications")
	}

	// Verify the pushed payload
	var pp PoppitPayload
	if err := json.Unmarshal(rc.pushed[0].value.([]byte), &pp); err != nil {
		t.Fatalf("failed to unmarshal pushed payload: %v", err)
	}
	if pp.Repo != "my-project" {
		t.Errorf("Repo = %q, want %q", pp.Repo, "my-project")
	}
	if len(pp.Commands) != 1 || pp.Commands[0] != "docker compose ps" {
		t.Errorf("Commands = %v, want [docker compose ps]", pp.Commands)
	}
}

func TestHandleCommand_EmptyText_ShowsDialog(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	payload := SlackCommand{Command: "/slack-compose", Text: "", ChannelID: "C123"}
	data, _ := json.Marshal(payload)
	svc.handleCommand(context.Background(), string(data))

	// Dialog is sent to SlackLiner
	if len(rc.pushed) != 1 {
		t.Fatalf("expected 1 push to Redis (block kit dialog), got %d", len(rc.pushed))
	}
	if rc.pushed[0].key != "slack_messages" {
		t.Errorf("pushed to key %q, want %q", rc.pushed[0].key, "slack_messages")
	}
}

func TestHandleCommand_UnknownProject_ShowsDialog(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	payload := SlackCommand{Command: "/slack-compose", Text: "nonexistent", ChannelID: "C123"}
	data, _ := json.Marshal(payload)
	svc.handleCommand(context.Background(), string(data))

	if len(rc.pushed) != 1 {
		t.Fatalf("expected 1 push to Redis (block kit dialog), got %d", len(rc.pushed))
	}
	if rc.pushed[0].key != "slack_messages" {
		t.Errorf("pushed to key %q, want %q", rc.pushed[0].key, "slack_messages")
	}
}

func TestHandleCommand_WrongCommand_Ignored(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	payload := SlackCommand{Command: "/other-command", Text: "my-project"}
	data, _ := json.Marshal(payload)
	svc.handleCommand(context.Background(), string(data))

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 Redis pushes for unknown command, got %d", len(rc.pushed))
	}
}

func TestHandleCommand_InvalidJSON(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	// Should not panic and should not push anything
	svc.handleCommand(context.Background(), "not valid json")

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 Redis pushes for invalid JSON, got %d", len(rc.pushed))
	}
}

// ---- handlePoppitOutput ----

func TestHandlePoppitOutput_SendsToSlackLiner(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	out := PoppitCommandOutput{
		Type:    "slack-compose",
		Command: "docker compose ps",
		Output:  "container1 Up",
		Metadata: map[string]interface{}{
			"project": "my-project",
		},
	}
	data, _ := json.Marshal(out)
	svc.handlePoppitOutput(context.Background(), string(data))

	if len(rc.pushed) != 1 {
		t.Fatalf("expected 1 push to Redis, got %d", len(rc.pushed))
	}
	if rc.pushed[0].key != "slack_messages" {
		t.Errorf("pushed to key %q, want %q", rc.pushed[0].key, "slack_messages")
	}

	var slp SlackLinerPayload
	if err := json.Unmarshal(rc.pushed[0].value.([]byte), &slp); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if slp.Channel != "#slack-compose" {
		t.Errorf("Channel = %q, want %q", slp.Channel, "#slack-compose")
	}
}

func TestHandlePoppitOutput_WrongType_Ignored(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	out := PoppitCommandOutput{Type: "other-type", Command: "whatever"}
	data, _ := json.Marshal(out)
	svc.handlePoppitOutput(context.Background(), string(data))

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 Redis pushes, got %d", len(rc.pushed))
	}
}

func TestHandlePoppitOutput_WithThreadTS(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	out := PoppitCommandOutput{
		Type:    "slack-compose",
		Command: "docker compose up -d",
		Output:  "Starting...",
		Metadata: map[string]interface{}{
			"project":   "my-project",
			"thread_ts": "111.222",
			"channel":   "C999",
		},
	}
	data, _ := json.Marshal(out)
	svc.handlePoppitOutput(context.Background(), string(data))

	if len(rc.pushed) != 1 {
		t.Fatalf("expected 1 push, got %d", len(rc.pushed))
	}
	var slp SlackLinerPayload
	json.Unmarshal(rc.pushed[0].value.([]byte), &slp)

	if slp.ThreadTS != "111.222" {
		t.Errorf("ThreadTS = %q, want %q", slp.ThreadTS, "111.222")
	}
	if slp.Channel != "C999" {
		t.Errorf("Channel = %q, want %q", slp.Channel, "C999")
	}
}

func TestHandlePoppitOutput_WithStderr(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	out := PoppitCommandOutput{
		Type:     "slack-compose",
		Command:  "docker compose up -d",
		Output:   "",
		Stderr:   "error: something went wrong",
		Metadata: map[string]interface{}{"project": "my-project"},
	}
	data, _ := json.Marshal(out)
	svc.handlePoppitOutput(context.Background(), string(data))

	if len(rc.pushed) != 1 {
		t.Fatalf("expected 1 push, got %d", len(rc.pushed))
	}
	var slp SlackLinerPayload
	json.Unmarshal(rc.pushed[0].value.([]byte), &slp)
	if slp.Text == "" {
		t.Error("expected non-empty text containing stderr")
	}
}

// ---- handleReaction ----

func TestHandleReaction_KnownEmojiKnownProject(t *testing.T) {
	rc := &mockRedisClient{}
	sc := &mockSlackClient{
		message: &SlackMessage{
			Metadata: SlackMetadata{
				EventType:    "slack-compose",
				EventPayload: map[string]interface{}{"project": "my-project"},
			},
		},
	}
	svc := newTestService(rc, sc)

	reaction := SlackReaction{
		Event: SlackReactionEvent{
			Reaction: EmojiUpArrow,
			Item:     SlackReactionItem{Channel: "C123", TS: "111.222"},
		},
	}
	data, _ := json.Marshal(reaction)
	svc.handleReaction(context.Background(), string(data))

	if len(rc.pushed) != 1 {
		t.Fatalf("expected 1 push, got %d", len(rc.pushed))
	}
	if rc.pushed[0].key != "poppit:notifications" {
		t.Errorf("key = %q, want %q", rc.pushed[0].key, "poppit:notifications")
	}
	var pp PoppitPayload
	json.Unmarshal(rc.pushed[0].value.([]byte), &pp)
	if pp.Commands[0] != "docker compose up -d" {
		t.Errorf("command = %q, want %q", pp.Commands[0], "docker compose up -d")
	}
}

func TestHandleReaction_UnsupportedEmoji_Ignored(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	reaction := SlackReaction{
		Event: SlackReactionEvent{Reaction: "thumbsup"},
	}
	data, _ := json.Marshal(reaction)
	svc.handleReaction(context.Background(), string(data))

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 pushes for unsupported emoji, got %d", len(rc.pushed))
	}
}

func TestHandleReaction_SlackError_NoPush(t *testing.T) {
	rc := &mockRedisClient{}
	sc := &mockSlackClient{err: fmt.Errorf("slack API error")}
	svc := newTestService(rc, sc)

	reaction := SlackReaction{
		Event: SlackReactionEvent{
			Reaction: EmojiDownArrow,
			Item:     SlackReactionItem{Channel: "C123", TS: "111.222"},
		},
	}
	data, _ := json.Marshal(reaction)
	svc.handleReaction(context.Background(), string(data))

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 pushes when slack returns error, got %d", len(rc.pushed))
	}
}

func TestHandleReaction_NonSlackComposeMessage_Ignored(t *testing.T) {
	rc := &mockRedisClient{}
	sc := &mockSlackClient{
		message: &SlackMessage{
			Metadata: SlackMetadata{EventType: "other-event"},
		},
	}
	svc := newTestService(rc, sc)

	reaction := SlackReaction{
		Event: SlackReactionEvent{
			Reaction: EmojiUpArrow,
			Item:     SlackReactionItem{Channel: "C123", TS: "111.222"},
		},
	}
	data, _ := json.Marshal(reaction)
	svc.handleReaction(context.Background(), string(data))

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 pushes for non-slack-compose message, got %d", len(rc.pushed))
	}
}

// ---- handleBlockAction ----

func TestHandleBlockAction_KnownAction(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	action := SlackBlockAction{
		Type: "block_actions",
		Actions: []BlockActionElement{
			{ActionID: ActionDockerUp, Type: "button", Value: "up"},
		},
		State: BlockActionState{
			Values: map[string]map[string]BlockActionValue{
				BlockIDProjectBlock: {
					ActionIDSlackCompose: {
						Type: "external_select",
						SelectedOption: &BlockActionOption{
							Text:  BlockActionText{Type: "plain_text", Text: "my-project"},
							Value: "my-project",
						},
					},
				},
			},
		},
		Message: BlockActionMessage{TS: "123.456"},
		Channel: BlockActionChannel{ID: "C789"},
	}
	data, _ := json.Marshal(action)
	svc.handleBlockAction(context.Background(), string(data))

	if len(rc.pushed) != 1 {
		t.Fatalf("expected 1 push, got %d", len(rc.pushed))
	}
	if rc.pushed[0].key != "poppit:notifications" {
		t.Errorf("key = %q, want %q", rc.pushed[0].key, "poppit:notifications")
	}
	var pp PoppitPayload
	json.Unmarshal(rc.pushed[0].value.([]byte), &pp)
	if pp.Commands[0] != "docker compose up -d" {
		t.Errorf("command = %q, want %q", pp.Commands[0], "docker compose up -d")
	}
	if pp.Metadata["thread_ts"] != "123.456" {
		t.Errorf("thread_ts = %v, want %q", pp.Metadata["thread_ts"], "123.456")
	}
}

func TestHandleBlockAction_NoProjectSelected_Ignored(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	action := SlackBlockAction{
		Type:    "block_actions",
		Actions: []BlockActionElement{{ActionID: ActionDockerUp, Type: "button"}},
		State:   BlockActionState{Values: map[string]map[string]BlockActionValue{}},
	}
	data, _ := json.Marshal(action)
	svc.handleBlockAction(context.Background(), string(data))

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 pushes when no project selected, got %d", len(rc.pushed))
	}
}

func TestHandleBlockAction_UnknownProject_Ignored(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	action := SlackBlockAction{
		Type:    "block_actions",
		Actions: []BlockActionElement{{ActionID: ActionDockerUp, Type: "button"}},
		State: BlockActionState{
			Values: map[string]map[string]BlockActionValue{
				BlockIDProjectBlock: {
					ActionIDSlackCompose: {
						SelectedOption: &BlockActionOption{Value: "unknown-project"},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(action)
	svc.handleBlockAction(context.Background(), string(data))

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 pushes for unknown project, got %d", len(rc.pushed))
	}
}

func TestHandleBlockAction_NonButtonAction_Ignored(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	action := SlackBlockAction{
		Type:    "block_actions",
		Actions: []BlockActionElement{{ActionID: ActionDockerUp, Type: "static_select"}},
		State: BlockActionState{
			Values: map[string]map[string]BlockActionValue{
				BlockIDProjectBlock: {
					ActionIDSlackCompose: {
						SelectedOption: &BlockActionOption{Value: "my-project"},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(action)
	svc.handleBlockAction(context.Background(), string(data))

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 pushes for non-button action, got %d", len(rc.pushed))
	}
}

func TestHandleBlockAction_InvalidJSON(t *testing.T) {
	rc := &mockRedisClient{}
	svc := newTestService(rc, nil)

	svc.handleBlockAction(context.Background(), "invalid json")

	if len(rc.pushed) != 0 {
		t.Errorf("expected 0 pushes for invalid JSON, got %d", len(rc.pushed))
	}
}
