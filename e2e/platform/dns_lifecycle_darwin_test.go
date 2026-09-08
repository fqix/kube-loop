//go:build e2e && darwin

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func assertPlatformE2EDNS(t *testing.T, want bool) {
	t.Helper()
	for _, key := range []string{
		"State:/Network/Service/io.kubeloop/DNS",
		"State:/Network/Service/io.kubeloop.search/DNS",
	} {
		cmd := exec.Command("/usr/sbin/scutil")
		cmd.Stdin = strings.NewReader("show " + key + "\nquit\n")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("read DNS Dynamic Store key %s: %v: %s", key, err, output)
		}
		if want {
			for _, value := range []string{platformE2EDomain, "0 : 127.0.0.1", "ServerPort : 1053"} {
				if !strings.Contains(string(output), value) {
					t.Fatalf("missing %q in %s: %s", value, key, output)
				}
			}
		} else if strings.TrimSpace(string(output)) != "No such key" {
			t.Fatalf("DNS Dynamic Store key %s remains: %s", key, output)
		}
	}
	if _, err := os.Stat(filepath.Join("/etc/resolver", platformE2EDomain)); !os.IsNotExist(err) {
		t.Fatalf("unexpected resolver file (stat error=%v)", err)
	}
	// configd publishes the effective resolver list asynchronously.
	deadline := time.Now().Add(5 * time.Second)
	for {
		output, err := exec.Command("/usr/sbin/scutil", "--dns").CombinedOutput()
		if err != nil {
			t.Fatalf("read effective DNS: %v: %s", err, output)
		}
		configured := false
		for block := range strings.SplitSeq(string(output), "\n\n") {
			fields := strings.Join(strings.Fields(block), " ")
			if strings.Contains(fields, "domain : "+platformE2EDomain) &&
				strings.Contains(fields, "nameserver[0] : 127.0.0.1") &&
				strings.Contains(fields, "port : 1053") && strings.Contains(fields, "Supplemental") {
				configured = true
			}
		}
		if want && configured || !want && !strings.Contains(string(output), platformE2EDomain) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("effective macOS DNS configured=%v want %v: %s", !want, want, output)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
