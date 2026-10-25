package dto

// Cross-domain view types are module-owned snapshots. They deliberately
// duplicate JSON shapes instead of coupling one module's API to another.
type AuthConfig struct {
	Mobile   MobileAuthenticateConfig `json:"mobile"`
	Email    EmailAuthticateConfig    `json:"email"`
	Device   DeviceAuthticateConfig   `json:"device"`
	Register PubilcRegisterConfig     `json:"register"`
}

type DeviceAuthticateConfig struct {
	Enable         bool `json:"enable"`
	ShowAds        bool `json:"show_ads"`
	EnableSecurity bool `json:"enable_security"`
	OnlyRealDevice bool `json:"only_real_device"` // Requires authenticated device transport, not hardware attestation.
}

type PlatformDownloadLinkSnapshot struct {
	IOS     string `json:"ios,omitempty"`
	Android string `json:"android,omitempty"`
	Windows string `json:"windows,omitempty"`
	Mac     string `json:"mac,omitempty"`
	Linux   string `json:"linux,omitempty"`
	Harmony string `json:"harmony,omitempty"`
} // @name dto.DownloadLink

type EmailAuthticateConfig struct {
	Enable             bool   `json:"enable"`
	EnableVerify       bool   `json:"enable_verify"`
	EnableDomainSuffix bool   `json:"enable_domain_suffix"`
	DomainSuffixList   string `json:"domain_suffix_list"`
}

type FilterServerTrafficLogRequest struct {
	FilterLogParams
	ServerId int64 `form:"server_id,optional"`
}

type FilterServerTrafficLogResponse struct {
	Total int64              `json:"total"`
	List  []ServerTrafficLog `json:"list"`
}

type FilterSubscribeTrafficRequest struct {
	FilterLogParams
	UserId          int64 `form:"user_id,optional"`
	UserSubscribeId int64 `form:"user_subscribe_id,optional"`
}

type FilterSubscribeTrafficResponse struct {
	Total int64                     `json:"total"`
	List  []UserSubscribeTrafficLog `json:"list"`
}

type FilterTrafficLogDetailsRequest struct {
	FilterLogParams
	ServerId    int64 `form:"server_id,optional"`
	SubscribeId int64 `form:"subscribe_id,optional"`
	UserId      int64 `form:"user_id,optional"`
}

type FilterTrafficLogDetailsResponse struct {
	Total int64               `json:"total"`
	List  []TrafficLogDetails `json:"list"`
}

type GetNodeMultiplierResponse struct {
	Periods []TimePeriod `json:"periods"`
}

type GetSubscribeClientResponse struct {
	Total int64             `json:"total"`
	List  []SubscribeClient `json:"list"`
}

