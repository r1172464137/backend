package dto

type CreateNodeRequest struct {
	Name     string   `json:"name"`
	Tags     []string `json:"tags,omitempty"`
	Port     uint16   `json:"port"`
	Address  string   `json:"address"`
	ServerId int64    `json:"server_id"`
	Protocol string   `json:"protocol"`
	Enabled  *bool    `json:"enabled"`
}

type DeleteNodeRequest struct {
	Id int64 `json:"id"`
}

type FilterNodeListRequest struct {
	Page   int    `form:"page" validate:"required,gt=0"`
	Size   int    `form:"size" validate:"required,gt=0,lte=100"`
	Search string `form:"search,omitempty"`
}

type FilterNodeListResponse struct {
	Total int64  `json:"total"`
	List  []Node `json:"list"`
}

type Node struct {
	Id        int64    `json:"id"`
	Name      string   `json:"name"`
	Tags      []string `json:"tags"`
	Port      uint16   `json:"port"`
	Address   string   `json:"address"`
	ServerId  int64    `json:"server_id"`
	Protocol  string   `json:"protocol"`
	Enabled   *bool    `json:"enabled"`
	Sort      int      `json:"sort,omitempty"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}

type NodeDNS struct {
	Proto      string   `json:"proto"`
	Address    string   `json:"address"`
	ServerName string   `json:"server_name,omitempty"`
	Domains    []string `json:"domains"`
}

type NodeOutbound struct {
	ID                   string   `json:"id,omitempty"`
	Name                 string   `json:"name"`
	Protocol             string   `json:"protocol"`
	Address              string   `json:"address"`
	Port                 int64    `json:"port"`
	User                 string   `json:"user,omitempty"`
	Password             string   `json:"password"`
	UUID                 string   `json:"uuid,omitempty"`
	Cipher               string   `json:"cipher,omitempty"`
	Plugin               string   `json:"plugin,omitempty"`
	PluginOptions        any      `json:"plugin_opts,omitempty"`
	Security             string   `json:"security,omitempty"`
	SNI                  string   `json:"sni,omitempty"`
	ALPN                 []string `json:"alpn,omitempty"`
	AllowInsecure        bool     `json:"allow_insecure,omitempty"`
	Fingerprint          string   `json:"fingerprint,omitempty"`
	Transport            string   `json:"transport,omitempty"`
	Host                 string   `json:"host,omitempty"`
	Path                 string   `json:"path,omitempty"`
	ServiceName          string   `json:"service_name,omitempty"`
	XHTTPMode            string   `json:"xhttp_mode,omitempty"`
	XHTTPExtra           string   `json:"xhttp_extra,omitempty"`
	Flow                 string   `json:"flow,omitempty"`
	Encryption           string   `json:"encryption,omitempty"`
	EncryptionMode       string   `json:"encryption_mode,omitempty"`
	EncryptionRTT        string   `json:"encryption_rtt,omitempty"`
	EncryptionTicket     string   `json:"encryption_ticket,omitempty"`
	EncryptionPadding    string   `json:"encryption_client_padding,omitempty"`
	EncryptionPassword   string   `json:"encryption_password,omitempty"`
	Multiplex            string   `json:"multiplex,omitempty"`
	UoT                  bool     `json:"uot,omitempty"`
	UoTVersion           int      `json:"uot_version,omitempty"`
	CongestionController string   `json:"congestion_controller,omitempty"`
	UDPStream            bool     `json:"udp_stream,omitempty"`
	ReduceRtt            bool     `json:"reduce_rtt,omitempty"`
	Heartbeat            int      `json:"heartbeat,omitempty"`
	RealityPublicKey     string   `json:"reality_public_key,omitempty"`
	RealityShortId       string   `json:"reality_short_id,omitempty"`
	SpiderX              string   `json:"spider_x,omitempty"`
	Settings             string   `json:"settings,omitempty"`
	StreamSettings       string   `json:"stream_settings,omitempty"`
	// Legacy match arrays are retained for protobuf compatibility and must be empty in v1.
	Rules []string `json:"rules"`
	// Must be empty in v1.
	IPRules []string `json:"ip_rules"`
	// Must be empty in v1.
	ProtocolRules []string `json:"protocol_rules"`
}

type QueryNodeTagResponse struct {
	Tags []string `json:"tags"`
}

type ResetSortRequest struct {
	Sort []NetworkSortItem `json:"sort"`
}

type NetworkSortItem struct {
	Id   int64 `json:"id" validate:"required"`
	Sort int64 `json:"sort" validate:"required"`
} // @name dto.SortItem
type ToggleNodeStatusRequest struct {
	Id     int64 `json:"id"`
	Enable *bool `json:"enable"`
}

type UpdateNodeRequest struct {
	Id       int64    `json:"id"`
	Name     string   `json:"name"`
	Tags     []string `json:"tags,omitempty"`
	Port     uint16   `json:"port"`
	Address  string   `json:"address"`
	ServerId int64    `json:"server_id"`
	Protocol string   `json:"protocol"`
	Enabled  *bool    `json:"enabled"`
}
