package gitx

import "strings"

// ParseStatusPorcelain returns the paths in the output of
// "git status --porcelain=v1 -z". For a rename or a copy, it returns the new path.
func ParseStatusPorcelain(out string) []string {
	var paths []string
	records := strings.Split(out, "\x00")
	for i := 0; i < len(records); i++ {
		rec := records[i]
		if len(rec) < 4 {
			continue
		}
		paths = append(paths, rec[3:])
		// A rename record is followed by a record that holds the old path.
		if rec[0] == 'R' || rec[0] == 'C' || rec[1] == 'R' || rec[1] == 'C' {
			i++
		}
	}
	return paths
}
