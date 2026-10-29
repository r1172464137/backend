package server

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

func bindOnlineUsersRequest(ctx *app.RequestContext, req *dto.OnlineUsersRequest) error {
	if !isProtobufRequest(ctx) {
		return ctx.BindJSON(req)
	}

	var message serverv1.PushOnlineUsersRequest
	if err := ctx.BindProtobuf(&message); err != nil {
		return err
	}
	req.Users = make([]dto.OnlineUser, 0, len(message.Users))
	for _, user := range message.Users {
		if user == nil {
			continue
		}
		req.Users = append(req.Users, dto.OnlineUser{SID: user.UserId, IP: user.Ip})
	}
	return nil
}

func bindUserTrafficRequest(ctx *app.RequestContext, req *dto.ServerPushUserTrafficRequest) error {
	if !isProtobufRequest(ctx) {
		return ctx.BindJSON(req)
	}

	var message serverv1.PushUserTrafficRequest
	if err := ctx.BindProtobuf(&message); err != nil {
		return err
	}
	req.Traffic = make([]dto.UserTraffic, 0, len(message.Traffic))
	for _, traffic := range message.Traffic {
		if traffic == nil {
			continue
		}
		req.Traffic = append(req.Traffic, dto.UserTraffic{
			SID:      traffic.UserId,
			Upload:   traffic.Upload,
			Download: traffic.Download,
		})
	}
	return nil
}

func bindServerStatusRequest(ctx *app.RequestContext, req *dto.ServerPushStatusRequest) error {
	if !isProtobufRequest(ctx) {
		if err := validateStatusJSON(ctx.Request.Body()); err != nil {
			return err
		}
		return ctx.BindJSON(req)
	}

	var message serverv1.PushServerStatusRequest
	if err := ctx.BindProtobuf(&message); err != nil {
		return err
	}
	if len(message.ProtoReflect().GetUnknown()) > 0 {
		return fmt.Errorf("unknown status protobuf field")
	}
	for _, identity := range []*serverv1.RoutingIdentity{message.RoutingApplied, message.RoutingPending} {
		if identity != nil && len(identity.ProtoReflect().GetUnknown()) > 0 {
			return fmt.Errorf("unknown routing identity field")
		}
	}
	if message.RoutingError != nil {
		if len(message.RoutingError.ProtoReflect().GetUnknown()) > 0 {
			return fmt.Errorf("unknown routing error field")
		}
		if identity := message.RoutingError.Identity; identity != nil && len(identity.ProtoReflect().GetUnknown()) > 0 {
			return fmt.Errorf("unknown routing identity field")
		}
	}
	req.Cpu = message.Cpu
	req.Mem = message.Mem
	req.Disk = message.Disk
	req.UpdatedAt = message.UpdatedAt
	req.RoutingVersion = int(message.RoutingVersion)
	req.InstanceID = message.InstanceId
	req.RoutingSequence = message.RoutingSequence
	req.RoutingApplied = routingIdentityFromProtobuf(message.RoutingApplied)
	req.RoutingPending = routingIdentityFromProtobuf(message.RoutingPending)
	if message.RoutingError != nil {
		req.RoutingError = &dto.RoutingReportError{Identity: routingIdentityFromProtobuf(message.RoutingError.Identity), Stage: message.RoutingError.Stage, Message: message.RoutingError.Message}
	}
	return nil
}

