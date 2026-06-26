package toolloop

import (
	"strings"
	"testing"
)

func TestBeforeModelAddsReminderForContinuousMonitoredToolResults(t *testing.T) {
	messages := []Message{{Role: "user", MessageType: "text"}}
	for index := 0; index < 5; index++ {
		messages = append(messages,
			Message{Role: "assistant", MessageType: "tool_call", ToolName: "read_project_context"},
			Message{Role: "tool", MessageType: "tool_result", ToolName: "read_project_context"},
		)
	}

	state := BeforeModel(messages, 5, nil, DefaultConfig())

	if len(state.PendingReminders) != 1 {
		t.Fatalf("pending reminders = %#v, want one", state.PendingReminders)
	}
	if !strings.Contains(state.PendingReminders[0], "read_project_context") ||
		!strings.Contains(state.PendingReminders[0], "<system-reminder>") {
		t.Fatalf("unexpected reminder = %q", state.PendingReminders[0])
	}
}

func TestBeforeModelUsesPerRuleCooldown(t *testing.T) {
	messages := []Message{{Role: "user", MessageType: "text"}}
	for index := 0; index < 5; index++ {
		messages = append(messages,
			Message{Role: "assistant", MessageType: "tool_call", ToolName: "read_project_context"},
			Message{Role: "tool", MessageType: "tool_result", ToolName: "read_project_context"},
		)
	}

	first := BeforeModel(messages, 5, nil, Config{Threshold: 5, CooldownTurns: 3, MonitoredTools: []string{"read_project_context"}})
	second := BeforeModel(messages, 6, first.Cooldowns, Config{Threshold: 5, CooldownTurns: 3, MonitoredTools: []string{"read_project_context"}})
	third := BeforeModel(messages, 8, second.Cooldowns, Config{Threshold: 5, CooldownTurns: 3, MonitoredTools: []string{"read_project_context"}})

	if len(first.PendingReminders) != 1 {
		t.Fatalf("first reminders = %#v", first.PendingReminders)
	}
	if len(second.PendingReminders) != 0 {
		t.Fatalf("second reminders = %#v, want cooldown suppression", second.PendingReminders)
	}
	if len(third.PendingReminders) != 1 {
		t.Fatalf("third reminders = %#v, want reminder after cooldown", third.PendingReminders)
	}
}
