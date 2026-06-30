package skills

import (
	"fmt"
	"strings"
)

func PromptBlock(registry *Registry, role Role) string {
	if registry == nil {
		return ""
	}
	available := registry.SkillsForRole(role)
	if len(available) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Skills Library\n\n")
	b.WriteString("Agent Skills are modular production instructions that extend this role's operating knowledge.\n")
	b.WriteString("Only skill metadata is visible here; full instructions must be loaded with load_agent_skill.\n\n")
	b.WriteString("Skill Utilization Protocol:\n")
	b.WriteString("1. During Think & Plan, identify whether the task matches any available skill description.\n")
	b.WriteString("2. If a relevant skill exists, call load_agent_skill(name=...) before executing related tools or finalizing the plan.\n")
	b.WriteString("3. If the loaded skill changes the plan, update the plan before acting.\n")
	b.WriteString("4. Follow loaded skill instructions unless they conflict with ClipAnvil system rules, role boundaries, tool schemas, DB facts, or user instructions.\n")
	b.WriteString("5. Never invent skills. Use only the names listed below.\n\n")
	b.WriteString("Available Skills:\n")
	for _, skill := range available {
		fmt.Fprintf(&b, "- %s: %s\n", skill.Name, skill.Description)
	}
	return strings.TrimSpace(b.String())
}
