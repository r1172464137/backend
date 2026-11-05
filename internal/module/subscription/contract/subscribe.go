// Package dto is the subscription module's contract: the commands, queries
// and results of its facade and HTTP handlers, and the module's own JSON
// snapshots of the identity and network data its views embed.
package dto

type (
	// SubscribeRequest names the subscription to deliver and the query
	// parameters its template receives. The client application, and with it
	// the format, is chosen by the request's user agent.
	SubscribeRequest struct {
		Token  string
		Params map[string]string
	}
	SubscribeResponse struct {
		Config  []byte
		Header  string
		Headers map[string]string
	}
)

type BatchDeleteSubscribeGroupRequest struct {
	Ids []int64 `json:"ids" validate:"required"`
}

type BatchDeleteSubscribeRequest struct {
	Ids []int64 `json:"ids" validate:"required"`
}

type CreateSubscribeGroupRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type CreateSubscribeRequest struct {
	Name              string              `json:"name" validate:"required"`
	Language          string              `json:"language"`
	Description       string              `json:"description"`
	UnitPrice         int64               `json:"unit_price"`
	UnitTime          string              `json:"unit_time"`
	Discount          []SubscribeDiscount `json:"discount"`
	Replacement       int64               `json:"replacement"`
	Inventory         int64               `json:"inventory"`
	Traffic           int64               `json:"traffic"`
	SpeedLimit        int64               `json:"speed_limit"`
	DeviceLimit       int64               `json:"device_limit"`
	Quota             int64               `json:"quota"`
	Nodes             StringInt64Slice    `json:"nodes"`
	NodeTags          []string            `json:"node_tags"`
	Show              *bool               `json:"show"`
	Sell              *bool               `json:"sell"`
	DeductionRatio    int64               `json:"deduction_ratio"`
	AllowDeduction    *bool               `json:"allow_deduction"`
	ResetCycle        int64               `json:"reset_cycle"`
	RenewalReset      *bool               `json:"renewal_reset"`
	ShowOriginalPrice bool                `json:"show_original_price"`
}

type DeleteSubscribeGroupRequest struct {
	Id int64 `json:"id" validate:"required"`
}

type DeleteSubscribeRequest struct {
	Id int64 `json:"id" validate:"required"`
}

type GetSubscribeDetailsRequest struct {
	Id int64 `form:"id" validate:"required"`
}

type GetSubscribeGroupListResponse struct {
	List  []SubscribeGroup `json:"list"`
	Total int64            `json:"total"`
}

type GetSubscribeListRequest struct {
	Page     int64  `form:"page" validate:"required,gt=0"`
	Size     int64  `form:"size" validate:"required,gt=0,lte=100"`
	Language string `form:"language,omitempty"`
	Search   string `form:"search,omitempty"`
}

type GetSubscribeListResponse struct {
	List  []SubscribeItem `json:"list"`
	Total int64           `json:"total"`
}

type GetSubscribeLogRequest struct {
	Page int `form:"page" validate:"required,gt=0"`
	Size int `form:"size" validate:"required,gt=0,lte=100"`
}

type GetSubscribeLogResponse struct {
	List  []UserSubscribeLog `json:"list"`
	Total int64              `json:"total"`
}

type PreUnsubscribeRequest struct {
	Id int64 `json:"id"`
}

type PreUnsubscribeResponse struct {
	DeductionAmount int64 `json:"deduction_amount"`
}

type QuerySubscribeGroupListResponse struct {
	List  []SubscribeGroup `json:"list"`
	Total int64            `json:"total"`
}

type QuerySubscribeListRequest struct {
	Language string `form:"language"`
}

type QuerySubscribeListResponse struct {
	List  []Subscribe `json:"list"`
	Total int64       `json:"total"`
}

type ResetAllSubscribeTokenResponse struct {
	Success bool `json:"success"`
}

type Subscribe struct {
	Id                int64               `json:"id"`
	Name              string              `json:"name"`
	Language          string              `json:"language"`
	Description       string              `json:"description"`
	UnitPrice         int64               `json:"unit_price"`
	UnitTime          string              `json:"unit_time"`
	Discount          []SubscribeDiscount `json:"discount"`
	Replacement       int64               `json:"replacement"`
	Inventory         int64               `json:"inventory"`
	Traffic           int64               `json:"traffic"`
	SpeedLimit        int64               `json:"speed_limit"`
	DeviceLimit       int64               `json:"device_limit"`
	Quota             int64               `json:"quota"`
	Nodes             StringInt64Slice    `json:"nodes"`
	NodeTags          []string            `json:"node_tags"`
	Show              bool                `json:"show"`
	Sell              bool                `json:"sell"`
	Sort              int64               `json:"sort"`
	DeductionRatio    int64               `json:"deduction_ratio"`
	AllowDeduction    bool                `json:"allow_deduction"`
	ResetCycle        int64               `json:"reset_cycle"`
	RenewalReset      bool                `json:"renewal_reset"`
	ShowOriginalPrice bool                `json:"show_original_price"`
	CreatedAt         int64               `json:"created_at"`
	UpdatedAt         int64               `json:"updated_at"`
}

type SubscribeGroup struct {
	Id          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type SubscribeItem struct {
	Subscribe
	Sold int64 `json:"sold"`
}

type UnsubscribeRequest struct {
	Id int64 `json:"id"`
}

type UpdateSubscribeGroupRequest struct {
	Id          int64  `json:"id" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type UpdateSubscribeRequest struct {
	Id                int64               `json:"id" validate:"required"`
	Name              string              `json:"name" validate:"required"`
	Language          string              `json:"language"`
	Description       string              `json:"description"`
	UnitPrice         int64               `json:"unit_price"`
	UnitTime          string              `json:"unit_time"`
	Discount          []SubscribeDiscount `json:"discount"`
	Replacement       int64               `json:"replacement"`
	Inventory         int64               `json:"inventory"`
	Traffic           int64               `json:"traffic"`
	SpeedLimit        int64               `json:"speed_limit"`
	DeviceLimit       int64               `json:"device_limit"`
	Quota             int64               `json:"quota"`
	Nodes             StringInt64Slice    `json:"nodes"`
	NodeTags          []string            `json:"node_tags"`
	Show              *bool               `json:"show"`
	Sell              *bool               `json:"sell"`
	Sort              int64               `json:"sort"`
	DeductionRatio    int64               `json:"deduction_ratio"`
	AllowDeduction    *bool               `json:"allow_deduction"`
	ResetCycle        int64               `json:"reset_cycle"`
	RenewalReset      *bool               `json:"renewal_reset"`
	ShowOriginalPrice bool                `json:"show_original_price"`
}
