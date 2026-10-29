package config

import (
	"encoding/json"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/perfect-panel/server/pkg/trace"
)

type Config struct {
	Model         string              `yaml:"Model" default:"prod"`
	Host          string              `yaml:"Host" default:"0.0.0.0"`
	Port          int                 `yaml:"Port" default:"8080"`
	Debug         bool                `yaml:"Debug" default:"false"`
	AppLocation   string              `yaml:"AppLocation" default:"Asia/Shanghai"`
	Transport     TransportConfig     `yaml:"Transport"`
	TLS           TLS                 `yaml:"TLS"`
	JwtAuth       JwtAuth             `yaml:"JwtAuth"`
	Logger        logger.LogConf      `yaml:"Logger"`
	Trace         trace.Config        `yaml:"Trace"`
	Database      orm.Config          `yaml:"Database"`
	MySQL         *orm.Config         `yaml:"MySQL,omitempty"` // Deprecated: use Database.
	Redis         RedisConfig         `yaml:"Redis"`
	Site          SiteConfig          `yaml:"Site"`
	Node          NodeConfig          `yaml:"Node"`
	Mobile        MobileConfig        `yaml:"Mobile"`
	Email         EmailConfig         `yaml:"Email"`
	Device        DeviceConfig        `yaml:"device"`
	Verify        Verify              `yaml:"Verify"`
	VerifyCode    VerifyCode          `yaml:"VerifyCode"`
	Register      RegisterConfig      `yaml:"Register"`
	Subscribe     SubscribeConfig     `yaml:"Subscribe"`
	EdgeSubscribe EdgeSubscribeConfig `yaml:"EdgeSubscribe"`
	Invite        InviteConfig        `yaml:"Invite"`
	Telegram      Telegram            `yaml:"Telegram"`
	Log           Log                 `yaml:"Log"`
	Currency      Currency            `yaml:"Currency"`
	Administrator AdministratorConfig `yaml:"Administrator"`
}

// AdministratorConfig includes an optional one-way staged hash for recovering
// first installation if the process stops after schema creation.
type AdministratorConfig struct {
	Email        string `yaml:"Email" default:"admin@ppanel.dev"`
	Password     string `yaml:"Password" default:"password"`
	PasswordHash string `yaml:"PasswordHash,omitempty"`
}

type RedisConfig struct {
	Host string `yaml:"Host" default:"localhost:6379"`
	Pass string `yaml:"Pass" default:""`
	DB   int    `yaml:"DB" default:"0"`
}

type TransportConfig struct {
	Driver string `yaml:"Driver" default:"hertz"`
}

type JwtAuth struct {
	AccessSecret string `yaml:"AccessSecret"`
	AccessExpire int64  `yaml:"AccessExpire" default:"604800"`
}

type Verify struct {
	TurnstileSiteKey    string `yaml:"TurnstileSiteKey" default:""`
	TurnstileSecret     string `yaml:"TurnstileSecret" default:""`
	LoginVerify         bool   `yaml:"LoginVerify" default:"false"`
	RegisterVerify      bool   `yaml:"RegisterVerify" default:"false"`
	ResetPasswordVerify bool   `yaml:"ResetPasswordVerify" default:"false"`
}

type SubscribeConfig struct {
	SingleModel           bool   `yaml:"SingleModel" default:"false"`
	SubscribePath         string `yaml:"SubscribePath" default:"/v1/subscribe/config"`
	SubscribeDomain       string `yaml:"SubscribeDomain" default:""`
	PanDomain             bool   `yaml:"PanDomain" default:"false"`
	UserAgentLimit        bool   `yaml:"UserAgentLimit" default:"false"`
	UserAgentList         string `yaml:"UserAgentList" default:""`
	ShowTutorial          bool   `yaml:"ShowTutorial" default:"true"`
	ProfileUpdateInterval int64  `yaml:"ProfileUpdateInterval" default:"0"`
	ProfileWebPageURL     string `yaml:"ProfileWebPageURL" default:""`
}

// EdgeSubscribeConfig configures the private manifest API consumed by the
// Cloudflare edge-subscribe Worker. It intentionally has no relationship to
// the user-facing subscription template configuration above.
type EdgeSubscribeConfig struct {
	Enabled             bool                     `yaml:"Enabled" default:"false"`
	MaxClockSkewSeconds int64                    `yaml:"MaxClockSkewSeconds" default:"300"`
	Keys                []EdgeSubscribeAccessKey `yaml:"Keys"`
}

