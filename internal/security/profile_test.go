package security

import (
	"testing"

	"github.com/0xarkstar/remops/internal/config"
)

func TestCheckPermission(t *testing.T) {
	tests := []struct {
		name     string
		profile  config.PermissionLevel
		required config.PermissionLevel
		wantErr  bool
	}{
		{"admin satisfies admin", config.LevelAdmin, config.LevelAdmin, false},
		{"admin satisfies operator", config.LevelAdmin, config.LevelOperator, false},
		{"admin satisfies viewer", config.LevelAdmin, config.LevelViewer, false},
		{"operator satisfies operator", config.LevelOperator, config.LevelOperator, false},
		{"operator satisfies viewer", config.LevelOperator, config.LevelViewer, false},
		{"operator denied admin", config.LevelOperator, config.LevelAdmin, true},
		{"viewer satisfies viewer", config.LevelViewer, config.LevelViewer, false},
		{"viewer denied operator", config.LevelViewer, config.LevelOperator, true},
		{"viewer denied admin", config.LevelViewer, config.LevelAdmin, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckPermission(tc.profile, tc.required)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
