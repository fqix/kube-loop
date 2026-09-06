// Command kubeloop-desktop-host runs the KubeLoop desktop application as a
// sidecar for the Electron shell.
//
// The shell spawns this process, reads the one-line JSON handshake from stdout,
// and then reaches every application binding over the loopback JSON-RPC
// endpoint — including the kubeloop:// callbacks it captures.
package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	embeddedassets "github.com/fengqi-dev/kube-loop/build/embedded"
	desktopapp "github.com/fengqi-dev/kube-loop/internal/app"
	"github.com/fengqi-dev/kube-loop/internal/apphost/rpc"
	internalLogging "github.com/fengqi-dev/kube-loop/internal/logging"
)

var version = "dev"

// shutdownTimeout bounds the ordered teardown of data planes and managers.
const shutdownTimeout = 15 * time.Second

func main() {
	// Logs go to stderr so stdout carries only the handshake line.
	jsonLogger := slog.New(internalLogging.WithContext(slog.NewJSONHandler(os.Stderr, nil)))
	slog.SetDefault(jsonLogger)
	log.SetOutput(slog.NewLogLogger(jsonLogger.Handler(), slog.LevelInfo).Writer())

	application := desktopapp.NewApp(version, embeddedassets.Files)

	server, err := rpc.New(application, jsonLogger)
	if err != nil {
		jsonLogger.Error("desktop shell bridge unavailable", "error", err)
		os.Exit(1)
	}
	desktopapp.SetHost(application, server)

	handshake, err := server.Listen()
	if err != nil {
		jsonLogger.Error("desktop shell bridge could not listen", "error", err)
		os.Exit(1)
	}
	if err := announce(handshake); err != nil {
		jsonLogger.Error("desktop shell handshake failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	desktopapp.StartupHandler(application)(ctx)

	served := make(chan error, 1)
	go func() { served <- server.Serve() }()

	select {
	case <-ctx.Done():
	case err := <-served:
		if err != nil {
			jsonLogger.Error("desktop shell bridge stopped", "error", err)
		}
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Close(shutdownContext); err != nil {
		jsonLogger.Warn("desktop shell bridge shutdown", "error", err)
	}
	desktopapp.ShutdownHandler(application)(shutdownContext)
}

// announce writes the handshake the shell waits for on stdout. The encoder
// writes straight to the file descriptor, and stdout is a pipe the shell reads,
// so there is nothing to flush afterwards.
func announce(handshake rpc.Handshake) error {
	return json.NewEncoder(os.Stdout).Encode(handshake)
}
