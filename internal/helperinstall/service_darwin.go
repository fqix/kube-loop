//go:build darwin

package helperinstall

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fqix/kube-loop/internal/helper"
)

func launchdPlistPath() string {
	return "/Library/LaunchDaemons/" + helper.ServiceLabel() + ".plist"
}

// legacySupervisorLabel is the LaunchDaemon that older desktop builds
// installed next to the helper (ADR 0023). Its update channel was removed;
// installs and uninstalls remove any leftover so it cannot keep replacing
// the helper binary underneath the current install.
func legacySupervisorLabel() string {
	if helper.IsDevBuild() {
		return "dev.fengqi.kubeloop.supervisor.dev"
	}
	return "dev.fengqi.kubeloop.supervisor"
}

func removeLegacySupervisor() {
	label := legacySupervisorLabel()
	plistPath := "/Library/LaunchDaemons/" + label + ".plist"
	//nolint:gosec // label is selected from fixed service identifiers.
	_ = exec.Command("launchctl", "bootout", "system/"+label).Run()
	_ = exec.Command("launchctl", "unload", "-w", plistPath).Run()
	_ = os.Remove(plistPath)
	_ = os.Remove("/Library/PrivilegedHelperTools/" + label)
	for _, name := range []string{"supervisor.json", "supervisor.lock", "supervisor.sock"} {
		_ = os.Remove(filepath.Join(helper.SystemStateDir(), name))
		_ = os.Remove(filepath.Join(filepath.Dir(helper.SocketPath()), name))
	}
}

func enableService(binaryPath string) error {
	removeLegacySupervisor()
	label := helper.ServiceLabel()
	plistPath := launchdPlistPath()
	logPath := helper.HelperLogPath()
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, label, binaryPath, logPath, logPath)
	//nolint:gosec // launchd requires its system configuration directory to be traversable.
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	//nolint:gosec // launchd property lists are intentionally system-readable and contain no secrets.
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}
	//nolint:gosec // label is selected from fixed helper service identifiers.
	_ = exec.Command("launchctl", "bootout", "system/"+label).Run()
	cmd := exec.Command("launchctl", "bootstrap", "system", plistPath)
	if _, err := cmd.CombinedOutput(); err != nil {
		// Older macOS fallback.
		cmd = exec.Command("launchctl", "load", "-w", plistPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("launchctl load helper: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	//nolint:gosec // label is selected from fixed helper service identifiers.
	_ = exec.Command("launchctl", "enable", "system/"+label).Run()
	//nolint:gosec // label is selected from fixed helper service identifiers.
	// RunAtLoad already starts the helper; kickstart must not kill it again.
	_ = exec.Command("launchctl", "kickstart", "system/"+label).Run()
	return nil
}

func disableService() error {
	removeLegacySupervisor()
	label := helper.ServiceLabel()
	plistPath := launchdPlistPath()
	//nolint:gosec // label is selected from fixed helper service identifiers.
	_ = exec.Command("launchctl", "bootout", "system/"+label).Run()
	_ = exec.Command("launchctl", "unload", "-w", plistPath).Run()
	_ = os.Remove(plistPath)
	return nil
}
