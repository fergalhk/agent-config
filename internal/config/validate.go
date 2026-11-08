package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/fergalhk/agent-config/internal/naming"
)

// Validate checks the skills map against the files under root.
// It returns one message for each finding. The result is empty when all is valid.
func Validate(root string, skills Skills) []string {
	var findings []string

	dirs := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(root, "skills"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return []string{fmt.Sprintf("cannot read skills directory: %v", err)}
	}
	for _, e := range entries {
		if !e.IsDir() {
			findings = append(findings, fmt.Sprintf("skills/%s is a file, but skills/ must contain only skill directories", e.Name()))
			continue
		}
		dirs[e.Name()] = true
		if _, ok := skills[e.Name()]; !ok {
			findings = append(findings, fmt.Sprintf("skills/%s has no entry in %s", e.Name(), SkillsPath))
		}
	}

	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		skill := skills[name]
		if !naming.ValidName(name) {
			findings = append(findings, fmt.Sprintf("%s lists skill %q, but the name is not valid. Use only lowercase letters, digits and hyphens", SkillsPath, name))
		}
		if !dirs[name] {
			findings = append(findings, fmt.Sprintf("%s lists skill %s, but skills/%s does not exist", SkillsPath, name, name))
		}
		_, shaErr := os.Stat(filepath.Join(root, ShaPath(name)))
		hasSha := shaErr == nil

		switch skill.Type {
		case typeForked:
			if skill.Fork == nil {
				findings = append(findings, fmt.Sprintf("skill %s has type forked, so it needs a fork entry", name))
			}
			if !hasSha {
				findings = append(findings, fmt.Sprintf("skill %s has type forked, so it needs %s", name, ShaPath(name)))
			}
		case typeFirstParty:
			if skill.Fork != nil {
				findings = append(findings, fmt.Sprintf("skill %s has type first_party, so it must not have a fork entry", name))
			}
			if hasSha {
				findings = append(findings, fmt.Sprintf("skill %s has type first_party, so %s must not exist", name, ShaPath(name)))
			}
		default:
			findings = append(findings, fmt.Sprintf("skill %s has unknown type %q", name, skill.Type))
		}
	}
	return findings
}
