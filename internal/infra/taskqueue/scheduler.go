package taskqueue

// The task types of the scheduled jobs.
const (
	SchedulerCheckSubscription = "scheduler:check:subscription"
	// SchedulerRemindExpiringSubscriptions warns owners whose subscription
	// expires soon. Daily, at a civil hour.
	SchedulerRemindExpiringSubscriptions = "scheduler:subscription:remind-expiring"
	// SchedulerDispatchDomainEvents pumps the generic domain-event outbox
	// onto the asynq queue as events:deliver tasks.
	SchedulerDispatchDomainEvents = "scheduler:events:dispatch"
	SchedulerResetTraffic         = "scheduler:reset:traffic"
	SchedulerTrafficStat          = "scheduler:traffic:stat"
	SchedulerLogCleanup           = "scheduler:log:cleanup"
	SchedulerFlushTraffic         = "scheduler:flush:traffic"
)
