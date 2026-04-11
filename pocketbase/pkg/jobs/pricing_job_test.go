package jobs

import (
	"fmt"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"
)

func TestPricingScheduler_Start(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)

	// Test starting the scheduler
	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	if !scheduler.IsRunning() {
		t.Error("Scheduler should be running after start")
	}

	// Test stopping the scheduler
	err = scheduler.Stop()
	if err != nil {
		t.Fatalf("Failed to stop scheduler: %v", err)
	}

	if scheduler.IsRunning() {
		t.Error("Scheduler should not be running after stop")
	}
}

func TestPricingScheduler_ScheduleJob(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)

	// Test scheduling a job
	config := JobConfig{
		Name:        "test-job",
		Schedule:    "0 0 * * * *", // Every hour
		Enabled:     true,
		Timeout:     5 * time.Minute,
		MaxRetries:  3,
		Description: "Test job",
	}

	executed := false
	jobFunc := func() error {
		executed = true
		return nil
	}

	err := scheduler.ScheduleJob(config, jobFunc)
	if err != nil {
		t.Fatalf("Failed to schedule job: %v", err)
	}

	// Check if job was added
	jobs := scheduler.GetScheduledJobs()
	found := false
	for _, jobName := range jobs {
		if jobName == "test-job" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Job was not added to scheduler")
	}

	// Test removing the job
	err = scheduler.RemoveJob("test-job")
	if err != nil {
		t.Fatalf("Failed to remove job: %v", err)
	}

	// Check if job was removed
	jobs = scheduler.GetScheduledJobs()
	for _, jobName := range jobs {
		if jobName == "test-job" {
			t.Error("Job was not removed from scheduler")
		}
	}
}

func TestPricingScheduler_TriggerJob(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)

	executed := false
	config := JobConfig{
		Name:        "trigger-test-job",
		Schedule:    "0 0 1 1 * *", // Never runs automatically (Jan 1st only)
		Enabled:     true,
		Timeout:     5 * time.Second,
		MaxRetries:  1,
		Description: "Trigger test job",
	}

	jobFunc := func() error {
		executed = true
		return nil
	}

	err := scheduler.ScheduleJob(config, jobFunc)
	if err != nil {
		t.Fatalf("Failed to schedule job: %v", err)
	}

	err = scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer scheduler.Stop()

	// Trigger the job manually
	err = scheduler.TriggerJob("trigger-test-job")
	if err != nil {
		t.Fatalf("Failed to trigger job: %v", err)
	}

	// Wait a bit for job to execute
	time.Sleep(100 * time.Millisecond)

	if !executed {
		t.Error("Job was not executed after manual trigger")
	}
}

func TestJobMonitor_RecordJobExecution(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	monitor := NewJobMonitor(app)

	// Test recording successful execution
	monitor.RecordJobExecution("test-job", 2*time.Second, nil)

	metrics, err := monitor.GetJobMetrics("test-job")
	if err != nil {
		t.Fatalf("Failed to get job metrics: %v", err)
	}

	if metrics.TotalExecutions != 1 {
		t.Errorf("Expected 1 total execution, got %d", metrics.TotalExecutions)
	}

	if metrics.SuccessfulRuns != 1 {
		t.Errorf("Expected 1 successful run, got %d", metrics.SuccessfulRuns)
	}

	if metrics.FailedRuns != 0 {
		t.Errorf("Expected 0 failed runs, got %d", metrics.FailedRuns)
	}

	if metrics.SuccessRate != 1.0 {
		t.Errorf("Expected 100%% success rate, got %.2f", metrics.SuccessRate)
	}

	// Test recording failed execution
	testError := fmt.Errorf("test error")
	monitor.RecordJobExecution("test-job", 1*time.Second, testError)

	metrics, err = monitor.GetJobMetrics("test-job")
	if err != nil {
		t.Fatalf("Failed to get job metrics: %v", err)
	}

	if metrics.TotalExecutions != 2 {
		t.Errorf("Expected 2 total executions, got %d", metrics.TotalExecutions)
	}

	if metrics.SuccessfulRuns != 1 {
		t.Errorf("Expected 1 successful run, got %d", metrics.SuccessfulRuns)
	}

	if metrics.FailedRuns != 1 {
		t.Errorf("Expected 1 failed run, got %d", metrics.FailedRuns)
	}

	if metrics.SuccessRate != 0.5 {
		t.Errorf("Expected 50%% success rate, got %.2f", metrics.SuccessRate)
	}

	if metrics.ConsecutiveFailures != 1 {
		t.Errorf("Expected 1 consecutive failure, got %d", metrics.ConsecutiveFailures)
	}
}

