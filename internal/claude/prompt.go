// Package claude builds the prompt for Claude Code, runs it, and checks its output.
package claude

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/fergalhk/agent-config/internal/config"
)

//go:embed prompt.tmpl
var promptText string

var promptTemplate = template.Must(template.New("prompt").Option("missingkey=error").Parse(promptText))

// PromptData holds the values that the prompt template uses.
type PromptData struct {
	Skill        string
	Source       config.Source
	ForkDir      string
	NotesPath    string
	OldDir       string
	NewDir       string
	DiffPath     string
	ActivityPath string
	ResultPath   string
	OldSHA       string
	NewSHA       string
	// HasActivity is true when the activity file exists.
	HasActivity bool
	// Date is today's date in the form YYYY-MM-DD.
	Date string
}

// BuildPrompt fills the embedded template with the given data.
func BuildPrompt(d PromptData) (string, error) {
	var sb strings.Builder
	if err := promptTemplate.Execute(&sb, d); err != nil {
		return "", fmt.Errorf("build prompt: %w", err)
	}
	return sb.String(), nil
}
