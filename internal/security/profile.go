package security

import (
	"fmt"

	"github.com/0xarkstar/remops/internal/config"
)

// CheckPermission verifies the profile has sufficient permission for the required level.
// Returns nil if allowed, error if denied.
func CheckPermission(profileLevel, requiredLevel config.PermissionLevel) error {
	if profileLevel >= requiredLevel {
		return nil
	}
	return fmt.Errorf("permission denied: operation requires %s level, current level is %s",
		requiredLevel, profileLevel)
}
