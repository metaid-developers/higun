package syslogs

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type IndexerLog struct {
	Height             int    `json:"height"`
	BlockHash          string `json:"block_hash"`
	ExpectedInTxCount  int    `json:"in_tx_count"`
	ExpectedOutTxCount int    `json:"out_tx_count"`
	ActualInTxCount    int    `json:"actual_in_tx_count"`
	ActualOutTxCount   int    `json:"actual_out_tx_count"`
	CompletionTime     int64  `json:"completion_time"`
	BlockTime          int64  `json:"block_time"`
	TxNum              int64  `json:"tx_num"`
	AddressNum         int64  `json:"address_num"`
	NewAddressNum      int64  `json:"new_address_num"`
	Reorg              int    `json:"reorg"`
}
type ErrLog struct {
	ErrType      string `json:"err_type"`
	Height       int    `json:"height"`
	BlockHash    string `json:"block_hash"`
	Timestamp    int64  `json:"timestamp"`
	ErrorMessage string `json:"error_message"`
}
type ReorgLog struct {
	Height       int    `json:"height"`
	EndHeight    int    `json:"end_height"`
	BlockHash    string `json:"block_hash"`
	NewBlockHash string `json:"new_block_hash"`
	ReorgSize    int    `json:"reorg_size"`
	Timestamp    int64  `json:"timestamp"`
	Status       int    `json:"status"`
}

var (
	db *sql.DB
)

func InitIndexerLogDB(dbPath string) error {
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err = db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Set SQLite to WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("failed to set WAL mode: %w", err)
	}

	if err = createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

func createTables() error {
	indexerLogTable := `CREATE TABLE IF NOT EXISTS IndexerLog (
		ID INTEGER PRIMARY KEY AUTOINCREMENT,
		Height INTEGER,
		BlockHash TEXT,
		ExpectedInTxCount INTEGER,
		ExpectedOutTxCount INTEGER,
		ActualInTxCount INTEGER,
		ActualOutTxCount INTEGER,
		TxNum INTEGER,
		AddressNum INTEGER,
		NewAddressNum INTEGER,
		CompletionTime INTEGER,
		BlockTime INTEGER,
		Reorg INTEGER
	)`

	errLogTable := `CREATE TABLE IF NOT EXISTS ErrLog (
		ID INTEGER PRIMARY KEY AUTOINCREMENT,
		ErrType TEXT,
		Height INTEGER,
		BlockHash TEXT,
		Timestamp INTEGER,
		ErrorMessage TEXT
	)`

	reorgLogTable := `CREATE TABLE IF NOT EXISTS ReorgLog (
		ID INTEGER PRIMARY KEY AUTOINCREMENT,
		Height INTEGER,
		EndHeight INTEGER,
		BlockHash TEXT,
		NewBlockHash TEXT,
		ReorgSize INTEGER,
		Timestamp INTEGER,
		Status INTEGER
	)`

	if _, err := db.Exec(reorgLogTable); err != nil {
		return fmt.Errorf("failed to create ReorgLog table: %w", err)
	}

	if _, err := db.Exec(indexerLogTable); err != nil {
		return fmt.Errorf("failed to create IndexerLog table: %w", err)
	}

	if _, err := db.Exec(errLogTable); err != nil {
		return fmt.Errorf("failed to create ErrLog table: %w", err)
	}
	// 新增：为 IndexerLog.Height 创建索引
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_indexerlog_height ON IndexerLog(Height);`); err != nil {
		return fmt.Errorf("failed to create index on IndexerLog.Height: %w", err)
	}
	return nil
}

func InsertIndexerLog(log IndexerLog) error {
	query := `INSERT INTO IndexerLog (Height, BlockHash, ExpectedInTxCount, ActualInTxCount, ExpectedOutTxCount, ActualOutTxCount, CompletionTime, BlockTime, TxNum, AddressNum, NewAddressNum, Reorg) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(query, log.Height, log.BlockHash, log.ExpectedInTxCount, log.ActualInTxCount, log.ExpectedOutTxCount, log.ActualOutTxCount, log.CompletionTime, log.BlockTime, log.TxNum, log.AddressNum, log.NewAddressNum, log.Reorg)
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("failed to insert IndexerLog: %w", err)
	}
	return nil
}
func UpdateIndexerReorg(fromHeight int, toHeight int) error {
	query := `UPDATE IndexerLog SET Reorg = 1 WHERE Height >= ? AND Height <= ?`
	_, err := db.Exec(query, fromHeight, toHeight)
	if err != nil {
		return fmt.Errorf("failed to update IndexerLog: %w", err)
	}
	return nil
}
func InsertErrLog(log ErrLog) error {
	query := `INSERT INTO ErrLog (ErrType, Height, BlockHash, Timestamp, ErrorMessage) 
		VALUES (?, ?, ?, ?, ?)`
	_, err := db.Exec(query, log.ErrType, log.Height, log.BlockHash, log.Timestamp, log.ErrorMessage)
	if err != nil {
		return fmt.Errorf("failed to insert ErrLog: %w", err)
	}
	return nil
}

