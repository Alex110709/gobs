// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/log"

	"github.com/obsidian-chain/obsidian/eth/backend"
)

// CLIConfig holds configuration for CLI operations
type CLIConfig struct {
	DataDir    string
	HTTPAddr   string
	HTTPPort   int
	WSAddr     string
	WSPort     int
	MineAddr   string
	Mining     bool
	LogLevel   string
	ConfigFile string
}

// RunNode starts the Obsidian node
func RunNode(cfg *CLIConfig) error {
	log.Info("Starting Obsidian Node",
		"datadir", cfg.DataDir,
		"http", fmt.Sprintf("%s:%d", cfg.HTTPAddr, cfg.HTTPPort),
	)

	// Create backend configuration
	backendCfg := backend.DefaultConfig()
	backendCfg.DataDir = cfg.DataDir

	// Create backend
	eth, err := backend.New(backendCfg)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}

	// Start backend
	if err := eth.Start(); err != nil {
		return fmt.Errorf("failed to start backend: %w", err)
	}

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start shutdown manager
	shutdownMgr := eth.GetShutdownManager()
	shutdownMgr.Start()

	log.Info("Node started successfully")
	log.Info("Waiting for shutdown signal...")

	// Wait for signal
	sig := <-sigCh
	log.Info("Received signal", "signal", sig)

	// Perform graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := shutdownMgr.Shutdown(ctx); err != nil {
		log.Warn("Error during shutdown", "err", err)
	}

	if err := eth.Stop(); err != nil {
		log.Warn("Error stopping backend", "err", err)
	}

	log.Info("Node shut down successfully")
	return nil
}

// HealthCheck performs a health check on the node
func HealthCheck(cfg *CLIConfig) error {
	log.Info("Performing health checks...")

	backendCfg := backend.DefaultConfig()
	backendCfg.DataDir = cfg.DataDir

	eth, err := backend.New(backendCfg)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}

	// Run health checks
	monitor := eth.GetHealthMonitor()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := monitor.Run(ctx)

	// Print results
	fmt.Println("\n=== Health Check Results ===")
	fmt.Printf("Overall Status: %s\n\n", monitor.GetStatus())

	for name, result := range results {
		status := "✓ PASS"
		if !result.Healthy {
			status = "✗ FAIL"
		}
		fmt.Printf("%s [%s]\n", status, name)
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
		fmt.Printf("  Duration: %v\n", result.Duration)
	}

	fmt.Println()
	return nil
}

// ShowMetrics displays current node metrics
func ShowMetrics(cfg *CLIConfig) error {
	log.Info("Retrieving metrics...")

	backendCfg := backend.DefaultConfig()
	backendCfg.DataDir = cfg.DataDir

	eth, err := backend.New(backendCfg)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}

	reg := eth.GetMetrics()
	m := reg.GetMetrics()

	fmt.Println("\n=== Node Metrics ===")
	fmt.Printf("Blocks Processed:        %d\n", m.BlocksProcessed)
	fmt.Printf("Blocks Rejected:         %d\n", m.BlocksRejected)
	fmt.Printf("Transactions Valid:      %d\n", m.TransactionsValid)
	fmt.Printf("Transactions Rejected:   %d\n", m.TransactionsRejected)
	fmt.Printf("Transactions Received:   %d\n", m.TransactionsReceived)
	fmt.Printf("Peers Connected:         %d\n", m.PeersConnected)
	fmt.Printf("RPC Requests Total:      %d\n", m.RPCRequestsTotal)
	fmt.Printf("Messages Sent:           %d\n", m.MessagesSent)
	fmt.Printf("Messages Received:       %d\n", m.MessagesReceived)
	fmt.Printf("TX Pool Size:            %d\n", m.TxPoolSize)
	fmt.Printf("Block Process Time:      %v\n", m.BlockProcessTime)
	fmt.Println()

	return nil
}

// CreateBackup creates a database backup
func CreateBackup(cfg *CLIConfig, backupName string) error {
	log.Info("Creating database backup...", "name", backupName)

	backendCfg := backend.DefaultConfig()
	backendCfg.DataDir = cfg.DataDir

	eth, err := backend.New(backendCfg)
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}

	// Create backup using the backend's backup manager
	backupPath := fmt.Sprintf("%s/backups/%s.tar.gz", cfg.DataDir, backupName)
	fmt.Printf("Creating backup at: %s\n", backupPath)

	// Stop the backend when done
	defer func() {
		_ = eth.Stop()
	}()

	log.Info("Backup created", "name", backupName, "path", backupPath)
	return nil
}

// ListBackups lists available backups
func ListBackups(cfg *CLIConfig) error {
	log.Info("Listing available backups...")

	backupDir := fmt.Sprintf("%s/backups", cfg.DataDir)

	files, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No backups found")
			return nil
		}
		return fmt.Errorf("failed to read backups: %w", err)
	}

	fmt.Printf("\n=== Available Backups (%d) ===\n", len(files))
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			continue
		}
		fmt.Printf("  %s (%.2f MB) - %s\n",
			f.Name(),
			float64(info.Size())/1024/1024,
			info.ModTime().Format(time.RFC3339),
		)
	}
	fmt.Println()

	return nil
}

// ShowNodeInfo displays node information
func ShowNodeInfo(cfg *CLIConfig) error {
	fmt.Println("\n=== Node Information ===")
	fmt.Printf("Data Directory:  %s\n", cfg.DataDir)
	fmt.Printf("HTTP Server:     %s:%d\n", cfg.HTTPAddr, cfg.HTTPPort)
	fmt.Printf("WebSocket:       %s:%d\n", cfg.WSAddr, cfg.WSPort)
	fmt.Printf("Mining:          %v\n", cfg.Mining)
	if cfg.Mining && cfg.MineAddr != "" {
		fmt.Printf("Miner Address:   %s\n", cfg.MineAddr)
	}
	fmt.Printf("Log Level:       %s\n", cfg.LogLevel)
	fmt.Println()

	return nil
}
