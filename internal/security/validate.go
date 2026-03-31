package security

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	safeNameRe      = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	shellDangerRe   = regexp.MustCompile("[;|&`$(){}\\\\<>\n\r\x00]")
)

// ValidateHostName checks that a host name is safe (alphanumeric, hyphen, underscore, dot).
func ValidateHostName(name string) error {
	if name == "" {
		return fmt.Errorf("host name must not be empty")
	}
	if !safeNameRe.MatchString(name) {
		return fmt.Errorf("invalid host name %q: must contain only alphanumeric characters, hyphens, underscores, or dots", name)
	}
	return nil
}

// ValidateServiceName checks that a service name is safe.
func ValidateServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service name must not be empty")
	}
	if !safeNameRe.MatchString(name) {
		return fmt.Errorf("invalid service name %q: must contain only alphanumeric characters, hyphens, underscores, or dots", name)
	}
	return nil
}

// ValidateContainerName checks that a container name is safe.
func ValidateContainerName(name string) error {
	if name == "" {
		return fmt.Errorf("container name must not be empty")
	}
	if !safeNameRe.MatchString(name) {
		return fmt.Errorf("invalid container name %q: must contain only alphanumeric characters, hyphens, underscores, or dots", name)
	}
	return nil
}

// DetectShellInjection checks for dangerous shell characters in input.
// Rejects: ; | & ` $ ( ) { } < > \ newline
func DetectShellInjection(input string) error {
	if shellDangerRe.MatchString(input) {
		return fmt.Errorf("input contains potentially dangerous shell characters")
	}
	return nil
}

// ValidateRemotePath checks that a remote filesystem path is safe for shell use.
// Rejects paths containing shell metacharacters but allows slashes, dots, hyphens,
// underscores, tildes, and spaces (when properly quoted).
func ValidateRemotePath(path string) error {
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "~/") {
		return fmt.Errorf("path %q must be absolute or start with ~/", path)
	}
	if shellDangerRe.MatchString(path) {
		return fmt.Errorf("path %q contains potentially dangerous characters", path)
	}
	return nil
}
