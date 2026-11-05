package dto

type CreateSubscribeApplicationRequest struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	Icon              string `json:"icon,omitempty"`
	Scheme            string `json:"scheme,omitempty"`
	UserAgent         string `json:"user_agent"`
	IsDefault         bool   `json:"is_default"`
	SubscribeTemplate string `json:"template"`
	OutputFormat      string `json:"output_format"`
	// DefaultParams holds the template params this client should receive when the
	// subscription URL does not carry them, in query-string form such as
	// "mode=rule&emoji=1".
	DefaultParams string       `json:"default_params,omitempty"`
	DownloadLink  DownloadLink `json:"download_link"`
}

type DeleteSubscribeApplicationRequest struct {
	Id int64 `json:"id"`
}

type DownloadLink struct {
	IOS     string `json:"ios,omitempty"`
	Android string `json:"android,omitempty"`
	Windows string `json:"windows,omitempty"`
	Mac     string `json:"mac,omitempty"`
	Linux   string `json:"linux,omitempty"`
	Harmony string `json:"harmony,omitempty"`
}

type GetSubscribeApplicationListRequest struct {
	Page int `form:"page" validate:"required,gt=0"`
	Size int `form:"size" validate:"required,gt=0,lte=100"`
}

type GetSubscribeApplicationListResponse struct {
	Total int64                  `json:"total"`
	List  []SubscribeApplication `json:"list"`
}

type PreviewSubscribeTemplateRequest struct {
	Id int64 `form:"id"`
}

type PreviewSubscribeTemplateResponse struct {
	Template string `json:"template"` // The rendered template preview
}

type SubscribeApplication struct {
	Id                int64        `json:"id"`
	Name              string       `json:"name"`
	Description       string       `json:"description,omitempty"`
	Icon              string       `json:"icon,omitempty"`
	Scheme            string       `json:"scheme,omitempty"`
	UserAgent         string       `json:"user_agent"`
	IsDefault         bool         `json:"is_default"`
	SubscribeTemplate string       `json:"template"`
	OutputFormat      string       `json:"output_format"`
	DefaultParams     string       `json:"default_params,omitempty"`
	DownloadLink      DownloadLink `json:"download_link,omitempty"`
	CreatedAt         int64        `json:"created_at"`
	UpdatedAt         int64        `json:"updated_at"`
}

type UpdateSubscribeApplicationRequest struct {
	Id                int64        `json:"id"`
	Name              string       `json:"name"`
	Description       string       `json:"description,omitempty"`
	Icon              string       `json:"icon,omitempty"`
	Scheme            string       `json:"scheme,omitempty"`
	UserAgent         string       `json:"user_agent"`
	IsDefault         bool         `json:"is_default"`
	SubscribeTemplate string       `json:"template"`
	OutputFormat      string       `json:"output_format"`
	DefaultParams     string       `json:"default_params,omitempty"`
	DownloadLink      DownloadLink `json:"download_link,omitempty"`
}