// Match the exact JSON wire vocabulary, including nested objects. Go's JSON
// decoder otherwise silently accepts unknown and case-variant field names.
func validateStatusJSON(body []byte) error {
	if err := config.ValidateUniqueJSON(body); err != nil {
		return err
	}
	var object func([]byte, map[string]bool) (map[string]json.RawMessage, error)
	object = func(raw []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return nil, fmt.Errorf("expected status object")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &fields); err != nil {
			return nil, err
		}
		for key := range fields {
			if !allowed[key] {
				return nil, fmt.Errorf("unknown status field")
			}
		}
		return fields, nil
	}
	top, err := object(body, map[string]bool{"cpu": true, "mem": true, "disk": true, "updated_at": true, "routing_version": true, "routing_applied": true, "routing_pending": true, "routing_error": true, "instance_id": true, "routing_sequence": true})
	if err != nil {
		return err
	}
	identityNames := map[string]bool{"mode": true, "source": true, "source_server_id": true, "source_revision": true, "local_transition_revision": true}
	checkIdentity := func(raw json.RawMessage) error {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		_, err := object(raw, identityNames)
		return err
	}
	for _, name := range []string{"routing_applied", "routing_pending"} {
		if raw, ok := top[name]; ok {
			if err := checkIdentity(raw); err != nil {
				return err
			}
		}
	}
	if raw, ok := top["routing_error"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		fields, err := object(raw, map[string]bool{"identity": true, "stage": true, "message": true})
		if err != nil {
			return err
		}
		if identity, ok := fields["identity"]; ok {
			if err := checkIdentity(identity); err != nil {
				return err
			}
		}
	}
	return nil
}

func routingIdentityFromProtobuf(value *serverv1.RoutingIdentity) *dto.RoutingCandidateIdentity {
	if value == nil {
		return nil
	}
	return &dto.RoutingCandidateIdentity{Mode: value.Mode, Source: value.Source, SourceServerID: value.SourceServerId, SourceRevision: value.SourceRevision, LocalTransitionRevision: value.LocalTransitionRevision}
}

func serverUserListResponseToProtobuf(response *dto.GetServerUserListResponse) *serverv1.GetServerUserListResponse {
	users := make([]*serverv1.ServerUser, 0, len(response.Users))
	for _, user := range response.Users {
		users = append(users, &serverv1.ServerUser{
			Id:          user.Id,
			Uuid:        user.UUID,
			SpeedLimit:  user.SpeedLimit,
			DeviceLimit: user.DeviceLimit,
		})
	}
	return &serverv1.GetServerUserListResponse{
		Code:    200,
		Message: "success",
		Data:    &serverv1.ServerUserListData{Users: users},
	}
}

func queryServerProtocolConfigResponseToProtobuf(response *dto.QueryServerConfigResponse) (*serverv1.QueryServerProtocolConfigResponse, error) {
	if err := validateRoutingDelivery(response, 0); err != nil {
		return nil, err
	}
	dns := make([]*serverv1.DNSResolver, 0, len(response.DNS))
	for _, resolver := range response.DNS {
		dns = append(dns, &serverv1.DNSResolver{
			Proto:      resolver.Proto,
			Address:    resolver.Address,
			ServerName: resolver.ServerName,
			Domains:    append([]string(nil), resolver.Domains...),
		})
	}

	protocols := make([]*serverv1.ServerProtocol, 0, len(response.Protocols))
	for _, protocol := range response.Protocols {
		converted, err := serverProtocolToProtobuf(protocol)
		if err != nil {
			return nil, err
		}
		protocols = append(protocols, converted)
	}

	routing, err := routingToProtobuf(response.Routing)
	if err != nil {
		return nil, err
	}
	return &serverv1.QueryServerProtocolConfigResponse{
		Code:    200,
		Message: "success",
		Data: &serverv1.QueryServerProtocolConfigData{
			TrafficReportThreshold: response.TrafficReportThreshold,
			PushInterval:           response.PushInterval,
			PullInterval:           response.PullInterval,
			IpStrategy:             response.IPStrategy,
			Dns:                    dns,
			Protocols:              protocols,
			Total:                  response.Total,
			Routing:                routing,
		},
	}, nil
}

