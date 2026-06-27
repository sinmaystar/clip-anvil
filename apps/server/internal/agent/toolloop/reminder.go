package toolloop

import (
	"fmt"
	"slices"
	"strings"
)

const RuleContinuousToolCall = "continuous_tool_call"

type Config struct {
	Threshold      int
	CooldownTurns  int
	MonitoredTools []string
}

type Message struct {
	Role        string
	MessageType string
	ToolName    string
}

type State struct {
	PendingReminders []string
	Cooldowns        map[string]int
}

func DefaultConfig() Config {
	return Config{
		Threshold:     5,
		CooldownTurns: 3,
		MonitoredTools: []string{
			"read_project_context",
			"read_project_memory",
		},
	}
}

func BeforeModel(messages []Message, iteration int, cooldowns map[string]int, config Config) State {
	config = normalizeConfig(config)
	nextCooldowns := cloneCooldowns(cooldowns)
	toolName, count := continuousToolCall(messages)
	if toolName == "" || count < config.Threshold || !slices.Contains(config.MonitoredTools, toolName) {
		return State{Cooldowns: nextCooldowns}
	}
	ruleKey := RuleContinuousToolCall + ":" + toolName
	last, seen := nextCooldowns[ruleKey]
	if seen && iteration-last < config.CooldownTurns {
		return State{Cooldowns: nextCooldowns}
	}
	nextCooldowns[ruleKey] = iteration
	return State{
		PendingReminders: []string{ContinuousToolCallReminder(toolName, count)},
		Cooldowns:        nextCooldowns,
	}
}

func ContinuousToolCallReminder(toolName string, count int) string {
	return fmt.Sprintf("<system-reminder>你在当前任务范围内已连续调用工具 %s %d 次。请暂停重复调用，先反思当前策略是否有效：是否已经获得足够信息、是否应该改用决策/写入/委派工具、是否需要向用户说明阻塞点，或换一种推进方法。</system-reminder>", strings.TrimSpace(toolName), count)
}

func NormalizeSystemReminder(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.Contains(text, "<system-reminder>") {
		return text
	}
	return "<system-reminder>" + text + "</system-reminder>"
}

func continuousToolCall(messages []Message) (string, int) {
	start := 0
	for index := len(messages) - 1; index >= 0; index-- {
		if strings.TrimSpace(messages[index].Role) == "user" {
			start = index + 1
			break
		}
	}
	toolName := ""
	count := 0
	for index := len(messages) - 1; index >= start; index-- {
		message := messages[index]
		role := strings.TrimSpace(message.Role)
		messageType := strings.TrimSpace(message.MessageType)
		if role == "assistant" && messageType == "tool_call" {
			continue
		}
		if role != "tool" || messageType != "tool_result" {
			break
		}
		current := strings.TrimSpace(message.ToolName)
		if current == "" {
			break
		}
		if toolName == "" {
			toolName = current
		}
		if current != toolName {
			break
		}
		count++
	}
	return toolName, count
}

func normalizeConfig(config Config) Config {
	if config.Threshold <= 0 {
		config.Threshold = 5
	}
	if config.CooldownTurns <= 0 {
		config.CooldownTurns = 3
	}
	if len(config.MonitoredTools) == 0 {
		config.MonitoredTools = DefaultConfig().MonitoredTools
	}
	return config
}

func cloneCooldowns(input map[string]int) map[string]int {
	out := map[string]int{}
	for key, value := range input {
		out[key] = value
	}
	return out
}
