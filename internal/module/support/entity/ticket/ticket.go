// Package ticket holds the support ticket rows (the ticket and ticket_follow
// tables), the ticket statuses and the follow types.
package ticket

import (
	"errors"
	"strings"
	"time"
)

const (
	Pending   = 1 // Pending  # Pending follow up
	Waiting   = 2 // Waiting  # Waiting for user response
	Processed = 3 // Processed
	Closed    = 4 // Closed
)

// ErrClosed reports a staff reply to a closed ticket. Closing ends the
// conversation for staff: a closed ticket takes no staff reply until it is
// explicitly reopened. The owner's reply is the exception and reopens it.
var ErrClosed = errors.New("ticket is closed")

// Follow types.
const (
	FollowText  = 1
	FollowImage = 2 // Content is an image reference rendered as <img src>
)

// FromUser marks a follow written by the ticket's owner; the user client has
// always sent this value. Staff replies carry anything else ("System" from
// the admin panel, "admin" from the Telegram bot).
const FromUser = "User"

// IsFromUser reports whether a follow was written by the ticket's owner.
// Older rows and callers spell the marker in lowercase or leave it empty.
func IsFromUser(from string) bool {
	return from == "" || strings.EqualFold(from, FromUser)
}

type Ticket struct {
	Id          int64     `gorm:"primaryKey"`
	Title       string    `gorm:"type:varchar(255);not null;default:'';comment:Title"`
	Description string    `gorm:"type:text;comment:Description"`
	UserId      int64     `gorm:"type:bigint;not null;default:0;comment:UserId"`
	Status      uint8     `gorm:"type:tinyint(1);not null;default:1;comment:Status"`
	CreatedAt   time.Time `gorm:"<-:create;comment:Create Time"`
	UpdatedAt   time.Time `gorm:"comment:Update Time"`
}

func (Ticket) TableName() string {
	return "ticket"
}

type Follow struct {
	Id        int64     `gorm:"primaryKey"`
	TicketId  int64     `gorm:"type:bigint;not null;default:0;comment:TicketId"`
	From      string    `gorm:"type:varchar(255);not null;default:'';comment:From"`
	Type      uint8     `gorm:"type:tinyint(1);not null;default:1;comment:Type: 1 text, 2 image"`
	Content   string    `gorm:"type:text;comment:Content"`
	CreatedAt time.Time `gorm:"<-:create;comment:Create Time"`
}

func (Follow) TableName() string {
	return "ticket_follow"
}

// Details is a ticket with its follows preloaded, the view of the ticket
// detail pages; it is a query result, not a table of its own.
type Details struct {
	Id          int64     `gorm:"primaryKey"`
	Title       string    `gorm:"type:varchar(255);not null;default:'';comment:Title"`
	Description string    `gorm:"type:text;comment:Description"`
	UserId      int64     `gorm:"type:bigint;not null;default:0;comment:UserId"`
	Status      uint8     `gorm:"type:tinyint(1);not null;default:1;comment:Status"`
	Follows     []Follow  `gorm:"foreignKey:TicketId;references:Id"`
	CreatedAt   time.Time `gorm:"<-:create;comment:Create Time"`
	UpdatedAt   time.Time `gorm:"comment:Update Time"`
}
