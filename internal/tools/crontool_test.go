package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockJobScheduler records calls and returns canned data.
type mockJobScheduler struct {
	lastJobName     string
	lastJobSchedule string
	lastJobPrompt   string
	jobs            []JobInfo
	addErr          error
}

func (m *mockJobScheduler) AddJob(name, schedule, prompt string) error {
	m.lastJobName = name
	m.lastJobSchedule = schedule
	m.lastJobPrompt = prompt
	return m.addErr
}

func (m *mockJobScheduler) ListJobs() []JobInfo {
	return m.jobs
}

func TestCronToolName(t *testing.T) {
	tool := &CronTool{}
	assert.Equal(t, "cron", tool.Name())
}

func TestCronToolParameters(t *testing.T) {
	tool := &CronTool{}
	params := tool.Parameters()
	assert.True(t, json.Valid(params), "Parameters() should return valid JSON")
}

func TestCronToolAddJob(t *testing.T) {
	scheduler := &mockJobScheduler{}
	tool := &CronTool{Scheduler: scheduler}
	input, _ := json.Marshal(cronInput{
		Action:   "add",
		Name:     "daily-check",
		Schedule: "24h",
		Prompt:   "Run daily diagnostics",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Output, "daily-check")
	assert.Contains(t, result.Output, "24h")

	assert.Equal(t, "daily-check", scheduler.lastJobName)
	assert.Equal(t, "24h", scheduler.lastJobSchedule)
	assert.Equal(t, "Run daily diagnostics", scheduler.lastJobPrompt)
}

func TestCronToolListJobs(t *testing.T) {
	scheduler := &mockJobScheduler{
		jobs: []JobInfo{
			{Name: "job1", Schedule: "1h", Prompt: "check status"},
			{Name: "job2", Schedule: "30m", Prompt: "monitor logs"},
		},
	}
	tool := &CronTool{Scheduler: scheduler}
	input, _ := json.Marshal(cronInput{Action: "list"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Output, "2 scheduled job(s)")
	assert.Contains(t, result.Output, "job1")
	assert.Contains(t, result.Output, "job2")
}

func TestCronToolListJobsEmpty(t *testing.T) {
	scheduler := &mockJobScheduler{jobs: []JobInfo{}}
	tool := &CronTool{Scheduler: scheduler}
	input, _ := json.Marshal(cronInput{Action: "list"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Output, "No scheduled jobs")
}

func TestCronToolUnknownAction(t *testing.T) {
	scheduler := &mockJobScheduler{}
	tool := &CronTool{Scheduler: scheduler}
	input, _ := json.Marshal(cronInput{Action: "delete"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "unknown action")
	assert.Contains(t, result.Error, "delete")
}

func TestCronToolAddMissingName(t *testing.T) {
	scheduler := &mockJobScheduler{}
	tool := &CronTool{Scheduler: scheduler}
	input, _ := json.Marshal(cronInput{
		Action:   "add",
		Schedule: "1h",
		Prompt:   "do stuff",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "name is required")
}

func TestCronToolAddMissingSchedule(t *testing.T) {
	scheduler := &mockJobScheduler{}
	tool := &CronTool{Scheduler: scheduler}
	input, _ := json.Marshal(cronInput{
		Action: "add",
		Name:   "job1",
		Prompt: "do stuff",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "schedule is required")
}

func TestCronToolAddMissingPrompt(t *testing.T) {
	scheduler := &mockJobScheduler{}
	tool := &CronTool{Scheduler: scheduler}
	input, _ := json.Marshal(cronInput{
		Action:   "add",
		Name:     "job1",
		Schedule: "1h",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "prompt is required")
}

func TestCronToolNilScheduler(t *testing.T) {
	tool := &CronTool{Scheduler: nil}
	input, _ := json.Marshal(cronInput{Action: "list"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "not available")
}

func TestCronToolAddJobError(t *testing.T) {
	scheduler := &mockJobScheduler{addErr: errors.New("duplicate name")}
	tool := &CronTool{Scheduler: scheduler}
	input, _ := json.Marshal(cronInput{
		Action:   "add",
		Name:     "job1",
		Schedule: "1h",
		Prompt:   "do stuff",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "failed to schedule job")
	assert.Contains(t, result.Error, "duplicate name")
}
