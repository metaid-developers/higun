package storage

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
)

// BackupManager database backup manager
type BackupManager struct {
	dataDir    string
	backupDir  string
	shardCount int
	isRunning  bool
	stopChan   chan struct{}

	// Storage instance references
	stores    map[string]*PebbleStore
	metaStore *MetaStore

	// Mapping from storage names to directory names
	storeDirs map[string]string
}

// NewBackupManager creates a new backup manager
func NewBackupManager(dataDir, backupDir string, shardCount int) *BackupManager {
	return &BackupManager{
		dataDir:    dataDir,
		backupDir:  backupDir,
		shardCount: shardCount,
		stopChan:   make(chan struct{}),
		stores:     make(map[string]*PebbleStore),
		storeDirs:  make(map[string]string),
	}
}

// Start starts scheduled backup
func (bm *BackupManager) Start() error {
	if bm.isRunning {
		return fmt.Errorf("backup manager is already running")
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(bm.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	bm.isRunning = true

	// Start scheduled backup goroutine
	go bm.scheduleBackup()

	log.Println("Database backup manager started, will perform backup daily at 3 AM")
	return nil
}

// RegisterStore registers a storage instance
func (bm *BackupManager) RegisterStore(name string, store *PebbleStore) {
	bm.stores[name] = store
	// Map name to corresponding directory name
	bm.storeDirs[name] = name
	log.Printf("Registered storage instance: %s -> %s", name, name)
}

// RegisterMetaStore registers a metadata storage instance
func (bm *BackupManager) RegisterMetaStore(metaStore *MetaStore) {
	bm.metaStore = metaStore
	log.Println("Registered metadata storage instance")
}

// Stop stops scheduled backup
func (bm *BackupManager) Stop() {
	if !bm.isRunning {
		return
	}

	close(bm.stopChan)
	bm.isRunning = false
	log.Println("Database backup manager stopped")
}

// scheduleBackup scheduled backup scheduler
func (bm *BackupManager) scheduleBackup() {
	for {
		// Calculate next 3 AM time
		now := time.Now()
		nextBackup := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())

		// If today's 3 AM has passed, set to tomorrow's 3 AM
		if now.After(nextBackup) {
			nextBackup = nextBackup.Add(24 * time.Hour)
		}

		// Wait until next backup time
		waitDuration := nextBackup.Sub(now)
		log.Printf("Next backup time: %s (waiting %v)", nextBackup.Format("2006-01-02 15:04:05"), waitDuration)

		select {
		case <-time.After(waitDuration):
			// Perform backup
			bm.performBackup()
		case <-bm.stopChan:
			// Received stop signal
			return
		}
	}
}

// performBackup performs backup operation
func (bm *BackupManager) performBackup() {
	log.Println("Starting database backup...")
	startTime := time.Now()

	// Generate backup directory name (with timestamp)
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupDirName := fmt.Sprintf("utxo_indexer_backup_%s", timestamp)
	backupDirPath := filepath.Join(bm.backupDir, backupDirName)

	// Create backup directory
	if err := os.MkdirAll(backupDirPath, 0755); err != nil {
		log.Printf("Failed to create backup directory: %v", err)
		return
	}

	successCount := 0
	totalCount := len(bm.stores) + 1 // +1 for metaStore

	// Backup all registered storage instances
	for name, store := range bm.stores {
		if err := bm.backupPebbleStore(name, store, backupDirPath); err != nil {
			log.Printf("Failed to backup storage %s: %v", name, err)
		} else {
			successCount++
			log.Printf("Successfully backed up storage: %s", name)
		}
	}

	// Backup metadata storage
	if bm.metaStore != nil {
		if err := bm.backupMetaStore(backupDirPath); err != nil {
			log.Printf("Failed to backup metadata storage: %v", err)
		} else {
			successCount++
			log.Printf("Successfully backed up metadata storage")
		}
	}

	// Clean old backup directories (keep last 7 days)
	bm.cleanOldBackups()

	duration := time.Since(startTime)
	log.Printf("Database backup completed: %d/%d storages backed up successfully, duration: %v, backup directory: %s",
		successCount, totalCount, duration, backupDirPath)
}

// backupPebbleStore backs up a Pebble storage instance
func (bm *BackupManager) backupPebbleStore(name string, store *PebbleStore, backupDirPath string) error {
	// Get corresponding directory name
	dirName, exists := bm.storeDirs[name]
	if !exists {
		dirName = name // If no mapping, use name as directory name
	}

	// Create backup database directory, maintaining same directory structure as original database
	backupDBDir := filepath.Join(backupDirPath, dirName)

	// Create backup database for each shard
	shards := store.GetShards()
	backupShards := make([]*pebble.DB, len(shards))

	// Create backup databases
	for i := range shards {
		shardBackupDir := filepath.Join(backupDBDir, fmt.Sprintf("shard_%d", i))
		if err := os.MkdirAll(shardBackupDir, 0755); err != nil {
			return fmt.Errorf("failed to create backup database directory: %w", err)
		}

		// Create backup database
		backupDB, err := pebble.Open(shardBackupDir, &pebble.Options{Logger: noopLogger})
		if err != nil {
			return fmt.Errorf("failed to create backup database: %w", err)
		}
		backupShards[i] = backupDB
		defer backupDB.Close()
	}

	// Iterate through all key-value pairs in original database and batch write to backup database
	for i, db := range shards {
		backupDB := backupShards[i]

		// Create iterator
		iter, err := db.NewIter(nil)
		if err != nil {
			return fmt.Errorf("failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Batch write
		batch := backupDB.NewBatch()
		count := 0
		const batchSize = 1000 // Commit every 1000 records

		for iter.First(); iter.Valid(); iter.Next() {
			key := iter.Key()
			value := iter.Value()

			// Copy key and value to batch
			keyCopy := make([]byte, len(key))
			valueCopy := make([]byte, len(value))
			copy(keyCopy, key)
			copy(valueCopy, value)

			if err := batch.Set(keyCopy, valueCopy, nil); err != nil {
				return fmt.Errorf("failed to write backup data: %w", err)
			}

			count++
			if count >= batchSize {
				// Commit batch
				if err := batch.Commit(pebble.Sync); err != nil {
					return fmt.Errorf("failed to commit backup batch: %w", err)
				}
				batch = backupDB.NewBatch()
				count = 0
			}
		}

		// Commit final batch
		if count > 0 {
			if err := batch.Commit(pebble.Sync); err != nil {
				return fmt.Errorf("failed to commit final backup batch: %w", err)
			}
		}
	}

	// Backup completed, return success
	return nil
}

// backupMetaStore backs up metadata storage
func (bm *BackupManager) backupMetaStore(backupDirPath string) error {
	// Create backup database directory, using same directory name as original database
	backupDBDir := filepath.Join(backupDirPath, "meta")

	// Create backup database
	backupDB, err := pebble.Open(backupDBDir, &pebble.Options{Logger: noopLogger})
	if err != nil {
		return fmt.Errorf("failed to create metadata backup database: %w", err)
	}
	defer backupDB.Close()

	// Create iterator
	iter, err := bm.metaStore.db.NewIter(nil)
	if err != nil {
		return fmt.Errorf("failed to create metadata iterator: %w", err)
	}
	defer iter.Close()

	// Batch write
	batch := backupDB.NewBatch()
	count := 0
	const batchSize = 1000 // Commit every 1000 records

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// Copy key and value to batch
		keyCopy := make([]byte, len(key))
		valueCopy := make([]byte, len(value))
		copy(keyCopy, key)
		copy(valueCopy, value)

		if err := batch.Set(keyCopy, valueCopy, nil); err != nil {
			return fmt.Errorf("failed to write metadata backup: %w", err)
		}

		count++
		if count >= batchSize {
			// Commit batch
			if err := batch.Commit(pebble.Sync); err != nil {
				return fmt.Errorf("failed to commit metadata backup batch: %w", err)
			}
			batch = backupDB.NewBatch()
			count = 0
		}
	}

	// Commit final batch
	if count > 0 {
		if err := batch.Commit(pebble.Sync); err != nil {
			return fmt.Errorf("failed to commit final metadata backup batch: %w", err)
		}
	}

	// Backup completed, return success
	return nil
}

// cleanOldBackups cleans old backup directories (keep last 7 days)
func (bm *BackupManager) cleanOldBackups() {
	entries, err := os.ReadDir(bm.backupDir)
	if err != nil {
		log.Printf("Failed to read backup directory: %v", err)
		return
	}

	cutoffTime := time.Now().AddDate(0, 0, -7) // 7 days ago
	deletedCount := 0

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "utxo_indexer_backup_") {
			continue
		}

		dirPath := filepath.Join(bm.backupDir, entry.Name())
		fileInfo, err := entry.Info()
		if err != nil {
			log.Printf("Failed to get directory info: %v", err)
			continue
		}

		// If directory is older than 7 days, delete it
		if fileInfo.ModTime().Before(cutoffTime) {
			if err := os.RemoveAll(dirPath); err != nil {
				log.Printf("Failed to delete old backup directory: %v", err)
			} else {
				deletedCount++
				log.Printf("Deleted old backup directory: %s", entry.Name())
			}
		}
	}

	if deletedCount > 0 {
		log.Printf("Cleanup completed, deleted %d old backup directories", deletedCount)
	}
}

// ManualBackup manually performs backup
func (bm *BackupManager) ManualBackup() error {
	log.Println("Starting manual backup...")
	bm.performBackup()
	return nil
}

// GetBackupStatus gets backup status
func (bm *BackupManager) GetBackupStatus() map[string]interface{} {
	status := map[string]interface{}{
		"is_running": bm.isRunning,
		"data_dir":   bm.dataDir,
		"backup_dir": bm.backupDir,
	}

	// 获取备份目录列表
	entries, err := os.ReadDir(bm.backupDir)
	if err == nil {
		var backupDirs []string
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "utxo_indexer_backup_") {
				backupDirs = append(backupDirs, entry.Name())
			}
		}
		status["backup_dirs"] = backupDirs
		status["backup_count"] = len(backupDirs)
	}

	return status
}
