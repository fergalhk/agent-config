package naming

import "testing"

func TestLocalName(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		dir    string
		want   string
	}{
		{"nested path", "sp-", "skills/brainstorming", "sp-brainstorming"},
		{"trailing slash", "sp-", "brainstorming/", "sp-brainstorming"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LocalName(tt.prefix, tt.dir); got != tt.want {
				t.Errorf("LocalName(%q, %q) = %q, want %q", tt.prefix, tt.dir, got, tt.want)
			}
		})
	}
}

func TestSkillFromUpdateBranch(t *testing.T) {
	tests := []struct {
		branch   string
		wantName string
		wantOK   bool
	}{
		{"skill-update/sp-x", "sp-x", true},
		{"skill-update/", "", false},
		{"skill-add/sp-x", "", false},
		{"main", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			name, ok := SkillFromUpdateBranch(tt.branch)
			if name != tt.wantName || ok != tt.wantOK {
				t.Errorf("SkillFromUpdateBranch(%q) = (%q, %v), want (%q, %v)",
					tt.branch, name, ok, tt.wantName, tt.wantOK)
			}
		})
	}
}

func TestValidName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"sp-brainstorming", true},
		{"Sp", false},
		{"-x", false},
		{"", false},
		{"a/b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidName(tt.name); got != tt.want {
				t.Errorf("ValidName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
