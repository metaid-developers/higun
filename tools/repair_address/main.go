package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/cespare/xxhash/v2"
	"github.com/cockroachdb/pebble"
)

func main() {
	dataDir := flag.String("data-dir", "data", "Pebble data directory")
	address := flag.String("address", "", "Address to repair")
	shardCount := flag.Int("shards", 4, "Shard count")
	flag.Parse()

	if *address == "" {
		log.Fatal("--address is required")
	}

	stores := map[string]string{
		"income":          "income",
		"spend":           "spend",
		"address_balance": "address_balance",
		"balance_rank":    "balance_rank",
	}

	for storeName, storeDir := range stores {
		shardIdx := int(xxhash.Sum64String(*address) % uint64(*shardCount))
		dbPath := fmt.Sprintf("%s/%s/shard_%d", *dataDir, storeDir, shardIdx)

		db, err := pebble.Open(dbPath, &pebble.Options{})
		if err != nil {
			log.Printf("WARN: cannot open %s: %v", storeName, err)
			continue
		}

		// Check if key exists
		val, closer, err := db.Get([]byte(*address))
		if err == pebble.ErrNotFound {
			log.Printf("%s: key not found, skip", storeName)
			db.Close()
			continue
		}
		if err != nil {
			log.Printf("WARN: %s read error: %v", storeName, err)
			if closer != nil {
				closer.Close()
			}
			db.Close()
			continue
		}
		valLen := len(val)
		closer.Close()

		// Delete the key
		if err := db.Delete([]byte(*address), pebble.Sync); err != nil {
			log.Printf("ERROR: %s delete failed: %v", storeName, err)
			db.Close()
			continue
		}

		log.Printf("%s: DELETED (was %d bytes, shard %d)", storeName, valLen, shardIdx)
		db.Close()
	}

	log.Println("DONE - restart HIGUN and trigger reindex")
}