func outboundToProtobuf(outbound dto.NodeOutbound) (*serverv1.Outbound, error) {
	pluginOptions, err := valueToProtobufValue(outbound.PluginOptions)
	if err != nil {
		return nil, err
	}
	return &serverv1.Outbound{
		Id:                      outbound.ID,
		Name:                    outbound.Name,
		Protocol:                outbound.Protocol,
		Address:                 outbound.Address,
		Port:                    outbound.Port,
		User:                    outbound.User,
		Password:                outbound.Password,
		Uuid:                    outbound.UUID,
		Cipher:                  outbound.Cipher,
		Plugin:                  outbound.Plugin,
		PluginOptions:           pluginOptions,
		Security:                outbound.Security,
		Sni:                     outbound.SNI,
		Alpn:                    append([]string(nil), outbound.ALPN...),
		AllowInsecure:           outbound.AllowInsecure,
		Fingerprint:             outbound.Fingerprint,
		Transport:               outbound.Transport,
		Host:                    outbound.Host,
		Path:                    outbound.Path,
		ServiceName:             outbound.ServiceName,
		XhttpMode:               outbound.XHTTPMode,
		XhttpExtra:              outbound.XHTTPExtra,
		Flow:                    outbound.Flow,
		Encryption:              outbound.Encryption,
		EncryptionMode:          outbound.EncryptionMode,
		EncryptionRtt:           outbound.EncryptionRTT,
		EncryptionTicket:        outbound.EncryptionTicket,
		EncryptionClientPadding: outbound.EncryptionPadding,
		EncryptionPassword:      outbound.EncryptionPassword,
		Multiplex:               outbound.Multiplex,
		Uot:                     outbound.UoT,
		UotVersion:              int64(outbound.UoTVersion),
		CongestionController:    outbound.CongestionController,
		UdpStream:               outbound.UDPStream,
		ReduceRtt:               outbound.ReduceRtt,
		Heartbeat:               int64(outbound.Heartbeat),
		RealityPublicKey:        outbound.RealityPublicKey,
		RealityShortId:          outbound.RealityShortId,
		SpiderX:                 outbound.SpiderX,
		Settings:                outbound.Settings,
		StreamSettings:          outbound.StreamSettings,
		Rules:                   append([]string(nil), outbound.Rules...),
		IpRules:                 append([]string(nil), outbound.IPRules...),
		ProtocolRules:           append([]string(nil), outbound.ProtocolRules...),
	}, nil
}

func routingToProtobuf(policy *dto.NodeRoutingPolicy) (*serverv1.RoutingPolicy, error) {
	if policy == nil {
		return nil, fmt.Errorf("ordered routing is required")
	}
	outbounds := make([]*serverv1.Outbound, 0, len(policy.Outbounds))
	for _, outbound := range policy.Outbounds {
		value, err := outboundToProtobuf(outbound)
		if err != nil {
			return nil, err
		}
		outbounds = append(outbounds, value)
	}
	rules := make([]*serverv1.RouteRule, 0, len(policy.Rules))
	for _, rule := range policy.Rules {
		if rule.Enabled == nil {
			return nil, fmt.Errorf("route %q is missing enabled", rule.ID)
		}
		var kind serverv1.RouteKind
		switch rule.Kind {
		case "domain":
			kind = serverv1.RouteKind_ROUTE_KIND_DOMAIN
		case "ip":
			kind = serverv1.RouteKind_ROUTE_KIND_IP
		case "protocol":
			kind = serverv1.RouteKind_ROUTE_KIND_PROTOCOL
		default:
			return nil, fmt.Errorf("unknown route kind %q", rule.Kind)
		}
		enabled := *rule.Enabled
		rules = append(rules, &serverv1.RouteRule{Id: rule.ID, Enabled: &enabled, Remark: rule.Remark, Kind: kind, Values: append([]string(nil), rule.Values...), OutboundId: rule.OutboundID})
	}
	if policy.Version != 1 {
		return nil, fmt.Errorf("unknown routing version %d", policy.Version)
	}
	return &serverv1.RoutingPolicy{Version: uint32(policy.Version), Outbounds: outbounds, Rules: rules,
		DefaultOutboundId: policy.DefaultOutboundID, Mode: policy.Mode, Source: policy.Source,
		SourceServerId: policy.SourceServerID, SourceRevision: policy.SourceRevision,
		LocalTransitionRevision: policy.LocalTransitionRevision}, nil
}

