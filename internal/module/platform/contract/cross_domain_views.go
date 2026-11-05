// Package dto holds the platform module's contract: the requests and
// responses of the audit log, system settings, admin console, admin tool and
// public site endpoints. The Go package keeps the name dto so the Swagger
// schema names (dto.*) stay stable.
//
// The views of other modules' data in this file are platform-owned
// snapshots: they deliberately duplicate the JSON shapes instead of coupling
// one module's API to another.
package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/perfect-panel/server/internal/config"
)

// AuthConfig is the public site's view of the sign-in and registration
// settings.
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
	ServerId int64 `form:"server_id"`
}

type FilterServerTrafficLogResponse struct {
	Total int64              `json:"total"`
	List  []ServerTrafficLog `json:"list"`
}

type FilterSubscribeTrafficRequest struct {
	FilterLogParams
	UserId          int64 `form:"user_id"`
	UserSubscribeId int64 `form:"user_subscribe_id"`
}

type FilterSubscribeTrafficResponse struct {
	Total int64                     `json:"total"`
	List  []UserSubscribeTrafficLog `json:"list"`
}

type FilterTrafficLogDetailsRequest struct {
	FilterLogParams
	ServerId    int64 `form:"server_id"`
	SubscribeId int64 `form:"subscribe_id"`
	UserId      int64 `form:"user_id"`
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
	NodeSecret             string                    `json:"node_secret"`
	NodePullInterval       int64                     `json:"node_pull_interval"`
	NodePushInterval       int64                     `json:"node_push_interval"`
	TrafficReportThreshold int64                     `json:"traffic_report_threshold"`
	IPStrategy             string                    `json:"ip_strategy"`
	DNS                    []PlatformNodeDNSSnapshot `json:"dns"`
	Routing                *config.NodeRouting       `json:"routing"`
	RoutingRevision        int64                     `json:"routing_revision"`
	RoutingDowngrade       bool                      `json:"routing_downgrade,omitempty"`
	RoutingMode            string                    `json:"routing_mode"`
	RoutingCapabilities    []int                     `json:"routing_capabilities"`
	RequestFields          map[string]struct{}       `json:"-" swaggerignore:"true"`
}

// UnmarshalJSON records field presence so an old admin client that does not
// know the additive route-rule fields cannot erase them during a PUT.
func (n *NodeConfig) UnmarshalJSON(data []byte) error {
	if err := config.ValidateUniqueJSON(data); err != nil {
		return err
	}
	type alias NodeConfig
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*n = NodeConfig(decoded)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"routing_revision", "node_pull_interval", "node_push_interval"} {
		if raw, ok := fields[name]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s must not be null", name)
		}
	}
	n.RequestFields = make(map[string]struct{}, len(fields))
	for field := range fields {
		if field != strings.ToLower(field) {
			return fmt.Errorf("invalid JSON field %q", field)
		}
		n.RequestFields[field] = struct{}{}
	}
	if raw, ok := fields["dns"]; ok {
		items, err := config.ParseNodeDNSJSON(raw)
		if err != nil {
			return err
		}
		n.DNS = make([]PlatformNodeDNSSnapshot, 0, len(items))
		for _, d := range items {
			n.DNS = append(n.DNS, PlatformNodeDNSSnapshot{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
		}
	}
	if raw, ok := fields["routing"]; ok && string(bytes.TrimSpace(raw)) != "null" {
		policy, err := config.ParseNodeRouting(raw)
		if err != nil {
			return err
		}
		n.Routing = policy
	}
	return nil
}

type PlatformNodeDNSSnapshot struct {
	Proto      string   `json:"proto"`
	Address    string   `json:"address"`
	ServerName string   `json:"server_name,omitempty"`
	Domains    []string `json:"domains"`
} // @name dto.NodeDNS

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
