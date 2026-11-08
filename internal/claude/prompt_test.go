package claude

import (
	"strings"
	"testing"

	"github.com/fergalhk/agent-config/internal/config"
)

func data() PromptData {
	return PromptData{
		Skill:        "sp-x",
		Source:       config.Source{Repo: "https://example.com/up.git", Directory: "skills/x"},
		ForkDir:      "/work/skills/sp-x",
		NotesPath:    "/work/docs/agents/skills/sp-x.md",
		OldDir:       "/scratch/old",
		NewDir:       "/scratch/new",
		DiffPath:     "/scratch/diff.patch",
		ActivityPath: "/scratch/activity.md",
		ResultPath:   "/scratch/result.md",
		OldSHA:       "1111111111111111111111111111111111111111",
		NewSHA:       "2222222222222222222222222222222222222222",
		Date:         "2026-11-08",
	}
}

func TestBuildPrompt_ContainsPathsAndRules(t *testing.T) {
	d := data()
	out, err := BuildPrompt(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		d.ForkDir, d.NotesPath, d.ResultPath, d.OldSHA, d.NewSHA, d.Date,
		"Read the notes file first.",
		"A human comment ranks above the notes file.",
		"Do not edit any file outside the fork directory, the notes file and the result file.",
		"## What changed upstream", "## What I applied",
		"## What I did not apply and why", "## Notes file changes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestBuildPrompt_Activity(t *testing.T) {
	d := data()
	out, err := BuildPrompt(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, d.ActivityPath) {
		t.Error("prompt mentions activity file without activity")
	}
	d.HasActivity = true
	out, err = BuildPrompt(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, d.ActivityPath) {
		t.Error("prompt lacks activity file with activity")
	}
}
