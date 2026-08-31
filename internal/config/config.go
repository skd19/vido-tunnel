package config

import (
	"crypto/rand"
	"flag"
	"os"
	"path/filepath"
	"strconv"
)

// Config holds all server and security settings
type Config struct {
	RootDir       string
	SecretKey     string
	Port          string
	VidoveoPath   string
	VidoveoPort   int
	SessionSecret []byte
}

// LoadConfig parses flags and environment variables with sensible defaults
func LoadConfig() *Config {
	defaultRoot := `C:\Users\sk\Videos\Vidoveo`
	if envRoot := os.Getenv("VIDO_ROOT_DIR"); envRoot != "" {
		defaultRoot = envRoot
	}

	defaultKey := "vido-secret-key-2026"
	if envKey := os.Getenv("VIDO_SECRET_KEY"); envKey != "" {
		defaultKey = envKey
	}

	defaultPort := "8080"
	if envPort := os.Getenv("VIDO_PORT"); envPort != "" {
		defaultPort = envPort
	}

	defaultVidoveoPath := `C:\Vidoveo\Vidoveo.exe`
	if envPath := os.Getenv("VIDOVEO_PATH"); envPath != "" {
		defaultVidoveoPath = envPath
	}

	defaultVidoveoPort := 7788
	if envVPort := os.Getenv("VIDOVEO_PORT"); envVPort != "" {
		if p, err := strconv.Atoi(envVPort); err == nil {
			defaultVidoveoPort = p
		}
	}

	rootDir := flag.String("root", defaultRoot, "Root directory path to browse and manage")
	secretKey := flag.String("key", defaultKey, "Secret key for authentication")
	port := flag.String("port", defaultPort, "HTTP server listening port")
	vidoveoPath := flag.String("vidoveo-path", defaultVidoveoPath, "Path to Vidoveo.exe executable")
	vidoveoPort := flag.Int("vidoveo-port", defaultVidoveoPort, "Port monitored for Vidoveo (e.g. 7788)")

	flag.Parse()

	// Clean root path
	cleanRoot := filepath.Clean(*rootDir)

	// Ensure session secret
	sessionSecret := make([]byte, 32)
	if _, err := rand.Read(sessionSecret); err != nil {
		sessionSecret = []byte("vido-tunnel-fallback-32-byte-secret!")
	}

	return &Config{
		RootDir:       cleanRoot,
		SecretKey:     *secretKey,
		Port:          *port,
		VidoveoPath:   *vidoveoPath,
		VidoveoPort:   *vidoveoPort,
		SessionSecret: sessionSecret,
	}
}
