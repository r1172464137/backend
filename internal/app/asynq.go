package app

import (
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
)

// QueueRedisOpt is the task queue's Redis connection, shared by the task
// client, the consumer and the periodic scheduler. The queue lives in the
// Redis database Redis.QueueDB names (5 unless the file says otherwise), so
// two deployments sharing one Redis keep their queues apart by giving each
// its own value; the producer, the consumer and the scheduler read it from
// here only.
func QueueRedisOpt(c config.Config) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: c.Redis.Host, Password: c.Redis.Pass, DB: c.Redis.QueueDB}
}

// NewAsynqClient returns the tracing asynq client: EnqueueContext stamps the
// caller's trace context onto the task for the worker-side middleware to
// resume. Pass task options to EnqueueContext, not NewTask — wrapping
// rebuilds the task.
func NewAsynqClient(c config.Config) *taskqueue.Client {
	return taskqueue.NewClient(asynq.NewClient(QueueRedisOpt(c)))
}

// NewAsynqInspector returns the queue inspector with which the paid-order
// reconciler repairs conflicting activation tasks.
func NewAsynqInspector(c config.Config) *asynq.Inspector {
	return asynq.NewInspector(QueueRedisOpt(c))
}
