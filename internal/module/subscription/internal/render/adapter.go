package render

import (
	"strings"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/pkg/logger"
)

type Adapter struct {
	Type           string            // 协议类型
	SiteName       string            // 站点名称
	Servers        []*node.Node      // 服务器列表
	UserInfo       User              // 用户信息
	ClientTemplate string            // 客户端配置模板
	OutputFormat   string            // 输出格式，默认是 base64
	SubscribeName  string            // 订阅名称
	Params         map[string]string // 其他参数
}

type Option func(*Adapter)

func WithParams(params map[string]string) Option {
	return func(opts *Adapter) {
		opts.Params = params
	}
}

// WithServers 设置服务器列表
func WithServers(servers []*node.Node) Option {
	return func(opts *Adapter) {
		opts.Servers = servers
	}
}

// WithUserInfo 设置用户信息
func WithUserInfo(user User) Option {
	return func(opts *Adapter) {
		opts.UserInfo = user
	}
}

// WithOutputFormat 设置输出格式
func WithOutputFormat(format string) Option {
	return func(opts *Adapter) {
		opts.OutputFormat = format
	}
}

// WithSiteName 设置站点名称
func WithSiteName(name string) Option {
	return func(opts *Adapter) {
		opts.SiteName = name
	}
}

// WithSubscribeName 设置订阅名称
func WithSubscribeName(name string) Option {
	return func(opts *Adapter) {
		opts.SubscribeName = name
	}
}

func NewAdapter(tpl string, opts ...Option) *Adapter {
	adapter := &Adapter{
		Servers:        []*node.Node{},
		UserInfo:       User{},
		ClientTemplate: tpl,
		OutputFormat:   "base64", // 默认输出格式
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

func (adapter *Adapter) Client() (*Client, error) {
	client := &Client{
		SiteName:       adapter.SiteName,
		SubscribeName:  adapter.SubscribeName,
		ClientTemplate: adapter.ClientTemplate,
		OutputFormat:   adapter.OutputFormat,
		Proxies:        []Proxy{},
		UserInfo:       adapter.UserInfo,
		Params:         adapter.Params,
	}

	proxies, err := adapter.Proxies(adapter.Servers)
	if err != nil {
		return nil, err
	}
	client.Proxies = proxies
	return client, nil
}

func (adapter *Adapter) Proxies(servers []*node.Node) ([]Proxy, error) {
	var proxies []Proxy

	for _, item := range servers {
		itemProtocol := canonicalProtocolType(item.Protocol)
		if item.Server == nil {
			logger.Errorf("[Adapter] Server is nil for node ID: %d", item.Id)
			continue
		}
		protocols, err := item.Server.UnmarshalProtocols()
		if err != nil {
			logger.Errorf("[Adapter] Unmarshal Protocols error: %s; server id : %d", err.Error(), item.ServerId)
			continue
		}
		for _, protocol := range protocols {
			protocolType := canonicalProtocolType(protocol.Type)
			if protocolType == itemProtocol {
				protocol.Type = protocolType
				plugin, pluginOptions := clientPluginConfig(protocol, item.Address)
				// 证书锚定与跳过验证互斥：客户端一旦跳过校验，下发的指纹便形同虚设。
				allowInsecure := protocol.AllowInsecure && protocol.CertPinSHA256 == ""
				proxies = append(
					proxies,
					Proxy{
						Sort:                    item.Sort,
						Name:                    item.Name,
						Server:                  item.Address,
						Port:                    item.Port,
						Type:                    protocolType,
						Tags:                    strings.Split(item.Tags, ","),
						Version:                 protocol.Version,
						Mode:                    protocol.Mode,
						Network:                 protocol.Network,
						Security:                protocol.Security,
						SNI:                     protocol.SNI,
						ALPN:                    protocol.ALPN,
						AllowInsecure:           allowInsecure,
						Fingerprint:             protocol.Fingerprint,
						RealityPublicKey:        protocol.RealityPublicKey,
						RealityShortId:          protocol.RealityShortId,
						Transport:               protocol.Transport,
						Host:                    protocol.Host,
						Path:                    protocol.Path,
						ServiceName:             protocol.ServiceName,
						Method:                  protocol.Cipher,
						ServerKey:               protocol.ServerKey,
						Plugin:                  plugin,
						PluginOptions:           pluginOptions,
						UoT:                     protocol.UoT,
						UoTVersion:              protocol.UoTVersion,
						Flow:                    protocol.Flow,
						HopPorts:                protocol.HopPorts,
						HopInterval:             protocol.HopInterval,
						ObfsPassword:            protocol.ObfsPassword,
						UpMbps:                  protocol.UpMbps,
						DownMbps:                protocol.DownMbps,
						DisableSNI:              protocol.DisableSNI,
						ReduceRtt:               protocol.ReduceRtt,
						Heartbeat:               protocol.Heartbeat,
						UDPRelayMode:            protocol.UDPRelayMode,
						CongestionController:    protocol.CongestionController,
						QUICCongestionControl:   protocol.QUICCongestionControl,
						PaddingScheme:           protocol.PaddingScheme,
						Multiplex:               protocol.Multiplex,
						TrafficPattern:          protocol.TrafficPattern,
						UserHintIsMandatory:     protocol.UserHintIsMandatory,
						Obfs:                    protocol.Obfs,
						SSRProtocol:             protocol.SSRProtocol,
						ProtocolParam:           protocol.ProtocolParam,
						ObfsParam:               protocol.ObfsParam,
						ObfsHost:                protocol.ObfsHost,
						ObfsPath:                protocol.ObfsPath,
						XhttpMode:               protocol.XhttpMode,
						XhttpExtra:              protocol.XhttpExtra,
						Encryption:              protocol.Encryption,
						EncryptionMode:          protocol.EncryptionMode,
						EncryptionRtt:           protocol.EncryptionRtt,
						EncryptionClientPadding: protocol.EncryptionClientPadding,
						EncryptionPassword:      protocol.EncryptionPassword,
						EchEnable:               protocol.EchEnable,
						EchServerName:           protocol.EchServerName,
						Ratio:                   protocol.Ratio,
						CertPinSHA256:           protocol.CertPinSHA256,
					},
				)
			}
		}
	}

	return proxies, nil
}

func canonicalProtocolType(raw string) string {
	protocol, err := node.NormalizeProtocolForStorage(node.Protocol{Type: raw})
	if err != nil {
		return strings.ToLower(strings.TrimSpace(raw))
	}
	return protocol.Type
}
