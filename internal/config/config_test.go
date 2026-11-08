package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSkills_ParsesForkedAndFirstParty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "skills.yaml")
	writeFile(t, p, `sp-brainstorming:
  type: forked
  fork:
    source:
      repo: github.com/obra/superpowers
      directory: skills/brainstorming
mine:
  type: first_party
`)
	skills, err := LoadSkills(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("want 2 skills, got %d", len(skills))
	}
	f := skills["sp-brainstorming"]
	if f.Type != "forked" || f.Fork == nil ||
		f.Fork.Source.Repo != "github.com/obra/superpowers" ||
		f.Fork.Source.Directory != "skills/brainstorming" {
		t.Errorf("unexpected forked skill: %+v", f)
	}
	m := skills["mine"]
	if m.Type != "first_party" || m.Fork != nil {
		t.Errorf("unexpected first party skill: %+v", m)
	}
	if got := skills.Forked(); !reflect.DeepEqual(got, []string{"sp-brainstorming"}) {
		t.Errorf("Forked() = %v", got)
	}
}

func TestLoadSkills_EmptyFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "skills.yaml")
	writeFile(t, p, "{}\n")
	skills, err := LoadSkills(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("want empty map, got %v", skills)
	}
}

func TestLoadForkSources_Parses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "forks.yaml")
	writeFile(t, p, `- source:
    repository: github.com/obra/superpowers
    directory: skills/
  prefix: sp-
  ignore: [foo]
`)
	got, err := LoadForkSources(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	e := got[0]
	if e.Source.Repository != "github.com/obra/superpowers" || e.Source.Directory != "skills/" ||
		e.Prefix != "sp-" || !reflect.DeepEqual(e.Ignore, []string{"foo"}) {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestReadSha_RejectsShortValue(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ShaPath("a")), "abc\n")
	if _, err := ReadSha(root, "a"); err == nil {
		t.Error("want error for short sha")
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	writeFile(t, filepath.Join(root, ShaPath("b")), sha+"\n")
	got, err := ReadSha(root, "b")
	if err != nil {
		t.Fatal(err)
	}
	if got != sha {
		t.Errorf("got %q", got)
	}
}
