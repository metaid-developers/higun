package api

import (
	"fmt"
	"reflect"
	"testing"
)

// A rebuild on an already-running FT mempool must not skip any step: rebuild,
// start and initialize all have to run again, in that order.
func TestFtRestartMempoolCoreRestartsStartAndInitialize(t *testing.T) {
	var steps []string
	server := &FtServer{
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
func TestFtRestartMempoolCorePropagatesFailure(t *testing.T) {
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
			server := &FtServer{
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
