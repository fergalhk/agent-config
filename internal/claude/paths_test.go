package claude

import (
	"strings"
	"testing"
)

func TestCheckChangedPaths_Allowed(t *testing.T) {
	err := CheckChangedPaths([]string{
		"skills/sp-x/SKILL.md",
		"skills/sp-x/scripts/a.sh",
		"docs/agents/skills/sp-x.md",
	}, "sp-x")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckChangedPaths_Rejects(t *testing.T) {
	bad := []string{
		"skills/sp-y/SKILL.md",
		"internal/config/skills.yaml",
		"docs/agents/skills/sp-x.md.bak",
		"skills/sp-x/../sp-y/SKILL.md",
	}
	err := CheckChangedPaths(bad, "sp-x")
	if err == nil {
		t.Fatal("expected error")
	}
	for _, p := range bad {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q lacks %q", err, p)
		}
	}
}