type HeartbeatResponse struct {
	Status    bool   `json:"status"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

type MobileAuthenticateConfig struct {
	Enable          bool     `json:"enable"`
	EnableWhitelist bool     `json:"enable_whitelist"`
	Whitelist       []string `json:"whitelist"`
}

type NodeConfig struct {
	NodeSecret             string                         `json:"node_secret"`
	NodePullInterval       int64                          `json:"node_pull_interval"`
	NodePushInterval       int64                          `json:"node_push_interval"`
	TrafficReportThreshold int64                          `json:"traffic_report_threshold"`
	IPStrategy             string                         `json:"ip_strategy"`
	DNS                    []PlatformNodeDNSSnapshot      `json:"dns"`
	Block                  []string                       `json:"block"`
	Outbound               []PlatformNodeOutboundSnapshot `json:"outbound"`
}

type PlatformNodeDNSSnapshot struct {
	Proto      string   `json:"proto"`
	Address    string   `json:"address"`
	ServerName string   `json:"server_name,omitempty"`
	Domains    []string `json:"domains"`
} // @name dto.NodeDNS

type PlatformNodeOutboundSnapshot struct {
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
	Rules                []string `json:"rules"`
} // @name dto.NodeOutbound

type OrdersStatistics struct {
	Date               string             `json:"date,omitempty"`
	AmountTotal        int64              `json:"amount_total"`
	NewOrderAmount     int64              `json:"new_order_amount"`
	RenewalOrderAmount int64              `json:"renewal_order_amount"`
	List               []OrdersStatistics `json:"list,omitempty"`
}

type PreViewNodeMultiplierResponse struct {
	CurrentTime string  `json:"current_time"`
	Ratio       float32 `json:"ratio"`
}

type PlatformProtocolSnapshot struct {
	// 通用字段：协议类型标识，例如 shadowsocks、vless、vmess、hysteria2、tuic、nowhere。
	Type string `json:"type"`
	// 通用字段：入站监听端口。
	Port uint16 `json:"port"`
	// 版本字段：Snell 接受 5/6，TUIC 接受 5，Nowhere 接受 1；0 表示使用协议默认值。
	Version int `json:"version,omitempty"`
	// Snell 专属字段：Snell v6 的工作模式；其他协议不应设置。
	Mode string `json:"mode,omitempty"`
	// 通用字段：是否启用该入站协议。
	Enable bool `json:"enable"`
	// TLS/REALITY 协议通用字段：选择 none、tls 或 reality；实际可选值由具体协议限制。
	Security string `json:"security,omitempty"`
	// 监听网络通用字段：选择 tcp、udp 或 both；Nowhere 规范化为 mix、tcp 或 udp，其他协议按各自能力校验。
	Network string `json:"network,omitempty"`
	// TLS 协议通用字段：证书域名及 TLS ServerName；REALITY 也用它作为服务端名称。
	SNI string `json:"sni,omitempty"`
	// TLS/HTTP/QUIC 协议通用字段：TLS ALPN 列表；Nowhere 必须且只能设置一个值，默认 now/1。
	ALPN []string `json:"alpn,omitempty"`
	// TLS 客户端兼容字段：允许跳过证书校验；入站配置通常不消费，不能作为服务端证书配置使用。
	AllowInsecure bool `json:"allow_insecure,omitempty"`
	// TLS 客户端兼容字段：uTLS 指纹；当前节点入站不消费，仅为出站/旧配置兼容保留。
	Fingerprint string `json:"fingerprint,omitempty"`
	// VLESS/VMess REALITY 专属字段：REALITY 握手转发目标地址。
	RealityServerAddr string `json:"reality_server_addr,omitempty"`
	// VLESS/VMess REALITY 专属字段：REALITY 握手转发目标端口。
	RealityServerPort int `json:"reality_server_port,omitempty"`
	// REALITY target-side PROXY protocol version: 0 (default), 1 or 2.
	RealityXver int `json:"reality_xver,omitempty"`
	// VLESS/VMess REALITY 服务端专属字段：服务端 X25519 私钥。
	RealityPrivateKey string `json:"reality_private_key,omitempty"`
	// VLESS/VMess REALITY 客户端信息字段：由私钥对应的公钥，主要用于订阅输出。
	RealityPublicKey string `json:"reality_public_key,omitempty"`
	// VLESS/VMess REALITY 专属字段：允许客户端使用的 short ID。
	RealityShortId string `json:"reality_short_id,omitempty"`
	// VLESS/VMess/Trojan 通用传输字段：tcp、ws、httpupgrade、grpc 或 xhttp。
	Transport string `json:"transport,omitempty"`
	// VLESS/VMess/Trojan 传输字段：WebSocket、HTTPUpgrade 或 XHTTP 的 Host。
	Host string `json:"host,omitempty"`
	// VLESS/VMess/Trojan 传输字段：WebSocket、HTTPUpgrade 或 XHTTP 的请求路径。
	Path string `json:"path,omitempty"`
	// VLESS/VMess/Trojan gRPC 传输专属字段：gRPC service name。
	ServiceName string `json:"service_name,omitempty"`
	// Shadowsocks/SSR 共用字段：Shadowsocks method 或 SSR cipher。
	Cipher string `json:"cipher,omitempty"`
	// 密钥型协议共用字段：Shadowsocks 2022 服务端密钥、SSR 密码或 Snell PSK。
	ServerKey string `json:"server_key,omitempty"`
	// Shadowsocks（AEAD/2022）专属字段：入站插件名，如 obfs、v2ray-plugin、shadow-tls、restls。
	Plugin string `json:"plugin,omitempty"`
	// Shadowsocks（AEAD/2022）专属字段：所选入站插件的结构化参数。
	PluginOptions any `json:"plugin_opts,omitempty"`
	// VLESS 专属字段：XTLS Vision 流控模式，当前有效值为 xtls-rprx-vision。
	Flow string `json:"flow,omitempty"`
	// 协议无关能力字段：UDP over TCP 开关，供支持 UoT 的协议使用，并非某一协议专属。
	UoT bool `json:"uot,omitempty"`
	// 协议无关能力字段：UoT 协议版本，当前支持 1 或 2；0 表示使用默认版本。
	UoTVersion int `json:"uot_version,omitempty"`
	// 监听器通用兼容字段：是否接收 PROXY protocol；当前节点入站尚未统一启用。
	AcceptProxyProtocol bool `json:"accept_proxy_protocol,omitempty"`
	// Hysteria2/TUIC 类 QUIC 协议字段：端口跳跃范围；当前节点入站尚未启用该能力。
	HopPorts string `json:"hop_ports,omitempty"`
	// Hysteria2/TUIC 类 QUIC 协议字段：端口跳跃时间间隔；当前节点入站尚未启用该能力。
	HopInterval int `json:"hop_interval,omitempty"`
	// Hysteria2 专属字段：Salamander 混淆密码，仅在 obfs=salamander 时使用。
	ObfsPassword string `json:"obfs_password,omitempty"`
	// TLS 客户端兼容字段：禁用 SNI；当前服务端入站不消费。
	DisableSNI bool `json:"disable_sni,omitempty"`
	// TUIC 专属字段：启用 QUIC 0-RTT，以减少首次握手往返。
	ReduceRtt bool `json:"reduce_rtt,omitempty"`
	// TUIC 专属字段：连接心跳间隔，单位为秒；0 使用节点默认值。
	Heartbeat int `json:"heartbeat,omitempty"`
	// TUIC/Hysteria 兼容字段：旧实现的 UDP relay 模式；当前节点入站不消费。
	UDPRelayMode string `json:"udp_relay_mode,omitempty"`
	// QUIC 协议共用字段：TUIC 的主拥塞控制字段，也是 Naive 的旧字段别名。
	CongestionController string `json:"congestion_controller,omitempty"`
	// QUIC 协议共用字段：Naive 的主拥塞控制字段，也是 TUIC 的兼容别名。
	QUICCongestionControl string `json:"quic_congestion_control,omitempty"`
	// 协议无关能力字段：多路复用级别（off、low、medium、high），适用于支持 mux 的流协议。
	Multiplex string `json:"multiplex,omitempty"`
	// AnyTLS 专属字段：TLS record padding 方案。
	PaddingScheme string `json:"padding_scheme,omitempty"`
	// Mieru 专属字段：流量形态/包长分布配置。
	TrafficPattern string `json:"traffic_pattern,omitempty"`
	// Mieru 专属字段：是否强制客户端携带可识别用户的 user hint。
	UserHintIsMandatory bool `json:"user_hint_is_mandatory,omitempty"`
	// Hysteria2 专属字段：服务端上行带宽参数，单位为 Mbps。
	UpMbps int `json:"up_mbps,omitempty"`
	// Hysteria2 专属字段：服务端下行带宽参数，单位为 Mbps。
	DownMbps int `json:"down_mbps,omitempty"`
	// 混淆协议共用字段：Hysteria2 的 Salamander、Snell v5 的 obfs、SSR 的 obfs 方法。
	Obfs string `json:"obfs,omitempty"`
	// SSR 专属字段：SSR protocol 方法；JSON 名称 protocol 与顶层 type 不同。
	SSRProtocol string `json:"protocol,omitempty"`
	// SSR 专属字段：SSR protocol_param。
	ProtocolParam string `json:"protocol_param,omitempty"`
	// SSR 专属字段：SSR obfs_param。
	ObfsParam string `json:"obfs_param,omitempty"`
	// 旧混淆实现兼容字段：混淆目标 Host；当前节点入站不消费，Shadowsocks 插件应使用 plugin_opts。
	ObfsHost string `json:"obfs_host,omitempty"`
	// 旧混淆实现兼容字段：混淆请求路径；当前节点入站不消费，Shadowsocks 插件应使用 plugin_opts。
	ObfsPath string `json:"obfs_path,omitempty"`
	// VLESS/VMess/Trojan XHTTP 传输专属字段：XHTTP 工作模式，如 auto、packet-up、stream-up。
	XhttpMode string `json:"xhttp_mode,omitempty"`
	// VLESS/VMess/Trojan XHTTP 传输专属字段：XHTTP 扩展路径/参数。
	XhttpExtra string `json:"xhttp_extra,omitempty"`
	// VLESS Encryption 专属字段：加密套件，如 none、mlkem768x25519plus。
	Encryption string `json:"encryption,omitempty"`
	// VLESS Encryption 专属字段：密钥封装模式，如 native、xorpub、random。
	EncryptionMode string `json:"encryption_mode,omitempty"`
	// VLESS Encryption 专属字段：握手往返模式，取值 0rtt 或 1rtt。
	EncryptionRtt string `json:"encryption_rtt,omitempty"`
	// VLESS Encryption 服务端专属字段：0-RTT ticket。
	EncryptionTicket string `json:"encryption_ticket,omitempty"`
	// VLESS Encryption 服务端专属字段：服务端方向 padding 规则。
	EncryptionServerPadding string `json:"encryption_server_padding,omitempty"`
	// VLESS Encryption 服务端专属字段：ML-KEM/X25519 私钥材料。
	EncryptionPrivateKey string `json:"encryption_private_key,omitempty"`
	// VLESS Encryption 客户端信息字段：客户端方向 padding 规则，用于订阅输出。
	EncryptionClientPadding string `json:"encryption_client_padding,omitempty"`
	// VLESS Encryption 客户端信息字段：1-RTT/派生认证密码，用于订阅输出。
	EncryptionPassword string `json:"encryption_password,omitempty"`
	// 订阅客户端字段：是否启用 Encrypted ClientHello；节点配置下发时会过滤。
	EchEnable bool `json:"ech_enable,omitempty"`
	// 订阅客户端字段：ECH 外层 ServerName；节点配置下发时会过滤。
	EchServerName string `json:"ech_server_name,omitempty"`
	// 面板通用字段：流量计费倍率，默认值为 1；不参与节点协议握手。
	Ratio float64 `json:"ratio,omitempty"`
	// TLS 协议通用字段：证书来源模式，支持 file、self、http、dns；none 表示不配置证书。
	CertMode string `json:"cert_mode,omitempty"`
	// TLS 协议通用字段：cert_mode=dns 时使用的 DNS 服务商标识。
	CertDNSProvider string `json:"cert_dns_provider,omitempty"`
	// TLS 协议通用字段：cert_mode=dns 时传给 DNS 服务商的环境变量/凭据配置。
	CertDNSEnv string `json:"cert_dns_env,omitempty"`
} // @name dto.Protocol

type PubilcRegisterConfig struct {
	StopRegister            bool  `json:"stop_register"`
	EnableIpRegisterLimit   bool  `json:"enable_ip_register_limit"`
	IpRegisterLimit         int64 `json:"ip_register_limit"`
	IpRegisterLimitDuration int64 `json:"ip_register_limit_duration"`
}

type PubilcVerifyCodeConfig struct {
	VerifyCodeInterval int64 `json:"verify_code_interval"`
}

type QueryIPLocationRequest struct {
	IP string `form:"ip" validate:"required"`
}

type QueryIPLocationResponse struct {
	Country string `json:"country"`
	Region  string `json:"region,omitempty"`
	City    string `json:"city"`
}

type ServerTotalDataResponse struct {
	OnlineUsers                   int64               `json:"online_users"`
	OnlineServers                 int64               `json:"online_servers"`
	OfflineServers                int64               `json:"offline_servers"`
	TodayUpload                   int64               `json:"today_upload"`
	TodayDownload                 int64               `json:"today_download"`
	MonthlyUpload                 int64               `json:"monthly_upload"`
	MonthlyDownload               int64               `json:"monthly_download"`
	UpdatedAt                     int64               `json:"updated_at"`
	ServerTrafficRankingToday     []ServerTrafficData `json:"server_traffic_ranking_today"`
	ServerTrafficRankingYesterday []ServerTrafficData `json:"server_traffic_ranking_yesterday"`
	UserTrafficRankingToday       []UserTrafficData   `json:"user_traffic_ranking_today"`
	UserTrafficRankingYesterday   []UserTrafficData   `json:"user_traffic_ranking_yesterday"`
}

type ServerTrafficData struct {
	ServerId int64  `json:"server_id"`
	Name     string `json:"name"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

type ServerTrafficLog struct {
	ServerId int64  `json:"server_id"` // Server ID
	Upload   int64  `json:"upload"`    // Upload traffic in bytes
	Download int64  `json:"download"`  // Download traffic in bytes
	Total    int64  `json:"total"`     // Total traffic in bytes (Upload + Download)
	Date     string `json:"date"`      // Date in YYYY-MM-DD format
	Details  bool   `json:"details"`   // Whether to show detailed traffic
}

type SetNodeMultiplierRequest struct {
	Periods []TimePeriod `json:"periods"`
}

type SubscribeClient struct {
	Id           int64                        `json:"id"`
	Name         string                       `json:"name"`
	Description  string                       `json:"description,omitempty"`
	Icon         string                       `json:"icon,omitempty"`
	Scheme       string                       `json:"scheme,omitempty"`
	IsDefault    bool                         `json:"is_default"`
	DownloadLink PlatformDownloadLinkSnapshot `json:"download_link,omitempty"`
}

type SubscribeConfig struct {
	SingleModel           bool   `json:"single_model"`
	SubscribePath         string `json:"subscribe_path"`
	SubscribeDomain       string `json:"subscribe_domain"`
	PanDomain             bool   `json:"pan_domain"`
	UserAgentLimit        bool   `json:"user_agent_limit"`
	UserAgentList         string `json:"user_agent_list"`
	ShowTutorial          bool   `json:"show_tutorial"`
	ProfileUpdateInterval int64  `json:"profile_update_interval"`
	ProfileWebPageURL     string `json:"profile_web_page_url"`
}

type SubscribeLog struct {
	UserId           int64  `json:"user_id"`
	Token            string `json:"token"`
	UserAgent        string `json:"user_agent"`
	ClientIP         string `json:"client_ip"`
	UserSubscribeId  int64  `json:"user_subscribe_id"`
	Timestamp        int64  `json:"timestamp"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type TicketWaitRelpyResponse struct {
	Count int64 `json:"count"`
}

type TrafficLogDetails struct {
	Id          int64 `json:"id"`
	ServerId    int64 `json:"server_id"`
	UserId      int64 `json:"user_id"`
	SubscribeId int64 `json:"subscribe_id"`
	Download    int64 `json:"download"`
	Upload      int64 `json:"upload"`
	Timestamp   int64 `json:"timestamp"`
}

type PlatformUserSnapshot struct {
	Id                    int64                            `json:"id"`
	Avatar                string                           `json:"avatar"`
	Balance               int64                            `json:"balance"`
	Commission            int64                            `json:"commission"`
	ReferralPercentage    uint8                            `json:"referral_percentage"`
	OnlyFirstPurchase     bool                             `json:"only_first_purchase"`
	GiftAmount            int64                            `json:"gift_amount"`
	Telegram              int64                            `json:"telegram"`
	ReferCode             string                           `json:"refer_code"`
	RefererId             int64                            `json:"referer_id"`
	Enable                bool                             `json:"enable"`
	IsAdmin               bool                             `json:"is_admin,omitempty"`
	EnableBalanceNotify   bool                             `json:"enable_balance_notify"`
	EnableLoginNotify     bool                             `json:"enable_login_notify"`
	EnableSubscribeNotify bool                             `json:"enable_subscribe_notify"`
	EnableTradeNotify     bool                             `json:"enable_trade_notify"`
	AuthMethods           []PlatformUserAuthMethodSnapshot `json:"auth_methods"`
	UserDevices           []PlatformUserDeviceSnapshot     `json:"user_devices"`
	Rules                 []string                         `json:"rules"`
	CreatedAt             int64                            `json:"created_at"`
	UpdatedAt             int64                            `json:"updated_at"`
	DeletedAt             int64                            `json:"deleted_at,omitempty"`
} // @name dto.User

type PlatformUserAuthMethodSnapshot struct {
	AuthType       string `json:"auth_type"`
	AuthIdentifier string `json:"auth_identifier"`
	Verified       bool   `json:"verified"`
} // @name dto.UserAuthMethod

type PlatformUserDeviceSnapshot struct {
	Id         int64  `json:"id"`
	Ip         string `json:"ip"`
	Identifier string `json:"identifier"`
	UserAgent  string `json:"user_agent"`
	Online     bool   `json:"online"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
} // @name dto.UserDevice

type UserStatistics struct {
	Date              string           `json:"date,omitempty"`
	Register          int64            `json:"register"`
	NewOrderUsers     int64            `json:"new_order_users"`
	RenewalOrderUsers int64            `json:"renewal_order_users"`
	List              []UserStatistics `json:"list,omitempty"`
}

type UserStatisticsResponse struct {
	Today   UserStatistics `json:"today"`
	Monthly UserStatistics `json:"monthly"`
	All     UserStatistics `json:"all"`
}

type UserSubscribeTrafficLog struct {
	SubscribeId int64  `json:"subscribe_id"` // Subscribe ID
	UserId      int64  `json:"user_id"`      // User ID
	Upload      int64  `json:"upload"`       // Upload traffic in bytes
	Download    int64  `json:"download"`     // Download traffic in bytes
	Total       int64  `json:"total"`        // Total traffic in bytes (Upload + Download)
	Date        string `json:"date"`         // Date in YYYY-MM-DD format
	Details     bool   `json:"details"`      // Whether to show detailed traffic
}

type UserTrafficData struct {
	// SID identifies the user_subscribe row the traffic was billed to, UID the
	// user owning it. UID is carried separately so the console can still name
	// the user after the subscription row is gone.
	SID      int64 `json:"sid"`
	UID      int64 `json:"uid"`
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

type VersionResponse struct {
	Version string `json:"version"`
}
