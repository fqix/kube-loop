// Command kube-loop is the desktop backend. The Electron shell in src/main
// launches it as a child process and speaks JSON-RPC
// over stdin/stdout; the process has no window or tray of its own.
package main

import (
	"context"
	"embed"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	desktopapp "github.com/fqix/kube-loop/internal/app"
	"github.com/fqix/kube-loop/internal/desktopipc"
	internalLogging "github.com/fqix/kube-loop/internal/logging"
)

// The README keeps the pattern valid for ordinary source builds. Release and
// IDE builds generate the platform helper in this directory before compiling
// the desktop backend.
//
//go:embed build/embedded/*
var embeddedHelperFiles embed.FS

var version = "dev"

func main() {
	// stdout carries the protocol; everything that logs goes to stderr so a
	// stray print can never corrupt a frame.
	protocolOut := os.Stdout
	os.Stdout = os.Stderr
	jsonLogger := slog.New(internalLogging.WithContext(slog.NewJSONHandler(os.Stderr, nil)))
	slog.SetDefault(jsonLogger)
	log.SetOutput(slog.NewLogLogger(jsonLogger.Handler(), slog.LevelInfo).Writer())

	if err := run(protocolOut, jsonLogger); err != nil {
		log.Fatal(err)
	}
}

func run(protocolOut *os.File, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := desktopapp.NewApp(version, embeddedHelperFiles)
	dispatcher := desktopipc.NewDispatcher()
	if err := dispatcher.Bind(app, "SetHost", "Close"); err != nil {
		return err
	}
	shutdownOnce := make(chan struct{}, 1)
	shutdown := func(shutdownContext context.Context) {
		select {
		case shutdownOnce <- struct{}{}:
			desktopapp.ShutdownHandler(app)(shutdownContext)
		default:
		}
	}
	server := desktopipc.NewServer(
		dispatcher, os.Stdin, protocolOut,
		desktopipc.WithLogger(logger),
		desktopipc.WithShutdownHandler(shutdown),
	)
	app.SetHost(server)
	desktopapp.StartupHandler(app)(ctx)

	err := server.Serve(ctx)
	// The shell may vanish without a shutdown request (crash, SIGKILL); run
	// the same cleanup so TUN sessions and intercepts are restored.
	shutdown(context.WithoutCancel(ctx))
	if err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
