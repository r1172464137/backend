package dto

// The bounds below keep a reply inside its TEXT column and the mirror into
// the Telegram admin group finite: a follow may be an inline image, so its
// content is bounded by the column, a title by its VARCHAR(255) column.
const (
	MaxTicketTitleLength       = 255
	MaxTicketDescriptionLength = 10000
	MaxTicketFollowLength      = 65535
)

type CreateTicketFollowRequest struct {
	TicketId int64  `json:"ticket_id" validate:"required,gt=0"`
	From     string `json:"from" validate:"required,max=255"`
	Type     uint8  `json:"type" validate:"required,oneof=1 2"`
	Content  string `json:"content" validate:"required,max=65535"`
}

type CreateUserTicketFollowRequest struct {
	TicketId int64 `json:"ticket_id" validate:"required,gt=0"`
	// From is ignored: the author is always the ticket owner.
	From    string `json:"from" validate:"max=255"`
	Type    uint8  `json:"type" validate:"oneof=0 1 2"`
	Content string `json:"content" validate:"required,max=65535"`
}

type CreateUserTicketRequest struct {
	Title       string `json:"title" validate:"required,max=255"`
	Description string `json:"description" validate:"max=10000"`
}

type Follow struct {
	Id        int64  `json:"id"`
	TicketId  int64  `json:"ticket_id"`
	From      string `json:"from"`
	Type      uint8  `json:"type"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

type GetTicketListRequest struct {
	Page   int64  `form:"page" validate:"required,gt=0"`
	Size   int64  `form:"size" validate:"required,gt=0,lte=100"`
	UserId int64  `form:"user_id,omitempty"`
	Status *uint8 `form:"status,omitempty"`
	Search string `form:"search,omitempty"`
}

type GetTicketListResponse struct {
	Total int64    `json:"total"`
	List  []Ticket `json:"list"`
}

type GetTicketRequest struct {
	Id int64 `form:"id" validate:"required"`
}

type GetUserTicketDetailRequest struct {
	Id int64 `form:"id" validate:"required"`
}

type GetUserTicketListRequest struct {
	Page   int    `form:"page" validate:"required,gt=0"`
	Size   int    `form:"size" validate:"required,gt=0,lte=100"`
	Status *uint8 `form:"status,omitempty"`
	Search string `form:"search,omitempty"`
}

type GetUserTicketListResponse struct {
	Total int64    `json:"total"`
	List  []Ticket `json:"list"`
}

type Ticket struct {
	Id          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	UserId      int64    `json:"user_id"`
	Follows     []Follow `json:"follow,omitempty"`
	Status      uint8    `json:"status"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

type UpdateTicketStatusRequest struct {
	Id     int64  `json:"id" validate:"required"`
	Status *uint8 `json:"status" validate:"required"`
}

type UpdateUserTicketStatusRequest struct {
	Id     int64  `json:"id" validate:"required"`
	Status *uint8 `json:"status" validate:"required"`
}

// StaffTicketUpdateCommand is an internal, trusted contract for a ticket
// change staff make outside the admin panel: today the Telegram bot's /rp,
// /close and /reopen commands and its ticket topics. It must NOT be bound to
// an HTTP request; the caller has already authenticated the administrator.
type StaffTicketUpdateCommand struct {
	TicketId int64
	// Reply, when not empty, is appended as a staff text follow and moves the
	// ticket to Waiting, exactly like a reply from the admin panel.
	Reply string
	// From is the reply's author as stored on the follow.
	From string
	// Status is the status to move the ticket to when there is no reply.
	Status uint8
	// FromMirror reports that the change was made inside the channel the
	// ticket notifier mirrors to (the ticket's Telegram topic). That channel
	// already shows it, so the notifier is skipped instead of echoing the
	// change back.
	FromMirror bool
}

// StaffTicketUpdateResult describes the ticket before the update applied.
type StaffTicketUpdateResult struct {
	PreviousStatus uint8
}
