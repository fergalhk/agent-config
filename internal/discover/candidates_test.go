package discover

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fergalhk/agent-config/internal/config"
	"github.com/fergalhk/agent-config/internal/github"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListCandidates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "skills/a/SKILL.md"), "a")
	writeTestFile(t, filepath.Join(root, "skills/b/notes.md"), "b")
	writeTestFile(t, filepath.Join(root, "skills/README.md"), "readme")
	writeTestFile(t, filepath.Join(root, "skills/c/SKILL.md"), "c")
	writeTestFile(t, filepath.Join(root, "skills/bad name/SKILL.md"), "bad")
	writeTestFile(t, filepath.Join(root, "skills/bad#name/SKILL.md"), "bad")

	var src config.ForkSource
	src.Source.Repository = "github.com/obra/superpowers"
	src.Source.Directory = "skills/"
	src.Prefix = "sp-"

	got, err := ListCandidates(root, src)
	if err != nil {
		t.Fatal(err)
	}
	want := []Candidate{{Dir: "skills/a", Name: "sp-a"}, {Dir: "skills/c", Name: "sp-c"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDecideAdd(t *testing.T) {
	open := github.PR{Number: 1, State: "open"}
	closed := github.PR{Number: 2, State: "closed"}
	merged := github.PR{Number: 3, State: "closed", Merged: true}
	for _, tc := range []struct {
		name     string
		prs      []github.PR
		behindBy int
		want     AddAction
		wantPR   int
	}{
		{"no PR", nil, 0, AddCreate, 0},
		{"merged PR only", []github.PR{merged}, 0, AddCreate, 0},
		{"open and current", []github.PR{open}, 0, AddSkipOpen, 1},
		{"open and behind", []github.PR{open}, 2, AddRebuild, 1},
		{"closed unmerged", []github.PR{closed}, 0, AddSkipClosed, 2},
		{"open wins over closed", []github.PR{closed, open}, 1, AddRebuild, 1},
	} {
		got, pr := DecideAdd(tc.prs, tc.behindBy)
		gotPR := 0
		if pr != nil {
			gotPR = pr.Number
		}
		if got != tc.want || gotPR != tc.wantPR {
			t.Errorf("%s: got (%v, #%d), want (%v, #%d)", tc.name, got, gotPR, tc.want, tc.wantPR)
		}
	}
}

func TestNewCandidates(t *testing.T) {
	cands := []Candidate{
		{Dir: "skills/a", Name: "sp-a"},
		{Dir: "skills/b", Name: "sp-b"},
		{Dir: "skills/c", Name: "sp-c"},
	}
	skills := config.Skills{"sp-a": {Type: "forked"}}
	got := NewCandidates(cands, skills, []string{"b"})
	want := []Candidate{{Dir: "skills/c", Name: "sp-c"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAddSkillEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.yaml")
	writeTestFile(t, path, "{}\n")

	srcA := config.Source{Repo: "github.com/obra/superpowers", Directory: "skills/a"}
	if err := AddSkillEntry(path, "sp-a", srcA); err != nil {
		t.Fatal(err)
	}
	skills, err := config.LoadSkills(path)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := skills["sp-a"]
	if !ok || a.Type != "forked" || a.Fork == nil || a.Fork.Source != srcA {
		t.Fatalf("sp-a entry is wrong: %+v", skills)
	}

	srcB := config.Source{Repo: "github.com/obra/superpowers", Directory: "skills/b"}
	if err := AddSkillEntry(path, "sp-b", srcB); err != nil {
		t.Fatal(err)
	}
	skills, err = config.LoadSkills(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 || skills["sp-a"].Fork == nil || skills["sp-a"].Fork.Source != srcA ||
		skills["sp-b"].Fork == nil || skills["sp-b"].Fork.Source != srcB {
		t.Fatalf("entries are wrong: %+v", skills)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ia, ib := strings.Index(string(data), "sp-a:"), strings.Index(string(data), "sp-b:")
	if ia < 0 || ib < 0 || ia > ib {
		t.Fatalf("want sp-a before sp-b, got:\n%s", data)
	}
}
