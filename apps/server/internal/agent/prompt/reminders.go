package prompt

import (
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/toolloop"
)

func AppendPendingReminders(messages []*schema.Message, reminders []string) []*schema.Message {
	text := pendingReminderText(reminders)
	if text == "" {
		return messages
	}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil {
			continue
		}
		if message.Role != schema.User && message.Role != schema.Tool {
			continue
		}
		message.Content = strings.TrimSpace(message.Content) + "\n\n" + text
		return messages
	}
	return append(messages, schema.UserMessage(text))
}

func pendingReminderText(reminders []string) string {
	normalized := make([]string, 0, len(reminders))
	for _, reminder := range reminders {
		if text := toolloop.NormalizeSystemReminder(reminder); text != "" {
			normalized = append(normalized, text)
		}
	}
	if len(normalized) == 0 {
		return ""
	}
	return "运行时提醒：\n" + strings.Join(normalized, "\n")
}
