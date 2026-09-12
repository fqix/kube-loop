//go:build ignore

// desktop-backend builds everything the Electron shell bundles as extra
// resources: the privileged helper (embedded into the backend), the patched
// sing-box, and the Go desktop backend itself. Output lands in build/bin so
// both `npm run dev` and electron-builder read from one place.
//
//	go run ./build/desktop-backend.go [goos/goarch]
//
// VITE_APP_VERSION sets the embedded application version (default "dev").
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	target := runtime.GOOS + "/" + runtime.GOARCH
	if len(os.Args) > 1 {
		target = os.Args[1]
	}
	goos, goarch, ok := strings.Cut(target, "/")
	if !ok || goos == "" || goarch == "" {
		fatalf("invalid target platform %q", target)
	}
	root, err := findRepositoryRoot()
	if err != nil {
		fatalf("%v", err)
	}
	version := os.Getenv("VITE_APP_VERSION")
	if version == "" {
		version = "dev"
	}

	run(root, "go", "run", "./build/helper-prebuild.go", target)
	run(root, "go", "run", "./build/stage-package-assets.go", target)

	output := filepath.Join(root, "build", "bin", BackendBinaryName(goos))
	fmt.Printf("==> Building desktop backend for %s (version=%s)\n", target, version)
	cmd := exec.Command(
		"go", "build", "-trimpath",
		"-ldflags", "-s -w -X main.version="+version,
		"-o", output, ".",
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("build desktop backend: %v", err)
	}
}

// BackendBinaryName is the file name the Electron shell spawns.
func BackendBinaryName(goos string) string {
	if goos == "windows" {
		return "kubeloop-backend.exe"
	}
	return "kubeloop-backend"
}

func run(root string, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("%s %s: %v", name, strings.Join(args, " "), err)
	}
}

func findRepositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above working directory")
		}
		dir = parent
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
