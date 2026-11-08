package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSha = "0123456789abcdef0123456789abcdef01234567\n"

func mkdir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
		t.Fatal(err)
	}
}

func forkOf() *Fork {
	return &Fork{Source: Source{Repo: "github.com/x/y", Directory: "skills/a"}}
}

func wantOne(t *testing.T, findings []string, substr string) {
	t.Helper()
	if len(findings) != 1 || !strings.Contains(findings[0], substr) {
		t.Errorf("want one finding containing %q, got %v", substr, findings)
	}
}

func TestValidate_InvalidName(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "skills/Bad Name")
	skills := Skills{"Bad Name": {Type: "first_party"}}
	wantOne(t, Validate(root, skills), "not valid")
}

func TestValidate_Clean(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "skills/a")
	mkdir(t, root, "skills/b")
	writeFile(t, filepath.Join(root, ShaPath("a")), testSha)
	skills := Skills{
		"a": {Type: "forked", Fork: forkOf()},
		"b": {Type: "first_party"},
	}
	if f := Validate(root, skills); len(f) != 0 {
		t.Errorf("want no findings, got %v", f)
	}
}

func TestValidate_EmptyConfigNoSkillsDir(t *testing.T) {
	if f := Validate(t.TempDir(), Skills{}); len(f) != 0 {
		t.Errorf("want no findings, got %v", f)
	}
}

func TestValidate_DirWithoutKey(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "skills/orphan")
	wantOne(t, Validate(root, Skills{}), "orphan")
}

func TestValidate_KeyWithoutDir(t *testing.T) {
	root := t.TempDir()
	wantOne(t, Validate(root, Skills{"ghost": {Type: "first_party"}}), "ghost")
}

func TestValidate_ForkedNeedsFork(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "skills/a")
	writeFile(t, filepath.Join(root, ShaPath("a")), testSha)
	wantOne(t, Validate(root, Skills{"a": {Type: "forked"}}), "fork")
}

func TestValidate_FirstPartyForbidsFork(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "skills/a")
	wantOne(t, Validate(root, Skills{"a": {Type: "first_party", Fork: forkOf()}}), "fork")
}

func TestValidate_UnknownType(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "skills/a")
	wantOne(t, Validate(root, Skills{"a": {Type: "other"}}), "other")
}

func TestValidate_ForkedNeedsShaFile(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "skills/a")
	wantOne(t, Validate(root, Skills{"a": {Type: "forked", Fork: forkOf()}}), "last-commit-sha")
}

func TestValidate_FirstPartyForbidsShaFile(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "skills/a")
	writeFile(t, filepath.Join(root, ShaPath("a")), testSha)
	wantOne(t, Validate(root, Skills{"a": {Type: "first_party"}}), "last-commit-sha")
}

func TestValidate_FileInSkillsDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills/README.md"), "x")
	wantOne(t, Validate(root, Skills{}), "README.md")
}
