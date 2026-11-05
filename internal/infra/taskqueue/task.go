package taskqueue

import "time"

const (
	// ScheduledBatchSendEmail runs a batch email task.
	ScheduledBatchSendEmail = "scheduled:email:batch"

	// ForthwithQuotaTask runs a quota task right away.
	ForthwithQuotaTask = "forthwith:quota:task"

	// SchedulerExchangeRate refreshes the currency exchange rates.
	SchedulerExchangeRate = "scheduler:exchange:rate"
)

// BatchEmailTaskTimeout bounds one run of a batch email task, as the
// asynq.Timeout of every batch email enqueue (the initial one and each
// continuation). asynq's default is 30 minutes, which a paced campaign to
// thousands of recipients overruns; the worker stops itself before the
// deadline and the campaign continues from a follow-up task, so the timeout
// only has to be long enough for a chunk worth running.
const BatchEmailTaskTimeout = 6 * time.Hour
