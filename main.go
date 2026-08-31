package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skd19/vido-tunnel/internal/auth"
	"github.com/skd19/vido-tunnel/internal/config"
	"github.com/skd19/vido-tunnel/internal/handlers"
	"github.com/skd19/vido-tunnel/internal/process"
	"github.com/skd19/vido-tunnel/internal/tray"
	"github.com/skd19/vido-tunnel/web"
)

func main() {
	cfg := config.LoadConfig()

	// Ensure the root directory exists or create it
	if err := os.MkdirAll(cfg.RootDir, 0755); err != nil {
		log.Printf("[WARNING] Could not create or access root dir: %v", err)
	}

	authMgr := auth.NewManager(cfg.SecretKey, cfg.SessionSecret)
	rateLimiter := auth.NewRateLimiter(5, 5*time.Minute)
	procMgr := process.NewManager(cfg.VidoveoPath, cfg.VidoveoPort)
	tunnelMgr := process.NewTunnelManager(cfg.TunnelName, cfg.CloudflaredPath)

	// Auto-start tunnel if configured (matching start-tunnel.ps1 behavior)
	if cfg.AutoStartTunnel {
		go func() {
			time.Sleep(1 * time.Second)
			status := tunnelMgr.GetStatus()
			if status.Installed && !status.Running {
				log.Printf("[TUNNEL] Auto-starting Cloudflare tunnel '%s'...", cfg.TunnelName)
				if err := tunnelMgr.Start(); err != nil {
					log.Printf("[TUNNEL] Auto-start failed: %v", err)
				} else {
					log.Printf("[TUNNEL] Cloudflare tunnel '%s' auto-started successfully.", cfg.TunnelName)
				}
			}
		}()
	}

	srv, err := handlers.NewServer(cfg, authMgr, rateLimiter, procMgr, tunnelMgr)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	mux := http.NewServeMux()

	// Static assets from embedded FS
	staticFS, err := fs.Sub(web.Content, "static")
	if err != nil {
		log.Fatalf("Failed to mount static assets: %v", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Public routes
	mux.HandleFunc("/login", srv.HandleLogin)
	mux.HandleFunc("/logout", srv.HandleLogout)

	// Protected routes
	mux.HandleFunc("/", handlers.RequireAuth(authMgr, srv.HandleBrowse))
	mux.HandleFunc("/browse", handlers.RequireAuth(authMgr, srv.HandleBrowse))
	mux.HandleFunc("/download", handlers.RequireAuth(authMgr, srv.HandleDownload))
	mux.HandleFunc("/download/zip", handlers.RequireAuth(authMgr, srv.HandleDownloadZip))
	mux.HandleFunc("/rename", handlers.RequireAuth(authMgr, srv.HandleRenameFolder))
	mux.HandleFunc("/control", handlers.RequireAuth(authMgr, srv.HandleControlPanel))
	mux.HandleFunc("/control/status", handlers.RequireAuth(authMgr, srv.HandleControlStatus))
	mux.HandleFunc("/control/start", handlers.RequireAuth(authMgr, srv.HandleControlStart))
	mux.HandleFunc("/control/stop", handlers.RequireAuth(authMgr, srv.HandleControlStop))
	mux.HandleFunc("/control/tunnel/start", handlers.RequireAuth(authMgr, srv.HandleControlTunnelStart))
	mux.HandleFunc("/control/tunnel/stop", handlers.RequireAuth(authMgr, srv.HandleControlTunnelStop))

	// Apply middleware stack
	handler := handlers.SecurityHeadersMiddleware(handlers.LoggingMiddleware(mux))

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Minute, // Long timeout for large file/ZIP streams
		IdleTimeout:  120 * time.Second,
	}

	// Server startup info banner
	fmt.Println("==========================================================")
	fmt.Println("           VIDO TUNNEL - SECURE WEB APPLICATION           ")
	fmt.Println("==========================================================")
	fmt.Printf(" Server URL       : http://localhost:%s\n", cfg.Port)
	fmt.Printf(" Scoped Root      : %s\n", cfg.RootDir)
	fmt.Printf(" Monitored Exe    : %s\n", cfg.VidoveoPath)
	fmt.Printf(" Monitored Port   : %d\n", cfg.VidoveoPort)
	fmt.Printf(" Cloudflare Tunnel: %s\n", cfg.TunnelName)
	fmt.Printf(" Auth Secret Key  : %s\n", cfg.SecretKey)
	fmt.Println("==========================================================")
	fmt.Println(" Ready for connections. Press Ctrl+C to shut down.")

	// Initialize System Tray App
	var trayApp *tray.App
	trayConfig := tray.Config{
		Title:        fmt.Sprintf("Vido Tunnel (Port %s)", cfg.Port),
		DashboardURL: fmt.Sprintf("http://localhost:%s", cfg.Port),
		ControlURL:   fmt.Sprintf("http://localhost:%s/control", cfg.Port),
		StoragePath:  cfg.RootDir,
		OnExit: func() {
			log.Println("[TRAY] Exit requested via system tray context menu.")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(ctx)
		},
	}
	trayApp = tray.New(trayConfig)

	// Graceful shutdown channel for terminal interrupts
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopChan
		log.Println("[SERVER] Interrupt signal received, shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
		trayApp.Stop()
	}()

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[SERVER] Server error: %v", err)
			trayApp.Stop()
		}
	}()

	// Run System Tray message loop on main OS thread
	if err := trayApp.Run(); err != nil {
		log.Printf("[TRAY] Tray loop error: %v", err)
	}

	log.Println("[SERVER] Vido Tunnel stopped cleanly.")
}
