// Command memory-probe measures the native UI in an isolated, disposable workspace.
// It never reads the user's connections and does not expose an HTTP profiler.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/ealink1/navi-fyne/internal/bootstrap"
	"github.com/ealink1/navi-fyne/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "memory probe:", err)
		os.Exit(1)
	}
}

func run() error {
	directory := flag.String("output", "", "new private output directory (required)")
	drivers := flag.String("drivers", "", "bundled driver directory")
	duration := flag.Duration("duration", 45*time.Second, "measure for this duration after the UI is ready")
	fixture := flag.Bool("fixture", false, "create a local SQLite fixture with two query drafts")
	flag.Parse()
	if *directory == "" || *duration < time.Second {
		return errors.New("output and a positive measurement duration are required")
	}
	output, err := filepath.Abs(*directory)
	if err != nil {
		return fmt.Errorf("resolve output directory: %w", err)
	}
	// Refuse reuse, so an existing application workspace cannot be opened by mistake.
	if err := os.Mkdir(output, 0700); err != nil {
		return fmt.Errorf("create new output directory: %w", err)
	}
	services, err := bootstrap.Open(filepath.Join(output, "workspace"), *drivers, "")
	if err != nil {
		return err
	}
	defer services.Close()
	if *fixture {
		if err := seedFixture(services); err != nil {
			return err
		}
	}
	application := app.NewWithID("io.github.ealink1.navifyne.memoryprobe")
	window := ui.New(application, ui.Dependencies{
		Profiles: services.Profiles, Engine: services.Engine, Drivers: services.Drivers,
		Releases: services.Releases, Root: services.Root, Version: "memory-probe", Close: services.Close,
	})
	window.Window.SetTitle("SuperLink Memory Probe")
	window.Show()
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	workers.Add(1)
	var measurementErr error
	go func() {
		defer workers.Done()
		startup := time.NewTimer(30 * time.Second)
		defer startup.Stop()
		select {
		case <-ctx.Done():
			return
		case <-startup.C:
			measurementErr = errors.New("UI did not become ready within 30 seconds")
			fyne.Do(window.Window.Close)
			return
		case <-window.Ready():
		}
		measurementErr = collect(ctx, output, *duration)
		if ctx.Err() == nil {
			fyne.Do(window.Window.Close)
		}
	}()
	application.Run()
	cancel()
	workers.Wait()
	return errors.Join(measurementErr, window.FlushAfterRun())
}
