package consensus

import (
	"context"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// ParallelPoSValidator handles parallel validation of PoS blocks
type ParallelPoSValidator struct {
	consensus *PoSConsensus
	config    *ParallelPoSConfig
	params    *types.ChainParams // Chain parameters for validation rules
	logger    *logrus.Entry

	// Worker pool for kernel validation
	workers   int
	workQueue chan *KernelValidationJob
	results   chan *KernelValidationResult

	// Metrics
	metrics   *PoSValidationMetrics
	metricsMu sync.RWMutex

	// Shutdown handling
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ParallelPoSConfig contains configuration for parallel PoS validation
type ParallelPoSConfig struct {
	Workers           int
	QueueSize         int
	ValidationTimeout time.Duration
	EnableCaching     bool
	CacheSize         int
}

// PoSValidationMetrics tracks PoS validation performance
type PoSValidationMetrics struct {
	KernelsValidated   uint64
	ValidationTime     time.Duration
	CacheHits          uint64
	CacheMisses        uint64
	ParallelEfficiency float64
}

// KernelValidationJob represents a kernel validation task
type KernelValidationJob struct {
	ID            string
	Kernel        *types.StakeKernel
	TargetBits    uint32
	BlockHeight   uint32
	PrevBlockTime uint32
}

// KernelValidationResult contains the result of kernel validation
type KernelValidationResult struct {
	JobID   string
	Valid   bool
	Error   error
	Hash    types.Hash
	Meets   bool // Whether it meets the target
	Duration time.Duration
}

// DefaultParallelPoSConfig returns default configuration
func DefaultParallelPoSConfig() *ParallelPoSConfig {
	return &ParallelPoSConfig{
		Workers:           runtime.NumCPU(),
		QueueSize:         runtime.NumCPU() * 2,
		ValidationTimeout: 10 * time.Second,
		EnableCaching:     true,
		CacheSize:         10000,
	}
}

// NewParallelPoSValidator creates a new parallel PoS validator
func NewParallelPoSValidator(consensus *PoSConsensus, config *ParallelPoSConfig, params *types.ChainParams) *ParallelPoSValidator {
	if config == nil {
		config = DefaultParallelPoSConfig()
	}
	if params == nil {
		params = types.MainnetParams()
	}

	ctx, cancel := context.WithCancel(context.Background())

	validator := &ParallelPoSValidator{
		consensus: consensus,
		config:    config,
		params:    params,
		logger:    logrus.WithField("component", "parallel_pos_validator"),
		workers:   config.Workers,
		workQueue: make(chan *KernelValidationJob, config.QueueSize),
		results:   make(chan *KernelValidationResult, config.QueueSize),
		metrics:   &PoSValidationMetrics{},
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start worker pool
	validator.startWorkers()

	return validator
}

// startWorkers initializes the worker pool
func (v *ParallelPoSValidator) startWorkers() {
	for i := 0; i < v.workers; i++ {
		v.wg.Add(1)
		go v.worker(i)
	}

	v.logger.Infof("Started %d PoS validation workers", v.workers)
}

// worker processes kernel validation jobs
func (v *ParallelPoSValidator) worker(id int) {
	defer v.wg.Done()

	workerLogger := v.logger.WithField("worker", id)
	workerLogger.Debug("PoS worker started")

	for {
		select {
		case job, ok := <-v.workQueue:
			if !ok {
				workerLogger.Debug("Work queue closed, worker exiting")
				return
			}

			// Process the validation job
			result := v.processKernelValidation(job)

			// Send result
			select {
			case v.results <- result:
			case <-v.ctx.Done():
				workerLogger.Debug("Context cancelled, worker exiting")
				return
			}

		case <-v.ctx.Done():
			workerLogger.Debug("Context cancelled, worker exiting")
			return
		}
	}
}

// processKernelValidation validates a single stake kernel
func (v *ParallelPoSValidator) processKernelValidation(job *KernelValidationJob) *KernelValidationResult {
	start := time.Now()

	result := &KernelValidationResult{
		JobID: job.ID,
		Valid: false,
	}

	// Calculate kernel hash
	kernelHash, err := v.consensus.CalculateKernelHash(job.Kernel)
	if err != nil {
		result.Error = fmt.Errorf("failed to calculate kernel hash: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	result.Hash = kernelHash

	// Check if kernel meets target
	target := types.CompactToBig(job.TargetBits)
	hashBig := new(big.Int).SetBytes(kernelHash[:])
	meets := hashBig.Cmp(target) <= 0
	result.Meets = meets

	if !meets {
		result.Error = fmt.Errorf("kernel does not meet target difficulty")
		result.Duration = time.Since(start)
		return result
	}

	// Validate stake age
	if err := v.validateStakeAge(job.Kernel, job.BlockHeight); err != nil {
		result.Error = fmt.Errorf("stake age validation failed: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Validate stake value
	if err := v.validateStakeValue(job.Kernel); err != nil {
		result.Error = fmt.Errorf("stake value validation failed: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// All validations passed
	result.Valid = true
	result.Duration = time.Since(start)

	// Update metrics
	atomic.AddUint64(&v.metrics.KernelsValidated, 1)
	v.updateMetrics(result.Duration)

	return result
}

// validateStakeAge checks if the stake has sufficient age
func (v *ParallelPoSValidator) validateStakeAge(kernel *types.StakeKernel, blockHeight uint32) error {
	// CRITICAL: Use ChainParams.StakeMinAge (3 hours for mainnet)
	// Legacy kernel.cpp:311-313 checks: nTimeBlockFrom + nStakeMinAge > nTimeTx
	// where nStakeMinAge = 3 * 60 * 60 (main.cpp:84)
	minStakeAge := uint32(v.params.StakeMinAge.Seconds())

	// Get UTXO age from kernel
	utxoAge := kernel.Timestamp - kernel.PrevBlockTime
	if utxoAge < minStakeAge {
		return fmt.Errorf("stake age %d seconds is less than minimum %d", utxoAge, minStakeAge)
	}

	// NOTE: C++ does NOT have a maximum stake age limit (no MaxStakeAge in kernel.cpp)
	// Removed the 90-day cap that was incorrectly added in Go implementation

	return nil
}

// validateStakeValue checks if the stake value is valid
func (v *ParallelPoSValidator) validateStakeValue(kernel *types.StakeKernel) error {
	// CRITICAL: Use ChainParams.MinStakeAmount (12000 TWINS for mainnet)
	// Legacy kernel.cpp:317-319: if (nValueIn < Params().StakingMinInput()) return error(...)
	// MinStakeAmount = 12000 * 100000000 satoshis
	minStakeValue := uint64(v.params.MinStakeAmount)

	if kernel.StakeValue < minStakeValue {
		return fmt.Errorf("stake value %d is less than minimum %d", kernel.StakeValue, minStakeValue)
	}

	return nil
}

// ValidateBlockKernels validates all kernels in a block in parallel
func (v *ParallelPoSValidator) ValidateBlockKernels(ctx context.Context, block *types.Block) error {
	if !block.IsProofOfStake() {
		return fmt.Errorf("block is not proof-of-stake")
	}

	// Extract kernels from block
	kernels, err := v.extractKernels(block)
	if err != nil {
		return fmt.Errorf("failed to extract kernels: %w", err)
	}

	// Create validation context with timeout
	valCtx, cancel := context.WithTimeout(ctx, v.config.ValidationTimeout)
	defer cancel()

	// Submit all kernels for validation
	jobs := make([]*KernelValidationJob, len(kernels))
	for i, kernel := range kernels {
		job := &KernelValidationJob{
			ID:            fmt.Sprintf("kernel_%d", i),
			Kernel:        kernel,
			TargetBits:    block.Header.Bits,
			BlockHeight:   block.Height(),
			PrevBlockTime: block.Header.Timestamp - 600, // Approximate, should get from previous block
		}
		jobs[i] = job

		select {
		case v.workQueue <- job:
		case <-valCtx.Done():
			return fmt.Errorf("timeout submitting kernel validation jobs")
		}
	}

	// Collect results
	var lastError error
	successCount := 0

	for i := 0; i < len(jobs); i++ {
		select {
		case result := <-v.results:
			if !result.Valid {
				lastError = result.Error
				v.logger.WithError(result.Error).Errorf("Kernel %s validation failed", result.JobID)
			} else {
				successCount++
			}

		case <-valCtx.Done():
			return fmt.Errorf("kernel validation timeout after %s", v.config.ValidationTimeout)
		}
	}

	// Check if all kernels are valid
	if successCount != len(jobs) {
		return fmt.Errorf("kernel validation failed: %d/%d valid, last error: %w",
			successCount, len(jobs), lastError)
	}

	return nil
}

// extractKernels extracts stake kernels from a PoS block
func (v *ParallelPoSValidator) extractKernels(block *types.Block) ([]*types.StakeKernel, error) {
	kernels := make([]*types.StakeKernel, 0)

	// In TWINS, the coinstake transaction contains the kernel
	if len(block.Transactions) < 2 {
		return nil, fmt.Errorf("PoS block missing coinstake transaction")
	}

	coinstake := block.Transactions[1]
	if !coinstake.IsCoinStake() {
		return nil, fmt.Errorf("second transaction is not a coinstake")
	}

	// Get stake modifier for this block (from previous block)
	modifier, err := v.consensus.modifierCache.GetStakeModifier(block.Header.PrevBlockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get stake modifier: %w", err)
	}

	// Create kernel from coinstake
	for _, input := range coinstake.Inputs {
		kernel := &types.StakeKernel{
			StakeModifier: modifier,
			Timestamp:     block.Header.Timestamp,
			PrevOut:       input.PreviousOutput,
			StakeValue:    uint64(coinstake.Outputs[0].Value), // First output is typically the stake
		}
		kernels = append(kernels, kernel)
	}

	return kernels, nil
}

// BatchValidateKernels validates multiple kernels in parallel
func (v *ParallelPoSValidator) BatchValidateKernels(ctx context.Context, kernels []*types.StakeKernel, targetBits uint32) ([]bool, error) {
	results := make([]bool, len(kernels))
	resultMap := make(map[string]int) // Map job ID to index

	// Submit all jobs
	for i, kernel := range kernels {
		job := &KernelValidationJob{
			ID:            fmt.Sprintf("batch_%d", i),
			Kernel:        kernel,
			TargetBits:    targetBits,
			BlockHeight:   0, // Should be provided
			PrevBlockTime: 0, // Should be provided
		}
		resultMap[job.ID] = i

		select {
		case v.workQueue <- job:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Collect results
	for i := 0; i < len(kernels); i++ {
		select {
		case result := <-v.results:
			idx, ok := resultMap[result.JobID]
			if !ok {
				return nil, fmt.Errorf("unknown job ID: %s", result.JobID)
			}
			results[idx] = result.Valid

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return results, nil
}

// updateMetrics updates validation metrics
func (v *ParallelPoSValidator) updateMetrics(duration time.Duration) {
	v.metricsMu.Lock()
	defer v.metricsMu.Unlock()

	v.metrics.ValidationTime += duration

	// Calculate parallel efficiency
	if v.metrics.KernelsValidated > 0 {
		avgTime := v.metrics.ValidationTime / time.Duration(v.metrics.KernelsValidated)
		sequentialEstimate := avgTime * time.Duration(v.workers)
		v.metrics.ParallelEfficiency = float64(sequentialEstimate) / float64(v.metrics.ValidationTime)
	}
}

// GetMetrics returns current validation metrics
func (v *ParallelPoSValidator) GetMetrics() PoSValidationMetrics {
	v.metricsMu.RLock()
	defer v.metricsMu.RUnlock()
	return *v.metrics
}

// Shutdown gracefully shuts down the validator
func (v *ParallelPoSValidator) Shutdown(timeout time.Duration) error {
	v.logger.Info("Shutting down parallel PoS validator")

	// Signal shutdown
	v.cancel()

	// Close work queue
	close(v.workQueue)

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		v.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(v.results)
		v.logger.Info("Parallel PoS validator shutdown complete")
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("shutdown timeout after %s", timeout)
	}
}

// OptimizeWorkerCount dynamically adjusts worker count based on load
func (v *ParallelPoSValidator) OptimizeWorkerCount() {
	metrics := v.GetMetrics()

	// If efficiency is low, we might have too many workers
	if metrics.ParallelEfficiency < 0.7 && v.workers > 2 {
		v.logger.Debug("Reducing worker count due to low efficiency")
		// In production, implement dynamic worker adjustment
	}

	// If queue is consistently full, add more workers
	if len(v.workQueue) > cap(v.workQueue)/2 {
		v.logger.Debug("Queue pressure detected, consider adding workers")
		// In production, implement dynamic worker scaling
	}
}