// Package discover finds new skills in upstream sources and raises a PR for each one.
package discover

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/fergalhk/agent-config/internal/config"
	"github.com/fergalhk/agent-config/internal/github"
	"github.com/fergalhk/agent-config/internal/naming"
)

//go:embed notes.tmpl
var notesText string

//go:embed prbody.tmpl
var prBodyText string

var (
	notesTemplate  = template.Must(template.New("notes").Option("missingkey=error").Parse(notesText))
	prBodyTemplate = template.Must(template.New("prbody").Option("missingkey=error").Parse(prBodyText))
)

// Candidate is a skill directory in an upstream repository.
type Candidate struct {
	// Dir is the path of the skill directory inside the upstream repository.
	Dir string
	// Name is the local skill name.
	Name string
}

// ListCandidates returns the immediate child directories of the source directory
// that contain SKILL.md. It skips a directory with an invalid local name.
// The result is sorted by name.
func ListCandidates(upstreamRoot string, src config.ForkSource) ([]Candidate, error) {
	base := strings.Trim(src.Source.Directory, "/")
	entries, err := os.ReadDir(filepath.Join(upstreamRoot, filepath.FromSlash(base)))
	if err != nil {
		return nil, fmt.Errorf("read upstream directory %s: %w", base, err)
	}
	cands := []Candidate{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join(base, e.Name())
		_, err := os.Stat(filepath.Join(upstreamRoot, filepath.FromSlash(dir), "SKILL.md"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("check %s/SKILL.md: %w", dir, err)
		}
		name := naming.LocalName(src.Prefix, dir)
		if !naming.ValidName(name) {
			log.Printf("skip %s: %q is not a valid skill name", dir, name)
			continue
		}
		cands = append(cands, Candidate{Dir: dir, Name: name})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].Name < cands[j].Name })
	return cands, nil
}

// NewCandidates drops the candidates that skills already lists, or whose
// directory base name is in ignore.
func NewCandidates(cands []Candidate, skills config.Skills, ignore []string) []Candidate {
	out := []Candidate{}
	for _, c := range cands {
		if _, ok := skills[c.Name]; ok {
			continue
		}
		if slices.Contains(ignore, path.Base(c.Dir)) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// AddAction is the action of the discovery job for one candidate.
type AddAction int

const (
	// AddCreate creates the branch and opens a new PR.
	AddCreate AddAction = iota
	// AddSkipOpen skips the candidate. An open PR exists and its branch is current.
	AddSkipOpen
	// AddSkipClosed skips the candidate. A closed unmerged PR exists.
	AddSkipClosed
	// AddRebuild builds the branch of the open PR again from main and pushes it with force.
	AddRebuild
)

// OpenPR returns the first open PR in prs, or nil.
func OpenPR(prs []github.PR) *github.PR {
	for i := range prs {
		if prs[i].State == "open" {
			return &prs[i]
		}
	}
	return nil
}

// DecideAdd returns the action for a candidate. prs holds the PRs for the
// add branch. behindBy is the number of main commits that the branch of the
// open PR does not have. DecideAdd uses behindBy only when a PR is open.
// It also returns the PR that the action is about, or nil for AddCreate.
func DecideAdd(prs []github.PR, behindBy int) (AddAction, *github.PR) {
	if pr := OpenPR(prs); pr != nil {
		if behindBy > 0 {
			return AddRebuild, pr
		}
		return AddSkipOpen, pr
	}
	for i := range prs {
		if !prs[i].Merged {
			return AddSkipClosed, &prs[i]
		}
	}
	return AddCreate, nil
}

// NotesTemplate returns the first content of the notes file for a new skill.
func NotesTemplate(name string, src config.Source) (string, error) {
	var sb strings.Builder
	data := struct {
		Name   string
		Source config.Source
	}{name, src}
	if err := notesTemplate.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("build notes: %w", err)
	}
	return sb.String(), nil
}

// PRBody returns the body of the PR that adds a new skill.
func PRBody(name string, src config.Source, sha string, ignoreName string) (string, error) {
	var sb strings.Builder
	data := struct {
		Name       string
		Source     config.Source
		SHA        string
		IgnoreName string
	}{name, src, sha, ignoreName}
	if err := prBodyTemplate.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("build PR body: %w", err)
	}
	return sb.String(), nil
}

// skillEntry is the form of a skill that AddSkillEntry writes.
// It leaves out an empty fork entry.
type skillEntry struct {
	Type string       `yaml:"type"`
	Fork *config.Fork `yaml:"fork,omitempty"`
}

// AddSkillEntry adds a forked skill to the skills file at skillsYAMLPath.
// It keeps the leading comment of the file. The encoder sorts the keys.
func AddSkillEntry(skillsYAMLPath, name string, src config.Source) error {
	skills, err := config.LoadSkills(skillsYAMLPath)
	if err != nil {
		return err
	}
	if _, ok := skills[name]; ok {
		return fmt.Errorf("skill %s already exists in %s", name, skillsYAMLPath)
	}
	skills[name] = config.Skill{Type: "forked", Fork: &config.Fork{Source: src}}

	entries := make(map[string]skillEntry, len(skills))
	for n, s := range skills {
		entries[n] = skillEntry{Type: s.Type, Fork: s.Fork}
	}

	data, err := os.ReadFile(skillsYAMLPath)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString(leadingComment(string(data)))
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(entries); err != nil {
		return fmt.Errorf("encode %s: %w", skillsYAMLPath, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode %s: %w", skillsYAMLPath, err)
	}
	return os.WriteFile(skillsYAMLPath, buf.Bytes(), 0o644)
}

// leadingComment returns the comment lines at the start of a YAML file.
func leadingComment(data string) string {
	var sb strings.Builder
	for _, line := range strings.SplitAfter(data, "\n") {
		if !strings.HasPrefix(line, "#") {
			break
		}
		sb.WriteString(line)
	}
	return sb.String()
}
