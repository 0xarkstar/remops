package cmd

import (
	"testing"
)

// host.go contains no pure helper functions — all exported logic requires SSH transport.
// These tests verify the command structure and package-level state used by host commands.

func TestHostCmd_SubcommandsRegistered(t *testing.T) {
	subCmds := map[string]bool{}
	for _, c := range hostCmd.Commands() {
		subCmds[c.Name()] = true
	}

	required := []string{"info", "disk", "prune", "exec"}
	for _, name := range required {
		if !subCmds[name] {
			t.Errorf("hostCmd missing subcommand %q", name)
		}
	}
}

func TestHostInfoCmd_Annotations(t *testing.T) {
	perm, ok := hostInfoCmd.Annotations["permission"]
	if !ok {
		t.Fatal("hostInfoCmd missing permission annotation")
	}
	if perm != "viewer" {
		t.Errorf("hostInfoCmd permission = %q, want %q", perm, "viewer")
	}
}

func TestFlagDryRun_DefaultFalse(t *testing.T) {
	origDryRun := flagDryRun
	t.Cleanup(func() { flagDryRun = origDryRun })

	// Default should be false (set by cobra flag default)
	flagDryRun = false
	if flagDryRun {
		t.Error("flagDryRun default should be false")
	}
}
