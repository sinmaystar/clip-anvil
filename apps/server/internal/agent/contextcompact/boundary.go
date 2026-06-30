package contextcompact

import "github.com/cloudwego/eino/schema"

func CurrentToolLoopFromIndex(baseLen int, sameTurnCount int) int {
	if sameTurnCount <= 0 {
		return 0
	}
	protectCount := 2
	if sameTurnCount < protectCount {
		protectCount = sameTurnCount
	}
	return baseLen + sameTurnCount - protectCount
}

func PendingReminderTargetIndex(messages []*schema.Message, reminders []string) int {
	if len(reminders) == 0 {
		return 0
	}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil {
			continue
		}
		if message.Role == schema.User || message.Role == schema.Tool {
			return index
		}
	}
	return len(messages)
}
