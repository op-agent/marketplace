package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SkillContext struct {
	ID          string
	Slug        string
	Name        string
	Description string
	SkillFile   string
	SkillDir    string
}

func LoadMarkdownBody(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ExtractMarkdownBody(string(data)), nil
}

func ExtractMarkdownBody(content string) string {
	s := strings.TrimPrefix(content, "\ufeff")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return strings.TrimSpace(s)
	}
	for i := 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "---" || t == "..." {
			return strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		}
	}
	return strings.TrimSpace(s)
}

func ExpandPlatformVariables(prompt, platform string) string {
	if prompt == "" {
		return ""
	}
	return strings.ReplaceAll(prompt, "${platform}", strings.TrimSpace(platform))
}

func ReadCwdAgentsContext(cwd string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", nil
	}
	path := filepath.Join(cwd, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func ResolveCwdAgentsPath(cwd string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", nil
	}
	path, err := filepath.Abs(filepath.Join(cwd, "AGENTS.md"))
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return path, nil
}

func AppendCwdAgentsContext(basePrompt, agentsText string) string {
	agentsText = strings.TrimSpace(agentsText)
	if agentsText == "" {
		return basePrompt
	}
	appendix := "## Working Directory AGENTS.md\n\n" + agentsText
	if strings.TrimSpace(basePrompt) == "" {
		return appendix
	}
	return strings.TrimRight(basePrompt, "\n") + "\n\n" + appendix
}

func AppendCwdAgentsPathInstruction(basePrompt, agentsPath string) string {
	agentsPath = strings.TrimSpace(agentsPath)
	if agentsPath == "" {
		return basePrompt
	}
	appendix := strings.Join([]string{
		"## Working Directory AGENTS.md",
		"",
		fmt.Sprintf("If this thread has not already read the workspace AGENTS.md, first use the read tool to read: %s", agentsPath),
	}, "\n")
	if strings.TrimSpace(basePrompt) == "" {
		return appendix
	}
	return strings.TrimRight(basePrompt, "\n") + "\n\n" + appendix
}

func BuildSystemPrompt(basePrompt, cwdAgents string, availableSkills, selectedSkills []SkillContext, selectedSkillContext map[string]any) string {
	prompt := AppendCwdAgentsContext(basePrompt, cwdAgents)
	return buildSystemPromptWithSkills(prompt, availableSkills, selectedSkills, selectedSkillContext)
}

func BuildSystemPromptWithCwdAgentsPath(basePrompt, cwdAgentsPath string, availableSkills, selectedSkills []SkillContext, selectedSkillContext map[string]any) string {
	prompt := AppendCwdAgentsPathInstruction(basePrompt, cwdAgentsPath)
	return buildSystemPromptWithSkills(prompt, availableSkills, selectedSkills, selectedSkillContext)
}

func buildSystemPromptWithSkills(prompt string, availableSkills, selectedSkills []SkillContext, selectedSkillContext map[string]any) string {
	if len(selectedSkills) == 0 {
		return appendSkillsAppendix(prompt, availableSkills)
	}

	selectedIDs := make(map[string]struct{}, len(selectedSkills))
	for _, skill := range selectedSkills {
		if id := strings.TrimSpace(skill.ID); id != "" {
			selectedIDs[id] = struct{}{}
		}
	}

	filteredAvailable := make([]SkillContext, 0, len(availableSkills))
	for _, skill := range availableSkills {
		if _, exists := selectedIDs[strings.TrimSpace(skill.ID)]; exists {
			continue
		}
		filteredAvailable = append(filteredAvailable, skill)
	}

	prompt = appendSkillsAppendix(prompt, filteredAvailable)
	return appendSelectedSkillsAppendix(prompt, selectedSkills, selectedSkillContext)
}

func appendSkillsAppendix(basePrompt string, skills []SkillContext) string {
	skills = normalizeSkills(skills)
	if len(skills) == 0 {
		return basePrompt
	}

	lines := []string{
		"## Available Skills",
		"When a task matches one of the skills below, first use the read tool to read that skill's SKILL.md before acting.",
		"Resolve any relative paths mentioned in a skill against the listed skill directory.",
		"Skills do not define new tools. Execute the workflow with the existing bash/read/write/edit tools.",
		"",
	}

	for _, skill := range skills {
		lines = append(lines, fmt.Sprintf("- @skills/%s", skill.Slug))
		lines = append(lines, fmt.Sprintf("  name: %s", skill.Name))
		lines = append(lines, fmt.Sprintf("  description: %s", skill.Description))
		lines = append(lines, fmt.Sprintf("  skill file: %s", skill.SkillFile))
		lines = append(lines, fmt.Sprintf("  skill dir: %s", skill.SkillDir))
	}

	appendix := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if strings.TrimSpace(basePrompt) == "" {
		return appendix
	}
	return strings.TrimRight(basePrompt, "\n") + "\n\n" + appendix
}

func appendSelectedSkillsAppendix(basePrompt string, skills []SkillContext, selectedSkillContext map[string]any) string {
	skills = normalizeSkills(skills)
	if len(skills) == 0 {
		return basePrompt
	}

	lines := []string{
		"## Selected Skills For This Turn",
		"The user explicitly selected the following skills for this turn.",
		"Before acting, use the read tool to read each selected skill's SKILL.md.",
		"Apply the selected skills for this turn in addition to the base prompt.",
		"",
	}

	for _, skill := range skills {
		lines = append(lines, fmt.Sprintf("- @skills/%s", skill.Slug))
		lines = append(lines, fmt.Sprintf("  name: %s", skill.Name))
		lines = append(lines, fmt.Sprintf("  description: %s", skill.Description))
		lines = append(lines, fmt.Sprintf("  skill file: %s", skill.SkillFile))
		lines = append(lines, fmt.Sprintf("  skill dir: %s", skill.SkillDir))
	}

	if len(selectedSkillContext) > 0 {
		keys := make([]string, 0, len(selectedSkillContext))
		for key := range selectedSkillContext {
			if strings.TrimSpace(key) == "" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			lines = append(lines, "", "Runtime context for this turn:")
			for _, key := range keys {
				lines = append(lines, fmt.Sprintf("- %s: %v", key, selectedSkillContext[key]))
			}
		}
	}

	appendix := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if strings.TrimSpace(basePrompt) == "" {
		return appendix
	}
	return strings.TrimRight(basePrompt, "\n") + "\n\n" + appendix
}

func normalizeSkills(skills []SkillContext) []SkillContext {
	if len(skills) == 0 {
		return nil
	}
	result := make([]SkillContext, 0, len(skills))
	seen := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		id := strings.TrimSpace(skill.ID)
		slug := strings.TrimSpace(skill.Slug)
		name := strings.TrimSpace(skill.Name)
		description := strings.TrimSpace(skill.Description)
		skillFile := strings.TrimSpace(skill.SkillFile)
		skillDir := strings.TrimSpace(skill.SkillDir)
		if slug == "" {
			slug = strings.TrimSpace(filepath.Base(skillDir))
		}
		if slug == "" || name == "" || description == "" {
			continue
		}
		seenID := id
		if seenID == "" {
			seenID = slug
		}
		if _, exists := seen[seenID]; exists {
			continue
		}
		seen[seenID] = struct{}{}
		result = append(result, SkillContext{
			ID:          id,
			Slug:        slug,
			Name:        name,
			Description: description,
			SkillFile:   skillFile,
			SkillDir:    skillDir,
		})
	}
	return result
}