// EdgeSubscribeAccessKey is an HMAC credential owned by a Worker deployment.
// Secret must be stored only in the PPanel runtime config and the Worker secret
// store; it must never be returned from an API.
type EdgeSubscribeAccessKey struct {
	ID     string `yaml:"ID"`
	Secret string `yaml:"Secret"`
}

type RegisterConfig struct {
	StopRegister            bool   `yaml:"StopRegister" default:"false"`
	EnableTrial             bool   `yaml:"EnableTrial" default:"false"`
	TrialSubscribe          int64  `yaml:"TrialSubscribe" default:"0"`
	TrialTime               int64  `yaml:"TrialTime" default:"0"`
	TrialTimeUnit           string `yaml:"TrialTimeUnit" default:""`
	IpRegisterLimit         int64  `yaml:"IpRegisterLimit" default:"0"`
	IpRegisterLimitDuration int64  `yaml:"IpRegisterLimitDuration" default:"0"`
	EnableIpRegisterLimit   bool   `yaml:"EnableIpRegisterLimit" default:"false"`
}

type EmailConfig struct {
	Enable                     bool   `yaml:"Enable" default:"true"`
	Platform                   string `yaml:"platform"`
	PlatformConfig             string `yaml:"platform_config"`
	EnableVerify               bool   `yaml:"enable_verify"`
	EnableNotify               bool   `yaml:"enable_notify"`
	EnableDomainSuffix         bool   `yaml:"enable_domain_suffix"`
	DomainSuffixList           string `yaml:"domain_suffix_list"`
	VerifyEmailTemplate        string `yaml:"verify_email_template"`
	ExpirationEmailTemplate    string `yaml:"expiration_email_template"`
	MaintenanceEmailTemplate   string `yaml:"maintenance_email_template"`
	TrafficExceedEmailTemplate string `yaml:"traffic_exceed_email_template"`
	// Subjects pair with the templates above and hydrate from the email auth
	// config by field name (tool.DeepCopy); empty means the queued fallback
	// subject is used.
	VerifyEmailSubject        string `yaml:"verify_email_subject"`
	ExpirationEmailSubject    string `yaml:"expiration_email_subject"`
	MaintenanceEmailSubject   string `yaml:"maintenance_email_subject"`
	TrafficExceedEmailSubject string `yaml:"traffic_exceed_email_subject"`
}

type MobileConfig struct {
	Enable          bool     `yaml:"Enable" default:"true"`
	Platform        string   `yaml:"platform"`
	PlatformConfig  string   `yaml:"platform_config"`
	EnableVerify    bool     `yaml:"enable_verify"`
	EnableWhitelist bool     `yaml:"enable_whitelist"`
	Whitelist       []string `yaml:"whitelist"`
}

type DeviceConfig struct {
	Enable         bool   `yaml:"enable" default:"true"`
	ShowAds        bool   `yaml:"show_ads"`
	EnableSecurity bool   `yaml:"enable_security"`
	OnlyRealDevice bool   `yaml:"only_real_device"`
	SecuritySecret string `yaml:"security_secret"`
}

type SiteConfig struct {
	Host       string `yaml:"Host" default:""`
	SiteName   string `yaml:"SiteName" default:""`
	SiteDesc   string `yaml:"SiteDesc" default:""`
	SiteLogo   string `yaml:"SiteLogo" default:""`
	Keywords   string `yaml:"Keywords" default:""`
	CustomHTML string `yaml:"CustomHTML" default:""`
	CustomData string `yaml:"CustomData" default:""`
}

type NodeConfig struct {
	NodeSecret             string       `yaml:"NodeSecret" default:""`
	NodePullInterval       int64        `yaml:"NodePullInterval" default:"60"`
	NodePushInterval       int64        `yaml:"NodePushInterval" default:"60"`
	TrafficReportThreshold int64        `yaml:"TrafficReportThreshold" default:"0"`
	IPStrategy             string       `yaml:"IPStrategy" default:""`
	DNS                    []NodeDNS    `yaml:"DNS"`
	Routing                *NodeRouting `yaml:"Routing,omitempty" json:"routing,omitempty"`
	RoutingRevision        int64        `yaml:"RoutingRevision,omitempty" json:"routing_revision,omitempty"`
}

func (n *NodeConfig) Marshal() ([]byte, error) {
	type Alias NodeConfig
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(n),
	})
}

func (n *NodeConfig) Unmarshal(data []byte) error {
	type Alias NodeConfig
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(n),
	}
	return json.Unmarshal(data, &aux)
}

type NodeDNS struct {
	Proto      string   `json:"proto"`
	Address    string   `json:"address"`
	ServerName string   `json:"server_name,omitempty"`
	Domains    []string `json:"domains"`
}