func InsertReorgLog(log ReorgLog) error {
	query := `INSERT INTO ReorgLog (Height, EndHeight, BlockHash, NewBlockHash, ReorgSize, Timestamp, Status) 
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(query, log.Height, log.EndHeight, log.BlockHash, log.NewBlockHash, log.ReorgSize, log.Timestamp, log.Status)
	if err != nil {
		return fmt.Errorf("failed to insert ReorgLog: %w", err)
	}
	return nil
}

func QueryIndexerLogs(limit, offset int) ([]IndexerLog, error) {
	query := `SELECT Height, BlockHash, ExpectedInTxCount, ActualInTxCount, ExpectedOutTxCount, ActualOutTxCount, CompletionTime, BlockTime,TxNum,AddressNum,Reorg FROM IndexerLog ORDER BY ID DESC LIMIT ? OFFSET ?`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query IndexerLogs: %w", err)
	}
	defer rows.Close()

	var logs []IndexerLog
	for rows.Next() {
		var log IndexerLog
		if err := rows.Scan(&log.Height, &log.BlockHash, &log.ExpectedInTxCount, &log.ActualInTxCount, &log.ExpectedOutTxCount, &log.ActualOutTxCount, &log.CompletionTime, &log.BlockTime, &log.TxNum, &log.AddressNum, &log.Reorg); err != nil {
			return nil, fmt.Errorf("failed to scan IndexerLog: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

func QueryUnReorgIndexerLogs(limit, offset int) ([]IndexerLog, error) {
	query := `SELECT Height, BlockHash, ExpectedInTxCount, ActualInTxCount, ExpectedOutTxCount, ActualOutTxCount, CompletionTime, BlockTime, TxNum, AddressNum FROM IndexerLog WHERE Reorg = 0 ORDER BY ID DESC LIMIT ? OFFSET ?`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query IndexerLogs: %w", err)
	}
	defer rows.Close()

	var logs []IndexerLog
	for rows.Next() {
		var log IndexerLog
		if err := rows.Scan(&log.Height, &log.BlockHash, &log.ExpectedInTxCount, &log.ActualInTxCount, &log.ExpectedOutTxCount, &log.ActualOutTxCount, &log.CompletionTime, &log.BlockTime, &log.TxNum, &log.AddressNum); err != nil {
			return nil, fmt.Errorf("failed to scan IndexerLog: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

func QueryErrLogs(limit, offset int) ([]ErrLog, error) {
	query := `SELECT ErrType, Height, BlockHash, Timestamp, ErrorMessage FROM ErrLog ORDER BY ID DESC LIMIT ? OFFSET ?`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query ErrLogs: %w", err)
	}
	defer rows.Close()

	var logs []ErrLog
	for rows.Next() {
		var log ErrLog
		if err := rows.Scan(&log.ErrType, &log.Height, &log.BlockHash, &log.Timestamp, &log.ErrorMessage); err != nil {
			return nil, fmt.Errorf("failed to scan ErrLog: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}
func QueryReorgLogs(limit, offset int) ([]ReorgLog, error) {
	query := `SELECT Height, EndHeight, BlockHash, NewBlockHash, ReorgSize, Timestamp, Status FROM ReorgLog ORDER BY ID DESC LIMIT ? OFFSET ?`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query ReorgLogs: %w", err)
	}
	defer rows.Close()

	var logs []ReorgLog
	for rows.Next() {
		var log ReorgLog
		if err := rows.Scan(&log.Height, &log.EndHeight, &log.BlockHash, &log.NewBlockHash, &log.ReorgSize, &log.Timestamp, &log.Status); err != nil {
			return nil, fmt.Errorf("failed to scan ReorgLog: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}
func UpdateReorgStatus(height int64, status int) error {
	query := `UPDATE ReorgLog SET Status = ? WHERE Height = ?`
	_, err := db.Exec(query, status, height)
	if err != nil {
		return fmt.Errorf("failed to update ReorgLog status: %w", err)
	}
	return nil
}
