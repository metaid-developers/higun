package diagnostics

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"log"
	"net"
	"net/http"
	httppprof "net/http/pprof"
	"os"
	"path/filepath"
	runtimepprof "runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/metaid/utxo_indexer/config"
)

type memorySampler struct {
	mu     sync.RWMutex
	latest Snapshot
}

func (s *memorySampler) set(snapshot Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = snapshot
}

func (s *memorySampler) get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}

func StartServer(cfg config.DiagnosticsConfig, cgroupRoot string) (func(), error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if err := validateDebugBind(cfg.Bind); err != nil {
		return nil, err
	}

	interval := time.Duration(cfg.SampleIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	sampler := &memorySampler{}
	dumpedProfiles := make(map[int]bool)
	captureAndStoreSnapshot(sampler, cfg, cgroupRoot, dumpedProfiles)

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/memory", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sampler.get()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	if cfg.ExpvarEnabled {
		mux.Handle("/debug/vars", expvar.Handler())
	}
	if cfg.PprofEnabled {
		mux.HandleFunc("/debug/pprof/", httppprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", httppprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", httppprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", httppprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", httppprof.Trace)
		mux.Handle("/debug/pprof/goroutine", httppprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/heap", httppprof.Handler("heap"))
		mux.Handle("/debug/pprof/allocs", httppprof.Handler("allocs"))
	}

	listener, err := net.Listen("tcp", cfg.Bind)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runMemorySampler(ctx, sampler, cfg, cgroupRoot, interval, dumpedProfiles)
	}()

	server := &http.Server{
		Addr:    cfg.Bind,
		Handler: mux,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[Diagnostics] server stopped with error: %v", err)
		}
	}()

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf("[Diagnostics] shutdown failed: %v", err)
			}
			wg.Wait()
		})
	}
	return stop, nil
}

func runMemorySampler(ctx context.Context, sampler *memorySampler, cfg config.DiagnosticsConfig, cgroupRoot string, interval time.Duration, dumpedProfiles map[int]bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			captureAndStoreSnapshot(sampler, cfg, cgroupRoot, dumpedProfiles)
		}
	}
}

func captureAndStoreSnapshot(sampler *memorySampler, cfg config.DiagnosticsConfig, cgroupRoot string, dumpedProfiles map[int]bool) {
	snapshot := CaptureSnapshot(cgroupRoot)
	sampler.set(snapshot)
	dumpHighWaterProfiles(cfg, snapshot, dumpedProfiles)
}

func dumpHighWaterProfiles(cfg config.DiagnosticsConfig, snapshot Snapshot, dumped map[int]bool) {
	current := snapshot.Cgroup.CurrentBytes
	limit := snapshot.Cgroup.LimitBytes
	if current == 0 || limit == 0 {
		return
	}

	percent := int((current * 100) / limit)
	for _, mark := range cfg.HighWaterProfilePercents {
		if mark <= 0 || dumped[mark] || percent < mark {
			continue
		}
		dumped[mark] = true
		if err := writeProfileDump(cfg.ProfileDir, mark, snapshot.Timestamp); err != nil {
			log.Printf("[Diagnostics] failed to dump high-water profiles at %d%%: %v", mark, err)
			continue
		}
	}
}

func writeProfileDump(profileDir string, mark int, timestamp time.Time) error {
	if profileDir == "" {
		profileDir = "."
	}
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return err
	}

	stamp := timestamp.UTC().Format("20060102T150405Z")
	heapPath := filepath.Join(profileDir, fmt.Sprintf("heap-%d-%s.pprof", mark, stamp))
	goroutinePath := filepath.Join(profileDir, fmt.Sprintf("goroutine-%d-%s.txt", mark, stamp))

	heapFile, err := os.Create(heapPath)
	if err != nil {
		return err
	}
	if err := runtimepprof.WriteHeapProfile(heapFile); err != nil {
		_ = heapFile.Close()
		return err
	}
	if err := heapFile.Close(); err != nil {
		return err
	}

	goroutineFile, err := os.Create(goroutinePath)
	if err != nil {
		return err
	}
	goroutineProfile := runtimepprof.Lookup("goroutine")
	if goroutineProfile == nil {
		_ = goroutineFile.Close()
		return fmt.Errorf("goroutine profile not found")
	}
	if err := goroutineProfile.WriteTo(goroutineFile, 2); err != nil {
		_ = goroutineFile.Close()
		return err
	}
	if err := goroutineFile.Close(); err != nil {
		return err
	}

	log.Printf("[Diagnostics] dumped high-water profiles at %d%%: heap=%s goroutine=%s", mark, heapPath, goroutinePath)
	return nil
}

func validateDebugBind(bind string) error {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return fmt.Errorf("invalid diagnostics bind %q: %w", bind, err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("diagnostics bind host %q is not localhost or a loopback IP", host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("diagnostics bind host %q is not loopback", host)
	}
	return nil
}

func IsDiagnosticsPath(path string) bool {
	return strings.HasPrefix(path, "/debug/")
}
