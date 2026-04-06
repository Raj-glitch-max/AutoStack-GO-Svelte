package jobs

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/robfig/cron/v3"
)

// PricingScheduler manages scheduled pricing update jobs
type PricingScheduler struct {
	app        core.App
	cron       *cron.Cron
	jobs       map[string]cron.EntryID
	jobsMutex  sync.RWMutex
	running    bool
	runMutex   sync.RWMutex
}

// JobConfig defines configuration for a scheduled job
type JobConfig struct {
	Name        string        `json:"name"`
	Schedule    string        `json:"schedule"`    // Cron expression
	Enabled     bool          `json:"enabled"`
	Timeout     time.Duration `json:"timeout"`
	MaxRetries  int           `json:"maxRetries"`
	Description string        `json:"description"`
}

// JobStatus represents the current status of a job
type JobStatus struct {
	Name           string    `json:"name"`
	Schedule       string    `json:"schedule"`
	Enabled        bool      `json:"enabled"`
	LastRun        time.Time `json:"lastRun"`
	NextRun        time.Time `json:"nextRun"`
	LastSuccess    time.Time `json:"lastSuccess"`
	LastError      string    `json:"lastError"`
	RunCount       int64     `json:"runCount"`
	SuccessCount   int64     `json:"successCount"`
	ErrorCount     int64     `json:"errorCount"`
	AverageRuntime float64   `json:"averageRuntime"` // in seconds
}

// NewPricingScheduler creates a new pricing scheduler instance
func NewPricingScheduler(app core.App) *PricingScheduler {
	// Create cron with UTC timezone and second precision
	c := cron.New(cron.WithLocation(time.UTC), cron.WithSeconds())
	
	return &PricingScheduler{
		app:     app,
		cron:    c,
		jobs:    make(map[string]cron.EntryID),
		running: false,
	}
}

// Start begins the scheduler
func (ps *PricingScheduler) Start() error {
	ps.runMutex.Lock()
	defer ps.runMutex.Unlock()
	
	if ps.running {
		return fmt.Errorf("scheduler is already running")
	}
	
	log.Println("Starting pricing scheduler...")
	
	// Schedule default pricing jobs
	if err := ps.scheduleDefaultJobs(); err != nil {
		return fmt.Errorf("failed to schedule default jobs: %w", err)
	}
	
	ps.cron.Start()
	ps.running = true
	
	log.Println("Pricing scheduler started successfully")
	return nil
}

// Stop stops the scheduler and waits for running jobs to complete
func (ps *PricingScheduler) Stop() error {
	ps.runMutex.Lock()
	defer ps.runMutex.Unlock()
	
	if !ps.running {
		return fmt.Errorf("scheduler is not running")
	}
	
	log.Println("Stopping pricing scheduler...")
	
	// Stop accepting new jobs and wait for running jobs to complete
	ctx := ps.cron.Stop()
	
	// Wait for all jobs to complete with timeout
	select {
	case <-ctx.Done():
		log.Println("All jobs completed gracefully")
	case <-time.After(30 * time.Second):
		log.Println("Timeout waiting for jobs to complete")
	}
	
	ps.running = false
	log.Println("Pricing scheduler stopped")
	return nil
}

// scheduleDefaultJobs sets up the default pricing update jobs
func (ps *PricingScheduler) scheduleDefaultJobs() error {
	// Daily pricing fetch at 2 AM UTC
	dailyPricingConfig := JobConfig{
		Name:        "daily-pricing-fetch",
		Schedule:    "0 0 2 * * *", // Every day at 2:00 AM UTC
		Enabled:     true,
		Timeout:     30 * time.Minute,
		MaxRetries:  3,
		Description: "Fetch latest AWS pricing data from Price List API",
	}
	
	if err := ps.ScheduleJob(dailyPricingConfig, ps.createPricingFetchJob()); err != nil {
		return fmt.Errorf("failed to schedule daily pricing fetch: %w", err)
	}
	
	// Hourly pricing cache health check
	healthCheckConfig := JobConfig{
		Name:        "pricing-cache-health-check",
		Schedule:    "0 0 * * * *", // Every hour at minute 0
		Enabled:     true,
		Timeout:     5 * time.Minute,
		MaxRetries:  1,
		Description: "Check pricing cache health and data freshness",
	}
	
	if err := ps.ScheduleJob(healthCheckConfig, ps.createHealthCheckJob()); err != nil {
		return fmt.Errorf("failed to schedule health check: %w", err)
	}
	
	// Weekly pricing data cleanup
	cleanupConfig := JobConfig{
		Name:        "pricing-data-cleanup",
		Schedule:    "0 0 3 * * 0", // Every Sunday at 3:00 AM UTC
		Enabled:     true,
		Timeout:     10 * time.Minute,
		MaxRetries:  2,
		Description: "Clean up old pricing data older than 90 days",
	}
	
	if err := ps.ScheduleJob(cleanupConfig, ps.createCleanupJob()); err != nil {
		return fmt.Errorf("failed to schedule cleanup job: %w", err)
	}
	
	return nil
}

