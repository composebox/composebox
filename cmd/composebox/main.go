package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/composebox/composebox/internal/auth"
	"github.com/composebox/composebox/internal/config"
	"github.com/composebox/composebox/internal/server"
	"github.com/composebox/composebox/internal/stack"
)

var (
	// Version is injected at build time via -ldflags.
	Version = "dev"
	// GitCommit is injected at build time via -ldflags.
	GitCommit = "none"
	// BuildDate is injected at build time via -ldflags.
	BuildDate = "unknown"
)

func init() {
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				Version = info.Main.Version
			}
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" && GitCommit == "none" {
					GitCommit = setting.Value
					if len(GitCommit) > 7 {
						GitCommit = GitCommit[:7]
					}
				}
				if setting.Key == "vcs.time" && BuildDate == "unknown" {
					BuildDate = setting.Value
				}
			}
		}
	}
}

func main() {
	cfg := config.Load()

	if cfg.ShowVersion {
		fmt.Printf("composebox %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate)
		os.Exit(0)
	}

	log.Printf("[INFO] Starting composebox %s (%s)...", Version, GitCommit)
	log.Printf("[INFO] Stacks directory: %s", cfg.StacksDir)

	authMgr, err := auth.New(cfg.StacksDir, cfg.Password, cfg.NoAuth)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize auth: %v", err)
	}

	if cfg.NoAuth {
		log.Println("[WARN] Authentication is DISABLED (-no-auth)")
	} else if cfg.Password != "" {
		log.Println("[INFO] Authentication enabled via predefined password")
	} else if authMgr.IsSetupRequired() {
		log.Println("[INFO] First-run detected: Admin password setup required via web interface")
	} else {
		log.Println("[INFO] Authentication enabled")
	}

	scan := stack.NewScanner(cfg.StacksDir)
	doc := stack.NewDockerClient()
	srv := server.New(scan, doc, authMgr, Version)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0, // 0 for streaming SSE
		IdleTimeout:       120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] composebox listening on http://%s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server error: %v", err)
		}
	}()

	<-stop
	log.Println("[INFO] Shutting down composebox...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Forced shutdown: %v", err)
	}

	fmt.Println("composebox exited cleanly.")
}
