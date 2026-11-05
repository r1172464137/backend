// Package readmodel holds the platform's own views of other modules' data:
// the figures its dashboards, audit views and public statistics show. The
// composition root converts the owning modules' facade results into them, so
// the shared kernel depends on no other module (ADR-001).
package readmodel

import "time"

// OrdersTotal sums the orders of a period, in minor units.
type OrdersTotal struct {
	AmountTotal        int64
	NewOrderAmount     int64
	RenewalOrderAmount int64
}

// OrdersTotalWithDate is one period of an order breakdown.
type OrdersTotalWithDate struct {
	Date               string
	AmountTotal        int64
	NewOrderAmount     int64
	RenewalOrderAmount int64
}

// UserStatisticsWithDate is one period of the registration and paying-user
// breakdown.
type UserStatisticsWithDate struct {
	Date              string
	Register          int64
	NewOrderUsers     int64
	RenewalOrderUsers int64
}

// Server is a server as the traffic rankings name it.
type Server struct {
	Id   int64
	Name string
}

// TotalTraffic sums the traffic of a period, in bytes.
type TotalTraffic struct {
	Download int64
	Upload   int64
}

// ServerTrafficRanking is one server's traffic in a ranking.
type ServerTrafficRanking struct {
	ServerId int64
	Download int64
	Upload   int64
	Total    int64
}

// UserTrafficRanking is one subscription's traffic in a ranking.
type UserTrafficRanking struct {
	UserId      int64
	SubscribeId int64
	Download    int64
	Upload      int64
	Total       int64
}

// TrafficLogDetailsFilter selects one page of the traffic log entries logged
// in [Start, End), a zero bound leaving that side open, for the non-zero ids.
type TrafficLogDetailsFilter struct {
	ServerId    int64
	UserId      int64
	SubscribeId int64
	Start       time.Time
	End         time.Time
	Page        int
	Size        int
}

// TrafficLog is one traffic log entry.
type TrafficLog struct {
	Id          int64
	ServerId    int64
	UserId      int64
	SubscribeId int64
	Download    int64
	Upload      int64
	Timestamp   time.Time
}

// AuthMethod is a configured authentication method.
type AuthMethod struct {
	Method  string
	Config  string
	Enabled bool
}
