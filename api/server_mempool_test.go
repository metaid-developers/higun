package api

import (
	"testing"
	"time"
)

func TestStartMempoolCoreStartsCleaner(t *testing.T) {
	cleanerStarted := make(chan struct{}, 1)
	initStarted := make(chan struct{}, 1)

	server := &Server{
		stopCh: make(chan struct{}),
		startMempoolFn: func() error {
			return nil
		},
		initializeMempoolFn: func() {
			initStarted <- struct{}{}
		},
		startMempoolCleanerFn: func() {
			cleanerStarted <- struct{}{}
		},
	}

	if err := server.StartMempoolCore(); err != nil {
		t.Fatalf("StartMempoolCore: %v", err)
	}

	select {
	case <-cleanerStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected mempool cleaner to start")
	}

	select {
	case <-initStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected mempool initialization to start")
	}
}
