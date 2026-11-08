// Package naming holds the rules for skill names and branch names.
package naming

import (
	"path"
	"regexp"
	"strings"
)

// AddPrefix starts the branch name for a skill that is being added.
const AddPrefix = "skill-add/"

// UpdatePrefix starts the branch name for a skill that is being updated.
const UpdatePrefix = "skill-update/"

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// LocalName returns the local skill name for a directory. The prefix
// is added to the last path element of dir.
func LocalName(prefix, dir string) string {
	return prefix + path.Base(strings.TrimSuffix(dir, "/"))
}

// AddBranch returns the branch name for adding the named skill.
func AddBranch(name string) string {
	return AddPrefix + name
}

// UpdateBranch returns the branch name for updating the named skill.
func UpdateBranch(name string) string {
	return UpdatePrefix + name
}

// SkillFromUpdateBranch returns the skill name from an update branch.
// It returns false when the branch is not an update branch for a skill.
func SkillFromUpdateBranch(branch string) (string, bool) {
	name, ok := strings.CutPrefix(branch, UpdatePrefix)
	if !ok || name == "" {
		return "", false
	}
	return name, true
}

// ValidName reports whether name is a valid skill name. A valid name
// uses only lowercase letters, digits and hyphens. It must not start
// with a hyphen.
func ValidName(name string) bool {
	return validName.MatchString(name)
}
