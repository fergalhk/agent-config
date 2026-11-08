package discover

import (
	"strings"
	"testing"

	"github.com/fergalhk/agent-config/internal/config"
)

func TestNotesTemplate_HasSections(t *testing.T) {
	src := config.Source{Repo: "github.com/obra/superpowers", Directory: "skills/brainstorming"}
	out, err := NotesTemplate("sp-brainstorming", src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## How I want this skill to differ from upstream",
		"## How to handle upstream changes",
		"## History of decisions",
		"github.com/obra/superpowers",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("notes do not contain %q:\n%s", want, out)
		}
	}
}

func TestPRBody_NamesIgnore(t *testing.T) {
	src := config.Source{Repo: "github.com/obra/superpowers", Directory: "skills/brainstorming"}
	sha := "0123456789abcdef0123456789abcdef01234567"
	out, err := PRBody("sp-brainstorming", src, sha, "brainstorming")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"github.com/obra/superpowers",
		"skills/brainstorming",
		sha,
		"ignore",
		"`brainstorming`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("PR body does not contain %q:\n%s", want, out)
		}
	}
}
