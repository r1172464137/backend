package dto

type CreateServerRequest struct {
	Name      string     `json:"name"`
	Country   string     `json:"country,omitempty"`
	City      string     `json:"city,omitempty"`
	Address   string     `json:"address"`
	Sort      int        `json:"sort,omitempty"`
	Protocols []Protocol `json:"protocols"`
}

type DeleteServerRequest struct {
	Id int64 `json:"id"`
}

type FilterServerListRequest struct {
	Page   int    `form:"page" validate:"required,gt=0"`
	Size   int    `form:"size" validate:"required,gt=0,lte=100"`
	Search string `form:"search,omitempty"`
}

type FilterServerListResponse struct {
	Total int64    `json:"total"`
	List  []Server `json:"list"`
}

type GetServerConfigRequest struct {
	ServerCommon
}

type GetServerConfigResponse struct {
	Basic    ServerBasic `json:"basic"`
	Protocol string      `json:"protocol"`
	Config   interface{} `json:"config"`
}

type GetServerProtocolsRequest struct {
	Id int64 `form:"id"`
}

type GetServerProtocolsResponse struct {
	Protocols []Protocol `json:"protocols"`
}

type GetServerUserListRequest struct {
	ServerCommon
}

type GetServerUserListResponse struct {
	Users []ServerUser `json:"users"`
}

type ServerNodeConfigValues struct {
	IPStrategy string         `json:"ip_strategy"`
	DNS        []NodeDNS      `json:"dns"`
	Block      []string       `json:"block"`
	Outbound   []NodeOutbound `json:"outbound"`
}

type ServerNodeConfigOverride struct {
	InheritIPStrategy bool   `json:"inherit_ip_strategy"`
	IPStrategy        string `json:"ip_strategy"`

	InheritDNS bool      `json:"inherit_dns"`
	DNS        []NodeDNS `json:"dns"`

	InheritBlock bool     `json:"inherit_block"`
	Block        []string `json:"block"`

	InheritOutbound bool           `json:"inherit_outbound"`
	Outbound        []NodeOutbound `json:"outbound"`
}

type OnlineUser struct {
	SID int64  `json:"uid"`
	IP  string `json:"ip"`
}

type OnlineUsersRequest struct {
	ServerCommon
	Users []OnlineUser `json:"users"`
}

type Protocol struct {
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
	// REALITY target-side PROXY protocol version: 0 (default), 1 or 2; independent of reception.
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
}

type QueryServerConfigRequest struct {
	ServerID  int64    `path:"server_id"`
	SecretKey string   `form:"secret_key"`
	Protocols []string `form:"protocols,omitempty"`
}

type QueryServerConfigResponse struct {
	TrafficReportThreshold int64          `json:"traffic_report_threshold"`
	PushInterval           int64          `json:"push_interval"`
	PullInterval           int64          `json:"pull_interval"`
	IPStrategy             string         `json:"ip_strategy"`
	DNS                    []NodeDNS      `json:"dns"`
	Block                  []string       `json:"block"`
	Outbound               []NodeOutbound `json:"outbound"`
	Protocols              []Protocol     `json:"protocols"`
	Total                  int64          `json:"total"`
}

type GetServerNodeConfigRequest struct {
	ServerID int64 `form:"server_id" validate:"required"`
}

type GetServerNodeConfigResponse struct {
	Global    ServerNodeConfigValues   `json:"global"`
	Override  ServerNodeConfigOverride `json:"override"`
	Effective ServerNodeConfigValues   `json:"effective"`
}

type UpdateServerNodeConfigRequest struct {
	ServerID int64 `json:"server_id" validate:"required"`
	ServerNodeConfigOverride
}

type Server struct {
	Id             int64        `json:"id"`
	Name           string       `json:"name"`
	Country        string       `json:"country"`
	City           string       `json:"city"`
	Address        string       `json:"address"`
	Sort           int          `json:"sort"`
	Protocols      []Protocol   `json:"protocols"`
	LastReportedAt int64        `json:"last_reported_at"`
	Status         ServerStatus `json:"status"`
	CreatedAt      int64        `json:"created_at"`
	UpdatedAt      int64        `json:"updated_at"`
}

type ServerBasic struct {
	PushInterval int64 `json:"push_interval"`
	PullInterval int64 `json:"pull_interval"`
}

type ServerCommon struct {
	Protocol  string `form:"protocol"`
	ServerId  int64  `form:"server_id"`
	SecretKey string `form:"secret_key"`
}

type ServerOnlineIP struct {
	IP       string `json:"ip"`
	Protocol string `json:"protocol"`
}

type ServerOnlineUser struct {
	IP          []ServerOnlineIP `json:"ip"`
	UserId      int64            `json:"user_id"`
	Subscribe   string           `json:"subscribe"`
	SubscribeId int64            `json:"subscribe_id"`
	Traffic     int64            `json:"traffic"`
	ExpiredAt   int64            `json:"expired_at"`
}

type ServerPushStatusRequest struct {
	ServerCommon
	Cpu       float64 `json:"cpu"`
	Mem       float64 `json:"mem"`
	Disk      float64 `json:"disk"`
	UpdatedAt int64   `json:"updated_at"`
	// CertPinSHA256 is transport metadata carried by the
	// X-Node-Certificate-SHA256 request header, not part of the body.
	CertPinSHA256 string `json:"-"`
}

type ServerStatus struct {
	Cpu      float64            `json:"cpu"`
	Mem      float64            `json:"mem"`
	Disk     float64            `json:"disk"`
	Protocol string             `json:"protocol"`
	Online   []ServerOnlineUser `json:"online"`
	Status   string             `json:"status"`
}

type ServerUser struct {
	Id          int64  `json:"id"`
	UUID        string `json:"uuid"`
	SpeedLimit  int64  `json:"speed_limit"`
	DeviceLimit int64  `json:"device_limit"`
}

type UpdateServerRequest struct {
	Id                int64                 `json:"id"`
	Name              string                `json:"name"`
	Country           string                `json:"country,omitempty"`
	City              string                `json:"city,omitempty"`
	Address           string                `json:"address"`
	Sort              int                   `json:"sort,omitempty"`
	Protocols         []Protocol            `json:"protocols"`
	ProtocolFieldSets []map[string]struct{} `json:"-"`
}
