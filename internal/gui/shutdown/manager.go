package shutdown

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Stage represents a shutdown stage
type Stage string

const (
	StageInitializing       Stage = "initializing"
	StageStoppingServices   Stage = "stopping_services"
	StageSavingData         Stage = "saving_data"
	StageClosingConnections Stage = "closing_connections"
	StageCleanup            Stage = "cleanup"
	StageComplete           Stage = "complete"
)

// Shutdown timeouts
const (
	// ShutdownTimeout is the maximum time for graceful shutdown
	ShutdownTimeout = 15 * time.Second
	// ServiceTimeout is the maximum time per service shutdown
	ServiceTimeout = 5 * time.Second
	// ForceExitDelay is the delay before force exit after completion
	ForceExitDelay = 500 * time.Millisecond
)

// Progress represents shutdown progress information
type Progress struct {
	Message    string  `json:"message"`
	Percentage float64 `json:"percentage"`
	Stage      Stage   `json:"stage"`
}

// Service represents a service that needs to be shut down
type Service interface {
	Name() string
	Shutdown(ctx context.Context) error
}

// Manager handles the graceful shutdown of the application
type Manager struct {
	mu               sync.RWMutex
	ctx              context.Context
	services         []Service
	isShuttingDown   bool
	shutdownComplete atomic.Bool
	currentStage     Stage
	progress         Progress
	shutdownChannel  chan error
	callbacks        []func() error
	lastError        error
}

// NewManager creates a new shutdown manager
func NewManager(ctx context.Context) *Manager {
	return &Manager{
		ctx:             ctx,
		services:        make([]Service, 0),
		shutdownChannel: make(chan error, 1),
		callbacks:       make([]func() error, 0),
	}
}

// RegisterService registers a service for shutdown
func (m *Manager) RegisterService(service Service) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.services = append(m.services, service)
	log.WithField("service", service.Name()).Debug("shutdown: registered service")
}

// RegisterCallback registers a cleanup callback
func (m *Manager) RegisterCallback(callback func() error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callbacks = append(m.callbacks, callback)
	log.Debug("shutdown: registered cleanup callback")
}

// StartShutdown initiates the shutdown sequence (non-blocking).
// Returns immediately after starting the shutdown goroutine.
// Use IsShutdownComplete() to check if shutdown has finished.
func (m *Manager) StartShutdown() error {
	m.mu.Lock()

	if m.isShuttingDown {
		m.mu.Unlock()
		return fmt.Errorf("shutdown already in progress")
	}

	m.isShuttingDown = true
	m.currentStage = StageInitializing
	m.mu.Unlock()

	log.Info("shutdown: starting shutdown sequence")

	// Run shutdown in a goroutine - do NOT block the caller
	go m.executeShutdown()

	// Return immediately - frontend polls GetShutdownProgress() for status
	return nil
}

// executeShutdown performs the actual shutdown sequence.
// No artificial delays - proceeds as fast as operations complete.
func (m *Manager) executeShutdown() {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic during shutdown: %v", r)
			log.WithError(err).Error("shutdown: panic during shutdown")
			m.setError(err)
			m.markComplete()
		}
	}()

	// Create shutdown context with overall timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()

	// Stage 1: Initialize
	m.updateProgress(StageInitializing, "TWINS Core is shutting down...", 10)

	// Stage 2: Stop services (with timeout)
	m.updateProgress(StageStoppingServices, "Stopping wallet services...", 20)
	if err := m.stopServicesWithContext(shutdownCtx); err != nil {
		m.emitError(fmt.Sprintf("Failed to stop services: %v", err))
		m.setError(err)
		// Continue with shutdown even on error
	}

	// Stage 3: Save data (with timeout)
	m.updateProgress(StageSavingData, "Saving wallet data...", 50)
	if err := m.saveDataWithContext(shutdownCtx); err != nil {
		m.emitError(fmt.Sprintf("Failed to save data: %v", err))
		// Continue with shutdown even if save fails
	}

	// Stage 4: Close connections
	m.updateProgress(StageClosingConnections, "Closing network connections...", 70)
	if err := m.closeConnections(); err != nil {
		m.emitError(fmt.Sprintf("Failed to close connections: %v", err))
		// Continue with shutdown
	}

	// Stage 5: Cleanup callbacks
	m.updateProgress(StageCleanup, "Cleaning up resources...", 85)
	if err := m.cleanupWithContext(shutdownCtx); err != nil {
		m.emitError(fmt.Sprintf("Failed during cleanup: %v", err))
		// Continue with shutdown
	}

	// Stage 6: Complete
	m.updateProgress(StageComplete, "Shutdown complete", 100)
	m.markComplete()

	// Emit completion event
	runtime.EventsEmit(m.ctx, "shutdown:complete")

	// Signal successful completion (for any waiters)
	select {
	case m.shutdownChannel <- m.getError():
	default:
		// Channel full or no receivers, that's fine
	}

	// Exit the application after a brief delay to let UI update
	go func() {
		time.Sleep(ForceExitDelay)
		runtime.Quit(m.ctx)
	}()
}

