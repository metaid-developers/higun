package storage

import (
	"log"
	"time"
)

// ExampleBackupUsage demonstrates how to use the backup manager
func ExampleBackupUsage() {
	// Create backup manager
	// dataDir: database directory path
	// backupDir: backup file storage directory
	// shardCount: number of shards
	backupMgr := NewBackupManager("/path/to/data", "/path/to/backups", 2)

	// Start scheduled backup (automatic backup daily at 3 AM)
	if err := backupMgr.Start(); err != nil {
		log.Printf("Failed to start backup manager: %v", err)
		return
	}

	// Register storage instances (example)
	// backupMgr.RegisterStore("contract_ft_utxo", contractFtUtxoStore)
	// backupMgr.RegisterStore("address_ft_income", addressFtIncomeStore)
	// backupMgr.RegisterMetaStore(metaStore)

	// Manually perform a backup
	if err := backupMgr.ManualBackup(); err != nil {
		log.Printf("Manual backup failed: %v", err)
	}

	// Get backup status
	status := backupMgr.GetBackupStatus()
	log.Printf("Backup status: %+v", status)

	// Run for a while then stop
	time.Sleep(10 * time.Second)
	backupMgr.Stop()
}
