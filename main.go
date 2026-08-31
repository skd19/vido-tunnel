package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
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
		log.Printf("[WARNING] Could not create or access root dir '%s': %v", cfg.RootDir, err)
	} else {
		log.Printf("[STORAGE] Root folder verified: %s", cfg.RootDir)
	}

	authMgr := auth.NewManager(cfg.SecretKey, cfg.SessionSecret)
	rateLimiter := auth.NewRateLimiter(5, 5*time.Minute)
	procMgr := process.NewManager(cfg.VidoveoPath, cfg.VidoveoPort)
	tunnelMgr := process.NewTunnelManager(cfg.TunnelName, cfg.CloudflaredPath)

	// Auto-check and restart Cloudflare tunnel on startup
	go func() {
		time.Sleep(1 * time.Second)
		log.Printf("[TUNNEL] Auto-checking Cloudflare tunnel '%s' on startup...", cfg.TunnelName)
		if err := tunnelMgr.EnsureRunningOrRestart(); err != nil {
			log.Printf("[TUNNEL] Startup tunnel status: %v", err)
		} else {
			log.Printf("[TUNNEL] Cloudflare tunnel '%s' active and running.", cfg.TunnelName)
		}
	}()

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
	mux.HandleFunc("/control/tunnel/restart", handlers.RequireAuth(authMgr, srv.HandleControlTunnelRestart))
	mux.HandleFunc("/control/app/exit", handlers.RequireAuth(authMgr, srv.HandleControlAppExit))

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

	// Centralized Teardown Handler
	var teardownOnce sync.Once
	var trayApp *tray.App

	teardown := func(stopVidoveo, stopTunnel, shutdownPC bool) {
		teardownOnce.Do(func() {
			log.Println("[TEARDOWN] Starting complete process cleanup...")

			// 1. Terminate Vidoveo.exe if requested and running
			if stopVidoveo {
				if isRun, _ := procMgr.FindRunningProcess(); isRun {
					log.Println("[TEARDOWN] Stopping Vidoveo application...")
					_ = procMgr.Stop()
				}
			}

			// 2. Stop Cloudflare tunnel if requested
			if stopTunnel {
				log.Println("[TEARDOWN] Stopping Cloudflare tunnel...")
				_ = tunnelMgr.Stop()
			}

			// 3. Stop HTTP server
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(ctx)

			// 4. Remove System Tray Icon and exit loop
			if trayApp != nil {
				trayApp.Stop()
			}

			// 5. If PC shutdown requested, run shutdown.exe
			if shutdownPC {
				log.Println("[POWER] Initiating computer shutdown in 10 seconds...")
				cmd := exec.Command("shutdown.exe", "/s", "/t", "10", "/c", "Vido Tunnel closed - System shutdown initiated")
				cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
				_ = cmd.Run()
			}

			log.Println("[SERVER] Vido Tunnel stopped cleanly.")
			go func() {
				time.Sleep(200 * time.Millisecond)
				os.Exit(0)
			}()
		})
	}

	// Connect HTTP Control Panel Exit to teardown
	srv.SetOnExit(func(stopVidoveo, stopTunnel, shutdownPC bool) {
		teardown(stopVidoveo, stopTunnel, shutdownPC)
	})

	// Initialize System Tray App
	trayConfig := tray.Config{
		Title:        fmt.Sprintf("Vido Tunnel (Port %s)", cfg.Port),
		DashboardURL: fmt.Sprintf("http://localhost:%s", cfg.Port),
		ControlURL:   fmt.Sprintf("http://localhost:%s/control", cfg.Port),
		StoragePath:  cfg.RootDir,
		SecretKey:    cfg.SecretKey,
		OnStartVidoveo: func() {
			log.Println("[TRAY] Starting Vidoveo from tray menu...")
			_ = procMgr.Start()
		},
		OnStopVidoveo: func() {
			log.Println("[TRAY] Stopping Vidoveo from tray menu...")
			_ = procMgr.Stop()
		},
		OnRestartTunnel: func() {
			log.Println("[TRAY] Restarting Cloudflare tunnel from tray menu...")
			_ = tunnelMgr.Restart()
		},
		OnExit: func() {
			log.Println("[TRAY] Exit requested via system tray context menu.")
			teardown(true, true, false)
		},
	}
	trayApp = tray.New(trayConfig)

	// Graceful shutdown channel for terminal interrupts
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopChan
		log.Println("[SERVER] Interrupt signal received, shutting down...")
		teardown(true, true, false)
	}()

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[SERVER] Server error: %v", err)
			teardown(true, true, false)
		}
	}()

	// Run System Tray message loop on main OS thread
	if err := trayApp.Run(); err != nil {
		log.Printf("[TRAY] Tray loop error: %v", err)
	}

	log.Println("[SERVER] Vido Tunnel stopped cleanly.")
	os.Exit(0)
}