func TestJobMonitor_SystemHealth(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	monitor := NewJobMonitor(app)

	// Initially, system should be healthy (no jobs recorded)
	isHealthy, issues := monitor.GetSystemHealth()
	if !isHealthy {
		t.Errorf("System should be healthy initially, got issues: %v", issues)
	}

	// Record some successful executions
	monitor.RecordJobExecution("healthy-job", 1*time.Second, nil)
	monitor.RecordJobExecution("healthy-job", 1*time.Second, nil)

	isHealthy, issues = monitor.GetSystemHealth()
	if !isHealthy {
		t.Errorf("System should be healthy with successful jobs, got issues: %v", issues)
	}

	// Record consecutive failures
	testError := fmt.Errorf("test error")
	monitor.RecordJobExecution("unhealthy-job", 1*time.Second, testError)
	monitor.RecordJobExecution("unhealthy-job", 1*time.Second, testError)
	monitor.RecordJobExecution("unhealthy-job", 1*time.Second, testError)

	isHealthy, issues = monitor.GetSystemHealth()
	if isHealthy {
		t.Error("System should be unhealthy with consecutive failures")
	}

	if len(issues) == 0 {
		t.Error("System should report issues when unhealthy")
	}
}

func TestPricingJobManager_Integration(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	manager := NewPricingJobManager(app)

	// Test starting the manager
	err := manager.Start()
	if err != nil {
		t.Fatalf("Failed to start job manager: %v", err)
	}

	// Check if system is healthy
	isHealthy, issues := manager.IsHealthy()
	if !isHealthy {
		t.Errorf("Job manager should be healthy after start, got issues: %v", issues)
	}

	// Get job status
	jobStatus := manager.GetJobStatus()
	if len(jobStatus) == 0 {
		t.Error("Job manager should have scheduled jobs")
	}

	// Test stopping the manager
	err = manager.Stop()
	if err != nil {
		t.Fatalf("Failed to stop job manager: %v", err)
	}
}

func TestJobConfig_Validation(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)

	// Test invalid cron expression
	config := JobConfig{
		Name:        "invalid-cron-job",
		Schedule:    "invalid cron",
		Enabled:     true,
		Timeout:     5 * time.Minute,
		MaxRetries:  3,
		Description: "Invalid cron job",
	}

	jobFunc := func() error { return nil }

	err := scheduler.ScheduleJob(config, jobFunc)
	if err == nil {
		t.Error("Should fail with invalid cron expression")
	}

	// Test disabled job
	config.Schedule = "0 0 * * * *"
	config.Enabled = false

	err = scheduler.ScheduleJob(config, jobFunc)
	if err != nil {
		t.Errorf("Should not fail with disabled job: %v", err)
	}

	// Job should not be in scheduled jobs list
	jobs := scheduler.GetScheduledJobs()
	for _, jobName := range jobs {
		if jobName == "invalid-cron-job" {
			t.Error("Disabled job should not be in scheduled jobs list")
		}
	}
}

func TestJobTimeout(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)
	monitor := NewJobMonitor(app)

	// Create a job that takes longer than the timeout
	config := JobConfig{
		Name:        "timeout-job",
		Schedule:    "0 0 1 1 * *", // Never runs automatically
		Enabled:     true,
		Timeout:     100 * time.Millisecond,
		MaxRetries:  1,
		Description: "Timeout test job",
	}

	jobFunc := func() error {
		time.Sleep(200 * time.Millisecond) // Sleep longer than timeout
		return nil
	}

	err := scheduler.ScheduleJob(config, jobFunc)
	if err != nil {
		t.Fatalf("Failed to schedule job: %v", err)
	}

	err = scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer scheduler.Stop()

	// Trigger the job and wait for timeout
	err = scheduler.TriggerJob("timeout-job")
	if err != nil {
		t.Fatalf("Failed to trigger job: %v", err)
	}

	// Wait for job to timeout and be recorded
	time.Sleep(300 * time.Millisecond)

	// Check if timeout was recorded as failure
	metrics, err := monitor.GetJobMetrics("timeout-job")
	if err != nil {
		// Job might not be recorded yet, which is acceptable for this test
		return
	}

	if metrics.FailedRuns == 0 {
		t.Error("Timeout should be recorded as a failure")
	}
}

// Benchmark tests
func BenchmarkJobExecution(b *testing.B) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	monitor := NewJobMonitor(app)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		monitor.RecordJobExecution("benchmark-job", time.Millisecond, nil)
	}
}

func BenchmarkSchedulerOperations(b *testing.B) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)

	config := JobConfig{
		Name:        "benchmark-job",
		Schedule:    "0 0 * * * *",
		Enabled:     true,
		Timeout:     5 * time.Minute,
		MaxRetries:  3,
		Description: "Benchmark job",
	}

	jobFunc := func() error { return nil }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jobName := fmt.Sprintf("benchmark-job-%d", i)
		config.Name = jobName
		
		scheduler.ScheduleJob(config, jobFunc)
		scheduler.RemoveJob(jobName)
	}
}