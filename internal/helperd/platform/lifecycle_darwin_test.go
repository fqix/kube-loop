//go:build darwin

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fengqi-dev/kube-loop/internal/protocol/sessionspec"
)

func testDarwinDNSMeta() sessionspec.DNSMeta {
	return sessionspec.DNSMeta{
		Listen: "127.0.0.1", Port: 1053, Ndots: 5,
		Domains: []string{"cluster.local", "svc"},
		Search:  []string{"default.svc.cluster.local", "svc.cluster.local", "cluster.local"},
	}
}

func TestDarwinDNSApply(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	resolverDir := filepath.Join(workDir, "resolver")
	scripts := []string{}
	d := darwinDNS{
		legacyResolverDir: resolverDir,
		run: func(name, input string, _ ...string) ([]byte, error) {
			if name == "/usr/sbin/networksetup" {
				t.Fatal("new DNS configuration must not change network service preferences")
			}
			if name == "/usr/sbin/scutil" {
				scripts = append(scripts, input)
			}
			return nil, nil
		},
	}
	dns := testDarwinDNSMeta()
	if err := d.apply(workDir, dns); err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 2 {
		t.Fatalf("got %d Dynamic Store operations", len(scripts))
	}
	for i, key := range []string{darwinDNSKey, darwinDNSSearchKey} {
		if !strings.Contains(scripts[i], "set "+key+"\n") {
			t.Fatalf("wrong key: %s", scripts[i])
		}
		// Let the real macOS parser validate our CFArray/CFNumber types without
		// publishing anything to the Dynamic Store or requiring root access.
		input := strings.Replace(scripts[i], "set "+key, "d.show", 1)
		output, err := runDarwinDNSCommand("/usr/sbin/scutil", input)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{
			"ServerAddresses : <array>", "0 : 127.0.0.1", "ServerPort : 1053", "Options : ndots:5",
			"SupplementalMatchDomains : <array>", "SupplementalMatchOrders : <array>", "0 : 100000",
		} {
			if !strings.Contains(string(output), value) {
				t.Fatalf("missing %q in %s", value, output)
			}
		}
		if i == 0 {
			if !strings.Contains(string(output), "SupplementalMatchDomainsNoSearch : 1") {
				t.Fatalf("routing suffixes must not enter the search list: %s", output)
			}
		} else {
			for _, value := range []string{"SupplementalMatchDomainsNoSearch : 0", "0 : default.svc.cluster.local", "1 : svc.cluster.local", "2 : cluster.local", "1 : 100001", "2 : 100002"} {
				if !strings.Contains(string(output), value) {
					t.Fatalf("missing %q in %s", value, output)
				}
			}
		}
	}
	if entries, err := os.ReadDir(workDir); err != nil || len(entries) != 0 {
		t.Fatalf("DNS apply wrote files: %v, %v", entries, err)
	}
	scripts = nil
	dns.Search = nil
	if err := d.apply(workDir, dns); err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 2 || scripts[1] != "remove "+darwinDNSSearchKey+"\nquit\n" {
		t.Fatalf("update must remove obsolete search state: %v", scripts)
	}
}

func TestDarwinDNSRejectsInvalidConfigurationBeforeChanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*sessionspec.DNSMeta)
	}{
		{name: "address command injection", change: func(d *sessionspec.DNSMeta) { d.Listen = "127.0.0.1\nquit" }},
		{name: "unspecified address", change: func(d *sessionspec.DNSMeta) { d.Listen = "0.0.0.0" }},
		{name: "port", change: func(d *sessionspec.DNSMeta) { d.Port = 65536 }},
		{name: "ndots", change: func(d *sessionspec.DNSMeta) { d.Ndots = -1 }},
		{name: "no match domains", change: func(d *sessionspec.DNSMeta) { d.Domains = nil }},
		{name: "default resolver", change: func(d *sessionspec.DNSMeta) { d.Domains = []string{""} }},
		{
			name:   "domain command injection",
			change: func(d *sessionspec.DNSMeta) { d.Domains = []string{"svc\nremove State:/Network/Global/DNS"} },
		},
		{name: "search command injection", change: func(d *sessionspec.DNSMeta) { d.Search = []string{"\"\nquit"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			oldPath := filepath.Join(dir, "cluster.local")
			original := []byte(dnsMarker + "\nnameserver 127.0.0.1\n")
			if err := os.WriteFile(oldPath, original, 0o600); err != nil {
				t.Fatal(err)
			}
			d := darwinDNS{legacyResolverDir: dir, run: func(string, string, ...string) ([]byte, error) {
				t.Fatal("invalid DNS configuration executed a command")
				return nil, nil
			}}
			dns := testDarwinDNSMeta()
			tt.change(&dns)
			if err := d.apply(t.TempDir(), dns); err == nil {
				t.Fatal("expected validation error")
			}
			if got, err := os.ReadFile(oldPath); err != nil || string(got) != string(original) {
				t.Fatalf("invalid config changed legacy state: %s, %v", got, err)
			}
		})
	}
}

