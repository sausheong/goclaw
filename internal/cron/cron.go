package cron

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// AgentFunc is a function that sends a prompt to an agent and returns its response.
type AgentFunc func(ctx context.Context, prompt string) (string, error)

// Job represents a scheduled task.
type Job struct {
	Name     string
	Schedule string        // cron-like: "30m", "1h", "24h", or time.Duration parseable string
	Prompt   string        // prompt to send to the agent
	AgentFn  AgentFunc
	interval time.Duration // parsed interval
}

// Scheduler runs cron jobs at their configured intervals.
type Scheduler struct {
	jobs   []Job
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler creates a new cron scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{}
}

// Add registers a new job with the scheduler.
func (s *Scheduler) Add(job Job) error {
	d, err := time.ParseDuration(job.Schedule)
	if err != nil {
		return err
	}
	job.interval = d
	s.jobs = append(s.jobs, job)
	return nil
}

// Start begins running all scheduled jobs.
func (s *Scheduler) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	for _, job := range s.jobs {
		s.wg.Add(1)
		go s.runJob(ctx, job)
	}

	slog.Info("cron scheduler started", "jobs", len(s.jobs))
}

// Stop cancels all running jobs and waits for them to finish.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	slog.Info("cron scheduler stopped")
}

func (s *Scheduler) runJob(ctx context.Context, job Job) {
	defer s.wg.Done()

	ticker := time.NewTicker(job.interval)
	defer ticker.Stop()

	slog.Info("cron job registered", "name", job.Name, "interval", job.interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			slog.Info("cron job running", "name", job.Name)

			response, err := job.AgentFn(ctx, job.Prompt)
			if err != nil {
				if ctx.Err() != nil {
					return // context cancelled, stop gracefully
				}
				slog.Error("cron job failed", "name", job.Name, "error", err)
				continue
			}

			slog.Info("cron job completed", "name", job.Name, "response_length", len(response))
		}
	}
}

// Jobs returns the list of configured jobs.
func (s *Scheduler) Jobs() []Job {
	return append([]Job{}, s.jobs...)
}