// setError stores the last error (thread-safe)
func (m *Manager) setError(err error) {
	m.mu.Lock()
	m.lastError = err
	m.mu.Unlock()
}

// getError returns the last error (thread-safe)
func (m *Manager) getError() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastError
}

// markComplete marks shutdown as complete (thread-safe)
func (m *Manager) markComplete() {
	m.shutdownComplete.Store(true)
	m.mu.Lock()
	m.currentStage = StageComplete
	m.mu.Unlock()
}

// stopServicesWithContext stops all registered services with context timeout
func (m *Manager) stopServicesWithContext(ctx context.Context) error {
	m.mu.RLock()
	services := make([]Service, len(m.services))
	copy(services, m.services)
	m.mu.RUnlock()

	if len(services) == 0 {
		log.Debug("shutdown: no services to stop")
		return nil
	}

	// Stop services in reverse order of registration
	for i := len(services) - 1; i >= 0; i-- {
		service := services[i]
		log.WithField("service", service.Name()).Debug("shutdown: stopping service")

		// Per-service timeout
		serviceCtx, cancel := context.WithTimeout(ctx, ServiceTimeout)
		err := service.Shutdown(serviceCtx)
		cancel()

		if err != nil {
			log.WithField("service", service.Name()).WithError(err).Warn("shutdown: error stopping service")
			// Continue stopping other services
		} else {
			log.WithField("service", service.Name()).Debug("shutdown: service stopped")
		}

		// Update progress
		progress := 20 + float64(len(services)-i)*30/float64(len(services))
		m.updateProgress(StageStoppingServices, fmt.Sprintf("Stopped %s", service.Name()), progress)

		// Check if overall context cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// saveDataWithContext is a progress-only stage; actual data saving is done in app.shutdown() callback.
func (m *Manager) saveDataWithContext(ctx context.Context) error {
	m.updateProgress(StageSavingData, "Data saved", 60)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// closeConnections is a no-op placeholder; connection cleanup is handled by service shutdown.
func (m *Manager) closeConnections() error {
	return nil
}

// cleanupWithContext performs final cleanup tasks with context timeout
func (m *Manager) cleanupWithContext(ctx context.Context) error {
	log.Debug("shutdown: performing cleanup")

	m.mu.RLock()
	callbacks := make([]func() error, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.mu.RUnlock()

	if len(callbacks) == 0 {
		return nil
	}

	// Execute all cleanup callbacks
	for i, callback := range callbacks {
		// Check context before each callback
		select {
		case <-ctx.Done():
			log.Warn("shutdown: cleanup interrupted by timeout")
			return ctx.Err()
		default:
		}

		if err := callback(); err != nil {
			log.WithField("callback", i).WithError(err).Warn("shutdown: cleanup callback failed")
			// Continue with other callbacks
		}

		// Update progress
		progress := 85 + float64(i+1)*10/float64(len(callbacks))
		m.updateProgress(StageCleanup, "Cleaning up...", progress)
	}

	return nil
}

// updateProgress updates and emits the current progress.
// Uses copy-under-lock pattern: state is updated under mutex, then the event
// is emitted after releasing the lock to prevent deadlock if EventsEmit blocks
// (e.g., Wails runtime already torn down during shutdown).
func (m *Manager) updateProgress(stage Stage, message string, percentage float64) {
	m.mu.Lock()
	m.currentStage = stage
	m.progress = Progress{
		Message:    message,
		Percentage: percentage,
		Stage:      stage,
	}
	progress := m.progress // copy under lock
	m.mu.Unlock()

	// Emit progress event to frontend (outside lock)
	runtime.EventsEmit(m.ctx, "shutdown:progress", progress)
	log.WithFields(log.Fields{"stage": string(stage), "pct": percentage}).Debug(message)
}

// emitError emits an error event to the frontend
func (m *Manager) emitError(message string) {
	runtime.EventsEmit(m.ctx, "shutdown:error", message)
	log.Error("shutdown: " + message)
}

// IsShuttingDown returns whether shutdown is in progress
func (m *Manager) IsShuttingDown() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.isShuttingDown && m.currentStage != StageComplete
}

// IsShutdownComplete returns whether shutdown has completed
func (m *Manager) IsShutdownComplete() bool {
	return m.shutdownComplete.Load()
}

// GetProgress returns the current shutdown progress
func (m *Manager) GetProgress() Progress {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.progress
}

// GetCurrentStage returns the current shutdown stage
func (m *Manager) GetCurrentStage() Stage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.currentStage
}

// ForceShutdown performs an immediate shutdown without cleanup.
// Uses os.Exit(1) to guarantee exit even if Wails runtime is stuck.
func (m *Manager) ForceShutdown() {
	log.Warn("shutdown: force shutdown initiated - exiting immediately")
	m.markComplete()

	// Try to emit event but don't wait
	go func() {
		runtime.EventsEmit(m.ctx, "shutdown:complete")
	}()

	// Force exit with os.Exit - this guarantees termination
	// even if runtime.Quit() would hang
	os.Exit(0)
}