package repository

import "github.com/perfect-panel/server/internal/repository/kernel"

// The platform contracts are defined once in the shared kernel.
type (
	SystemRepo = kernel.SystemRepo
	LogRepo    = kernel.LogRepo
	TaskRepo   = kernel.TaskRepo
	InboxRepo  = kernel.InboxRepo
	OutboxRepo = kernel.OutboxRepo
)
