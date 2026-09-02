package api

import (
	"fmt"
	"reflect"
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

// A rebuild on an already-running mempool must not skip any step: rebuild,
// start and initialize all have to run again, in that order.
func TestRestartMempoolCoreRestartsStartAndInitialize(t *testing.T) {
	var steps []string
	server := &Server{
		stopCh:      make(chan struct{}),
		mempoolInit: true,
		rebuildMempoolFn: func() error {
			steps = append(steps, "rebuild")
			return nil
		},
		startMempoolFn: func() error {
			steps = append(steps, "start")
			return nil
		},
		initializeMempoolFn: func() {
			steps = append(steps, "init")
		},
	}

	if err := server.restartMempoolCore(); err != nil {
		t.Fatalf("restartMempoolCore: %v", err)
	}

	want := []string{"rebuild", "start", "init"}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("rebuild executed steps %v, want %v", steps, want)
	}
}

// A failing step must fail the whole rebuild and skip the remaining steps.
func TestRestartMempoolCorePropagatesFailure(t *testing.T) {
	cases := []struct {
		name       string
		rebuildErr error
		startErr   error
	}{
		{"rebuild failure", fmt.Errorf("rebuild failed"), nil},
		{"start failure", nil, fmt.Errorf("start failed")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var startRan, initRan bool
			server := &Server{
				stopCh:      make(chan struct{}),
				mempoolInit: true,
				rebuildMempoolFn: func() error {
					return tc.rebuildErr
				},
				startMempoolFn: func() error {
					startRan = true
					return tc.startErr
				},
				initializeMempoolFn: func() {
					initRan = true
				},
			}

			if err := server.restartMempoolCore(); err == nil {
				t.Fatal("expected restartMempoolCore to fail")
			}
			if tc.rebuildErr != nil && (startRan || initRan) {
				t.Fatal("no later step may run after a rebuild failure")
			}
			if tc.startErr != nil && initRan {
				t.Fatal("initialize must not run after a start failure")
			}
		})
	}
}

// Rebuilding a mempool that was never started must bring up the cleaner
// goroutine (once) instead of leaving the rebuilt mempool uncleaned.
func TestRestartMempoolCoreStartsCleanerWhenNotInitialized(t *testing.T) {
	cleanerStarted := make(chan struct{}, 1)
	server := &Server{
		stopCh:      make(chan struct{}),
		mempoolInit: false,
		rebuildMempoolFn: func() error {
			return nil
		},
		startMempoolFn: func() error {
			return nil
		},
		initializeMempoolFn: func() {},
		startMempoolCleanerFn: func() {
			cleanerStarted <- struct{}{}
		},
	}

	if err := server.restartMempoolCore(); err != nil {
		t.Fatalf("restartMempoolCore: %v", err)
	}

	select {
	case <-cleanerStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected mempool cleaner to start")
	}
	if !server.mempoolInit {
		t.Fatal("expected mempoolInit to be set after rebuild")
	}
}