// ScheduleJob adds a new job to the scheduler
func (ps *PricingScheduler) ScheduleJob(config JobConfig, jobFunc func() error) error {
	ps.jobsMutex.Lock()
	defer ps.jobsMutex.Unlock()
	
	if !config.Enabled {
		log.Printf("Job %s is disabled, skipping", config.Name)
		return nil
	}
	
	// Wrap the job function with monitoring and error handling
	wrappedJob := ps.wrapJobWithMonitoring(config, jobFunc)
	
	// Add job to cron
	entryID, err := ps.cron.AddFunc(config.Schedule, wrappedJob)
	if err != nil {
		return fmt.Errorf("failed to add job %s: %w", config.Name, err)
	}
	
	// Store job entry ID for management
	ps.jobs[config.Name] = entryID
	
	log.Printf("Scheduled job: %s with schedule: %s", config.Name, config.Schedule)
	return nil
}

// RemoveJob removes a job from the scheduler
func (ps *PricingScheduler) RemoveJob(jobName string) error {
	ps.jobsMutex.Lock()
	defer ps.jobsMutex.Unlock()
	
	entryID, exists := ps.jobs[jobName]
	if !exists {
		return fmt.Errorf("job %s not found", jobName)
	}
	
	ps.cron.Remove(entryID)
	delete(ps.jobs, jobName)
	
	log.Printf("Removed job: %s", jobName)
	return nil
}

// wrapJobWithMonitoring wraps a job function with monitoring, timeout, and error handling
func (ps *PricingScheduler) wrapJobWithMonitoring(config JobConfig, jobFunc func() error) func() {
	return func() {
		startTime := time.Now()
		
		log.Printf("Starting job: %s", config.Name)
		
		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
		defer cancel()
		
		// Channel to receive job result
		resultChan := make(chan error, 1)
		
		// Run job in goroutine
		go func() {
			defer func() {
				if r := recover(); r != nil {
					resultChan <- fmt.Errorf("job panicked: %v", r)
				}
			}()
			
			resultChan <- jobFunc()
		}()
		
		// Wait for job completion or timeout
		var err error
		select {
		case err = <-resultChan:
			// Job completed
		case <-ctx.Done():
			err = fmt.Errorf("job timed out after %v", config.Timeout)
		}
		
		duration := time.Since(startTime)
		
		// Update job statistics
		ps.updateJobStats(config.Name, err == nil, duration, err)
		
		if err != nil {
			log.Printf("Job %s failed after %v: %v", config.Name, duration, err)
		} else {
			log.Printf("Job %s completed successfully in %v", config.Name, duration)
		}
	}
}

// updateJobStats updates job execution statistics
func (ps *PricingScheduler) updateJobStats(jobName string, success bool, duration time.Duration, err error) {
	// This would typically update a database record or metrics system
	// For now, we'll log the statistics
	
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}
	
	log.Printf("Job stats - Name: %s, Status: %s, Duration: %v", jobName, status, duration)
	
	// TODO: Store job statistics in database for monitoring dashboard
	// This could be implemented as a separate collection: jobExecutions
}

// GetJobStatus returns the current status of all scheduled jobs
func (ps *PricingScheduler) GetJobStatus() []JobStatus {
	ps.jobsMutex.RLock()
	defer ps.jobsMutex.RUnlock()
	
	var statuses []JobStatus
	
	for jobName, entryID := range ps.jobs {
		entry := ps.cron.Entry(entryID)
		
		status := JobStatus{
			Name:     jobName,
			Schedule: "unknown", // Would need to store schedule separately
			Enabled:  true,
			NextRun:  entry.Next,
			// Other fields would come from stored statistics
		}
		
		statuses = append(statuses, status)
	}
	
	return statuses
}

// TriggerJob manually triggers a job execution
func (ps *PricingScheduler) TriggerJob(jobName string) error {
	ps.jobsMutex.RLock()
	entryID, exists := ps.jobs[jobName]
	ps.jobsMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("job %s not found", jobName)
	}
	
	entry := ps.cron.Entry(entryID)
	if entry.Job == nil {
		return fmt.Errorf("job %s has no executable function", jobName)
	}
	
	log.Printf("Manually triggering job: %s", jobName)
	
	// Run job in background
	go entry.Job.Run()
	
	return nil
}

// IsRunning returns whether the scheduler is currently running
func (ps *PricingScheduler) IsRunning() bool {
	ps.runMutex.RLock()
	defer ps.runMutex.RUnlock()
	return ps.running
}

// GetScheduledJobs returns a list of all scheduled job names
func (ps *PricingScheduler) GetScheduledJobs() []string {
	ps.jobsMutex.RLock()
	defer ps.jobsMutex.RUnlock()
	
	var jobNames []string
	for name := range ps.jobs {
		jobNames = append(jobNames, name)
	}
	
	return jobNames
}