func (n *NodeDNS) Marshal() ([]byte, error) {
	type Alias NodeDNS
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(n),
	})
}

func (n *NodeDNS) Unmarshal(data []byte) error {
	type Alias NodeDNS
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(n),
	}
	return json.Unmarshal(data, &aux)
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
	Rules                []string `json:"rules" yaml:"rules"`
	IPRules              []string `json:"ip_rules" yaml:"ip_rules"`
	ProtocolRules        []string `json:"protocol_rules" yaml:"protocol_rules"`
}

func (n *NodeOutbound) Marshal() ([]byte, error) {
	type Alias NodeOutbound
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(n),
	})
}

type File struct {
	Host          string               `yaml:"Host" default:"0.0.0.0"`
	Port          int                  `yaml:"Port" default:"8080"`
	Transport     TransportConfig      `yaml:"Transport"`
	TLS           TLS                  `yaml:"TLS"`
	Debug         bool                 `yaml:"Debug" default:"true"`
	JwtAuth       JwtAuth              `yaml:"JwtAuth"`
	Logger        logger.LogConf       `yaml:"Logger"`
	Trace         trace.Config         `yaml:"Trace"`
	Database      orm.Config           `yaml:"Database"`
	MySQL         *orm.Config          `yaml:"MySQL,omitempty"` // Deprecated: use Database.
	Redis         RedisConfig          `yaml:"Redis"`
	EdgeSubscribe EdgeSubscribeConfig  `yaml:"EdgeSubscribe"`
	Administrator *AdministratorConfig `yaml:"Administrator,omitempty"`
}

func (c Config) DatabaseConfig() orm.Config {
	if hasDatabaseConfig(c.Database) {
		return c.Database
	}
	if c.MySQL != nil {
		return *c.MySQL
	}
	return c.Database
}

func (c *Config) SetDatabaseConfig(cfg orm.Config) {
	c.Database = cfg
	c.MySQL = nil
}

func (f File) DatabaseConfig() orm.Config {
	if hasDatabaseConfig(f.Database) {
		return f.Database
	}
	if f.MySQL != nil {
		return *f.MySQL
	}
	return f.Database
}

func (f *File) SetDatabaseConfig(cfg orm.Config) {
	f.Database = cfg
	f.MySQL = nil
}

func hasDatabaseConfig(cfg orm.Config) bool {
	return cfg.Addr != "" || cfg.Dbname != "" || cfg.Username != "" || cfg.Password != ""
}

type InviteConfig struct {
	ForcedInvite       bool   `yaml:"ForcedInvite" default:"false"`
	ReferralPercentage int64  `yaml:"ReferralPercentage" default:"0"`
	OnlyFirstPurchase  bool   `yaml:"OnlyFirstPurchase" default:"false"`
	WithdrawalMethod   string `yaml:"WithdrawalMethod" default:""`
}

type Telegram struct {
	Enable        bool   `yaml:"Enable" default:"false"`
	BotID         int64  `yaml:"BotID" default:""`
	BotName       string `yaml:"BotName" default:""`
	BotToken      string `yaml:"BotToken" default:""`
	EnableNotify  bool   `yaml:"EnableNotify" default:"false"`
	WebHookDomain string `yaml:"WebHookDomain" default:""`
	// GroupChatID is the validated administrators' group; zero when the
	// group is unconfigured or failed validation, which disables every
	// group-side feature (admin commands, topics, notifications).
	GroupChatID int64 `yaml:"GroupChatID" default:"0"`
}

type TLS struct {
	Enable   bool   `yaml:"Enable" default:"false"`
	CertFile string `yaml:"CertFile" default:""`
	KeyFile  string `yaml:"KeyFile" default:""`
}

type VerifyCode struct {
	ExpireTime int64 `yaml:"ExpireTime" default:"300"`
	Limit      int64 `yaml:"Limit" default:"15"`
	Interval   int64 `yaml:"Interval" default:"60"`
}

type Log struct {
	AutoClear bool  `yaml:"AutoClear" default:"true"`
	ClearDays int64 `yaml:"ClearDays" default:"7"`
}

type NodeDBConfig struct {
	NodeSecret             string
	NodePullInterval       int64
	NodePushInterval       int64
	TrafficReportThreshold int64
	IPStrategy             string
	DNS                    string
}

type Currency struct {
	Unit      string `yaml:"Unit" default:"CNY"`
	Symbol    string `yaml:"Symbol" default:"USD"`
	AccessKey string `yaml:"AccessKey" default:""`
}