func serverProtocolToProtobuf(protocol dto.Protocol) (*serverv1.ServerProtocol, error) {
	if protocol.RealityXver < 0 || protocol.RealityXver > 2 {
		return nil, fmt.Errorf("reality_xver must be 0, 1 or 2")
	}
	pluginOptions, err := valueToProtobufValue(protocol.PluginOptions)
	if err != nil {
		return nil, err
	}
	return &serverv1.ServerProtocol{
		Type:                    protocol.Type,
		Port:                    uint32(protocol.Port),
		Version:                 int64(protocol.Version),
		Mode:                    protocol.Mode,
		Enable:                  protocol.Enable,
		Security:                protocol.Security,
		Network:                 protocol.Network,
		Sni:                     protocol.SNI,
		Alpn:                    append([]string(nil), protocol.ALPN...),
		AllowInsecure:           protocol.AllowInsecure,
		Fingerprint:             protocol.Fingerprint,
		RealityServerAddr:       protocol.RealityServerAddr,
		RealityServerPort:       int64(protocol.RealityServerPort),
		RealityXver:             uint32(protocol.RealityXver),
		RealityPrivateKey:       protocol.RealityPrivateKey,
		RealityPublicKey:        protocol.RealityPublicKey,
		RealityShortId:          protocol.RealityShortId,
		Transport:               protocol.Transport,
		Host:                    protocol.Host,
		Path:                    protocol.Path,
		ServiceName:             protocol.ServiceName,
		Cipher:                  protocol.Cipher,
		ServerKey:               protocol.ServerKey,
		Plugin:                  protocol.Plugin,
		PluginOptions:           pluginOptions,
		Flow:                    protocol.Flow,
		Uot:                     protocol.UoT,
		UotVersion:              int64(protocol.UoTVersion),
		AcceptProxyProtocol:     protocol.AcceptProxyProtocol,
		HopPorts:                protocol.HopPorts,
		HopInterval:             int64(protocol.HopInterval),
		ObfsPassword:            protocol.ObfsPassword,
		DisableSni:              protocol.DisableSNI,
		ReduceRtt:               protocol.ReduceRtt,
		Heartbeat:               int64(protocol.Heartbeat),
		UdpRelayMode:            protocol.UDPRelayMode,
		CongestionController:    protocol.CongestionController,
		QuicCongestionControl:   protocol.QUICCongestionControl,
		Multiplex:               protocol.Multiplex,
		PaddingScheme:           protocol.PaddingScheme,
		TrafficPattern:          protocol.TrafficPattern,
		UserHintIsMandatory:     protocol.UserHintIsMandatory,
		UpMbps:                  int64(protocol.UpMbps),
		DownMbps:                int64(protocol.DownMbps),
		Obfs:                    protocol.Obfs,
		SsrProtocol:             protocol.SSRProtocol,
		ProtocolParam:           protocol.ProtocolParam,
		ObfsParam:               protocol.ObfsParam,
		ObfsHost:                protocol.ObfsHost,
		ObfsPath:                protocol.ObfsPath,
		XhttpMode:               protocol.XhttpMode,
		XhttpExtra:              protocol.XhttpExtra,
		Encryption:              protocol.Encryption,
		EncryptionMode:          protocol.EncryptionMode,
		EncryptionRtt:           protocol.EncryptionRtt,
		EncryptionTicket:        protocol.EncryptionTicket,
		EncryptionServerPadding: protocol.EncryptionServerPadding,
		EncryptionPrivateKey:    protocol.EncryptionPrivateKey,
		EncryptionClientPadding: protocol.EncryptionClientPadding,
		EncryptionPassword:      protocol.EncryptionPassword,
		EchEnable:               protocol.EchEnable,
		EchServerName:           protocol.EchServerName,
		Ratio:                   protocol.Ratio,
		CertMode:                protocol.CertMode,
		CertDnsProvider:         protocol.CertDNSProvider,
		CertDnsEnv:              protocol.CertDNSEnv,
	}, nil
}

func valueToProtobufValue(value interface{}) (*structpb.Value, error) {
	if value == nil {
		return nil, nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	result := &structpb.Value{}
	if err := protojson.Unmarshal(body, result); err != nil {
		return nil, err
	}
	return result, nil
}
