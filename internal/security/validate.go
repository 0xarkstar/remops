package security

import (
	"fmt"
	"regexp"
)

var (
	safeNameRe      = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	shellDangerRe   = regexp.MustCompile("[;|&`$(){}\\\\<>\n\r]")
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