func TestDarwinDNSRollsBackPartialApply(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, output string
		err          error
	}{
		{name: "scutil reports permission error with exit zero", output: "Permission denied\n"},
		{name: "command fails", err: errors.New("command failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scripts := []string{}
			d := darwinDNS{legacyResolverDir: t.TempDir(), run: func(name, input string, _ ...string) ([]byte, error) {
				if name != "/usr/sbin/scutil" {
					return nil, nil
				}
				scripts = append(scripts, input)
				if strings.Contains(input, "set "+darwinDNSSearchKey) {
					return []byte(tt.output), tt.err
				}
				return nil, nil
			}}
			if err := d.apply(t.TempDir(), testDarwinDNSMeta()); err == nil {
				t.Fatal("expected failure")
			}
			if len(scripts) != 4 || scripts[2] != "remove "+darwinDNSKey+"\nquit\n" ||
				scripts[3] != "remove "+darwinDNSSearchKey+"\nquit\n" {
				t.Fatalf("partial DNS state was not removed: %v", scripts)
			}
		})
	}
}

func TestDarwinDNSRestore(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, output string
		wantErr      bool
	}{
		{name: "removed"},
		{name: "already absent", output: "No such key\n"},
		{name: "permission error", output: "Permission denied\n", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			d := darwinDNS{legacyResolverDir: t.TempDir(), run: func(name, input string, _ ...string) ([]byte, error) {
				if name == "/usr/sbin/scutil" {
					calls++
					if input != "remove "+darwinDNSKey+"\nquit\n" && input != "remove "+darwinDNSSearchKey+"\nquit\n" {
						t.Fatalf("unexpected removal: %s", input)
					}
					return []byte(tt.output), nil
				}
				return nil, nil
			}}
			if err := d.restore(t.TempDir()); (err != nil) != tt.wantErr {
				t.Fatalf("restore: %v", err)
			}
			if calls != 2 {
				t.Fatalf("restore did not attempt both keys: %d", calls)
			}
		})
	}
}

func TestDarwinDNSLegacyCleanup(t *testing.T) {
	t.Parallel()
	dir, workDir := t.TempDir(), t.TempDir()
	owned := filepath.Join(dir, "cluster.local")
	other := filepath.Join(dir, "corp.example")
	if err := os.WriteFile(owned, []byte(dnsMarker+"\nnameserver 127.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("# User configuration\n"+dnsMarker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "symlink")
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(workDir, searchBackupFile)
	if err := os.WriteFile(backupPath, []byte(`{"Wi-Fi":["corp.example"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fail := true
	d := darwinDNS{legacyResolverDir: dir, run: func(name, _ string, args ...string) ([]byte, error) {
		if name != "/usr/sbin/networksetup" ||
			!reflect.DeepEqual(args, []string{"-setsearchdomains", "Wi-Fi", "corp.example"}) {
			t.Fatalf("unexpected legacy restoration: %s %v", name, args)
		}
		if fail {
			return nil, errors.New("networksetup failed")
		}
		return nil, nil
	}}
	if err := d.cleanupLegacy(workDir); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("failed restoration lost backup: %v", err)
	}
	fail = false
	if err := d.cleanupLegacy(workDir); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{owned, backupPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("not cleaned: %s (%v)", path, err)
		}
	}
	for _, path := range []string{other, link} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("unrelated resolver changed: %s (%v)", path, err)
		}
	}
	if err := d.cleanupLegacy(workDir); err != nil {
		t.Fatalf("cleanup not idempotent: %v", err)
	}
}
