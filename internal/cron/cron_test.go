package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerAddAndRun(t *testing.T) {
	var callCount atomic.Int32

	s := NewScheduler()
	err := s.Add(Job{
		Name:     "test-job",
		Schedule: "50ms",
		Prompt:   "do something",
		AgentFn: func(ctx context.Context, prompt string) (string, error) {
			callCount.Add(1)
			assert.Equal(t, "do something", prompt)
			return "done", nil
		},
	})
	require.NoError(t, err)

	assert.Len(t, s.Jobs(), 1)

	s.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	assert.GreaterOrEqual(t, callCount.Load(), int32(1))
}

func TestSchedulerInvalidSchedule(t *testing.T) {
	s := NewScheduler()
	err := s.Add(Job{
		Name:     "bad-job",
		Schedule: "invalid",
		Prompt:   "test",
		AgentFn: func(ctx context.Context, prompt string) (string, error) {
			return "", nil
		},
	})
	assert.Error(t, err)
}

func TestSchedulerStop(t *testing.T) {
	s := NewScheduler()
	s.Add(Job{
		Name:     "slow-job",
		Schedule: "1h",
		Prompt:   "test",
		AgentFn: func(ctx context.Context, prompt string) (string, error) {
			return "ok", nil
		},
	})

	s.Start(context.Background())

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return in time")
	}
}

func TestSchedulerMultipleJobs(t *testing.T) {
	var count1, count2 atomic.Int32

	s := NewScheduler()
	s.Add(Job{
		Name:     "job1",
		Schedule: "50ms",
		Prompt:   "first",
		AgentFn: func(ctx context.Context, prompt string) (string, error) {
			count1.Add(1)
			return "ok", nil
		},
	})
	s.Add(Job{
		Name:     "job2",
		Schedule: "50ms",
		Prompt:   "second",
		AgentFn: func(ctx context.Context, prompt string) (string, error) {
			count2.Add(1)
			return "ok", nil
		},
	})

	assert.Len(t, s.Jobs(), 2)

	s.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	assert.GreaterOrEqual(t, count1.Load(), int32(1))
	assert.GreaterOrEqual(t, count2.Load(), int32(1))
}
