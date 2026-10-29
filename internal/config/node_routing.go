package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits for an ordered-v1 policy are independent of the legacy TEXT-field
// limits. Both the submitted and canonical forms are bounded before use.
const (
	RoutingVersion1         = 1
	MaxRoutingOutbounds     = 128
	MaxRoutingRules         = 512
	MaxRoutingValuesPerRule = 1024
	MaxRoutingValues        = 8192
	MaxRoutingMatchBytes    = 256 * 1024
	MaxRoutingJSONBytes     = 1024 * 1024
	MaxRoutingNameRunes     = 200
	MaxRoutingRemarkRunes   = 200
	RoutingDirect           = "direct"
	RoutingBlock            = "block"
)

var routingUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// NodeRouting is the persisted content only; revision and source identify a
// delivery, not the policy. enabled is a pointer to retain false vs missing.
// Limits: 128 outbounds, 512 rules, 1024 values per rule, 8192 aggregate
// values, 256 KiB canonical match text and 1 MiB final JSON.
type NodeRouting struct {
	Version           int            `json:"version" binding:"required"`
	Outbounds         []NodeOutbound `json:"outbounds" binding:"required"`
	Rules             []NodeRoute    `json:"rules" binding:"required"`
	DefaultOutboundID string         `json:"default_outbound_id" binding:"required"`
}

type NodeRoute struct {
	ID         string   `json:"id" binding:"required"`
	Enabled    *bool    `json:"enabled" binding:"required"`
	Remark     string   `json:"remark"`
	Kind       string   `json:"kind" binding:"required"`
	Values     []string `json:"values" binding:"required"`
	OutboundID string   `json:"outbound_id" binding:"required"`
}

// ParseNodeRouting rejects unknown fields and trailing data, including an
// explicitly null policy. It accepts no more than MaxRoutingJSONBytes on the
// input wire; callers must also bound the surrounding HTTP/protobuf envelope.
func ParseNodeRouting(data []byte) (*NodeRouting, error) {
	if len(data) > MaxRoutingJSONBytes {
		return nil, errorsWithField("routing", "input exceeds 1 MiB")
	}
	if err := ValidateUniqueJSON(data); err != nil {
		return nil, errorsWithField("routing", err.Error())
	}
	if err := validateRoutingJSONFields(data); err != nil {
		return nil, errorsWithField("routing", err.Error())
	}
	var value *NodeRouting
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, errorsWithField("routing", fmt.Sprintf("invalid JSON: %v", err))
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errorsWithField("routing", "trailing JSON data")
	}
	return NormalizeNodeRouting(value)
}

// NormalizeNodeRouting validates the entire policy, including disabled rules,
// and returns a detached canonical copy. No caller-owned slices are changed.
func NormalizeNodeRouting(input *NodeRouting) (*NodeRouting, error) {
	if input == nil {
		return nil, errorsWithField("routing", "policy must not be null")
	}
	if input.Version != RoutingVersion1 {
		return nil, errorsWithField("routing.version", "unsupported routing version")
	}
	if input.Outbounds == nil || input.Rules == nil {
		return nil, errorsWithField("routing", "outbounds and rules must be arrays")
	}
	if len(input.Outbounds) > MaxRoutingOutbounds || len(input.Rules) > MaxRoutingRules {
		return nil, errorsWithField("routing", "too many outbounds or rules")
	}
	// Reject excessive input array elements before constructing a serialized
	// copy, even when normalization would deduplicate them away.
	inputValues := 0
	for i, rule := range input.Rules {
		for _, value := range rule.Values {
			if len(strings.TrimSpace(value)) > MaxRouteRuleBytes {
				return nil, errorsWithField(fmt.Sprintf("routing.rules[%d].values", i+1), "raw match value exceeds 4096 bytes")
			}
		}
		if len(rule.Values) > MaxRoutingValuesPerRule {
			return nil, errorsWithField(fmt.Sprintf("routing.rules[%d].values", i+1), "too many input values")
		}
		inputValues += len(rule.Values)
		if inputValues > MaxRoutingValues {
			return nil, errorsWithField("routing.rules", "too many input values")
		}
	}
	// Encoding here also bounds arbitrary connection fields/plugin options before
	// copying or normalizing the potentially much smaller canonical policy.
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, errorsWithField("routing", fmt.Sprintf("cannot encode policy: %v", err))
	}
	if len(encoded) > MaxRoutingJSONBytes {
		return nil, errorsWithField("routing", "input exceeds 1 MiB")
	}
	// Programmatic callers can supply maps without going through the HTTP
	// decoder. Apply the same recursive ambiguity rule before persistence.
	if err := ValidateUniqueJSON(encoded); err != nil {
		return nil, errorsWithField("routing", err.Error())
	}

	result := &NodeRouting{Version: input.Version, DefaultOutboundID: input.DefaultOutboundID,
		Outbounds: make([]NodeOutbound, len(input.Outbounds)), Rules: make([]NodeRoute, len(input.Rules))}
	ids := map[string]struct{}{RoutingDirect: {}, RoutingBlock: {}}
	names := make(map[string]struct{}, len(input.Outbounds))
	for i, source := range input.Outbounds {
		field := fmt.Sprintf("routing.outbounds[%d]", i+1)
		if !validRoutingID(source.ID, "ob_") {
			return nil, errorsWithField(field+".id", "must be ob_ followed by a lowercase UUIDv4")
		}
		if _, ok := ids[source.ID]; ok {
			return nil, errorsWithField(field+".id", "duplicate outbound ID")
		}
		ids[source.ID] = struct{}{}
		name := strings.TrimSpace(source.Name)
		if err := routingText(field+".name", name, MaxRoutingNameRunes, false); err != nil {
			return nil, err
		}
		if _, ok := names[name]; ok {
			return nil, errorsWithField(field+".name", "duplicate outbound name")
		}
		names[name] = struct{}{}
		if err := validateOutboundUTF8(field, source); err != nil {
			return nil, err
		}
		if err := ValidateNodeURI("spider_x", source.SpiderX, true); err != nil {
			return nil, errorsWithField(field+".spider_x", err.Error())
		}
		if !ValidXHTTPMode(source.XHTTPMode) {
			return nil, errorsWithField(field+".xhttp_mode", "unsupported XHTTP mode")
		}
		if strings.TrimSpace(source.XHTTPExtra) != "" {
			if err := ValidateXHTTPExtra(source.XHTTPExtra, source.XHTTPMode); err != nil {
				return nil, errorsWithField(field+".xhttp_extra", err.Error())
			}
		}
		switch strings.ToLower(strings.TrimSpace(source.Security)) {
		case "", "none", "tls", "reality":
		default:
			return nil, errorsWithField(field+".security", "unsupported stream security")
		}
		if err := routingText(field+".protocol", strings.TrimSpace(source.Protocol), MaxRoutingNameRunes, false); err != nil {
			return nil, err
		}
		if len(source.Rules) != 0 || len(source.IPRules) != 0 || len(source.ProtocolRules) != 0 {
			return nil, errorsWithField(field, "ordered outbound must not contain legacy matching rules")
		}
		if err := ValidatePluginOptionsWire(source.PluginOptions); err != nil {
			return nil, errorsWithField(field+".plugin_opts", err.Error())
		}
		for _, raw := range []struct{ field, value string }{{"settings", source.Settings}, {"stream_settings", source.StreamSettings}, {"xhttp_extra", source.XHTTPExtra}} {
			if strings.TrimSpace(raw.value) != "" {
				var object map[string]json.RawMessage
				if ValidateUniqueJSON([]byte(raw.value)) != nil || json.Unmarshal([]byte(raw.value), &object) != nil || object == nil {
					return nil, errorsWithField(field+"."+raw.field, "must be a JSON object")
				}
				if raw.field == "stream_settings" || raw.field == "xhttp_extra" {
					if err := ValidateNodeStreamSpiderJSON(raw.value); err != nil {
						return nil, errorsWithField(field+"."+raw.field, err.Error())
					}
				}
				if raw.field == "stream_settings" {
					if err := ValidateNodeRawXHTTP(raw.value); err != nil {
						return nil, errorsWithField(field+"."+raw.field, err.Error())
					}
				}
			}
		}
		// Copy through JSON so arbitrary plugin options and nested slices cannot
		// alias input. It also retains the exact raw settings strings.
		copyJSON, err := json.Marshal(source)
		if err != nil {
			return nil, errorsWithField(field, err.Error())
		}
		decoder := json.NewDecoder(bytes.NewReader(copyJSON))
		decoder.UseNumber() // do not round plugin option numbers while cloning
		if err := decoder.Decode(&result.Outbounds[i]); err != nil {
			return nil, errorsWithField(field, err.Error())
		}
		result.Outbounds[i].Name = name
		result.Outbounds[i].Protocol = strings.TrimSpace(source.Protocol)
		result.Outbounds[i].Rules = []string{}
		result.Outbounds[i].IPRules = []string{}
		result.Outbounds[i].ProtocolRules = []string{}
	}
	missing := map[string][]string{}
	if len(input.DefaultOutboundID) > 39 {
		return nil, errorsWithField("routing.default_outbound_id", "outbound reference is too long")
	}
	if _, ok := ids[input.DefaultOutboundID]; !ok {
		missing[input.DefaultOutboundID] = append(missing[input.DefaultOutboundID], "default")
	}
	ruleIDs := make(map[string]struct{}, len(input.Rules))
	var matchValues, matchBytes int
	for i, source := range input.Rules {
		field := fmt.Sprintf("routing.rules[%d]", i+1)
		if !validRoutingID(source.ID, "rr_") {
			return nil, errorsWithField(field+".id", "must be rr_ followed by a lowercase UUIDv4")
		}
		if _, ok := ruleIDs[source.ID]; ok {
			return nil, errorsWithField(field+".id", "duplicate rule ID")
		}
		ruleIDs[source.ID] = struct{}{}
		if source.Enabled == nil {
			return nil, errorsWithField(field+".enabled", "must be explicitly true or false")
		}
		if err := routingText(field+".remark", source.Remark, MaxRoutingRemarkRunes, true); err != nil {
			return nil, err
		}
		if len(source.OutboundID) > 39 {
			return nil, errorsWithField(field+".outbound_id", "outbound reference is too long")
		}
		if _, ok := ids[source.OutboundID]; !ok {
			missing[source.OutboundID] = append(missing[source.OutboundID], source.ID)
		}
		var values []string
		switch source.Kind {
		case "domain":
			values, err = NormalizeDomainRules(field+".values", source.Values)
		case "ip":
			values, err = NormalizeIPRules(field+".values", source.Values)
		case "protocol":
			values, err = NormalizeProtocolRules(field+".values", source.Values)
		default:
			return nil, errorsWithField(field+".kind", "unknown route kind")
		}
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			return nil, errorsWithField(field+".values", "must contain at least one match value")
		}
		matchValues += len(values)
		for _, value := range values {
			matchBytes += len(value)
		}
		if matchValues > MaxRoutingValues || matchBytes > MaxRoutingMatchBytes {
			return nil, errorsWithField("routing.rules", "aggregate match budget exceeded")
		}
		enabled := *source.Enabled
		result.Rules[i] = NodeRoute{ID: source.ID, Enabled: &enabled, Remark: source.Remark,
			Kind: source.Kind, Values: values, OutboundID: source.OutboundID}
	}
	if len(missing) > 0 {
		refs := make([]string, 0, len(missing))
		for id, where := range missing {
			refs = append(refs, fmt.Sprintf("%q referenced by %s", id, strings.Join(where, ", ")))
		}
		sort.Strings(refs)
		return nil, errorsWithField("routing", "unknown outbound references: "+strings.Join(refs, "; "))
	}
	canonical, err := json.Marshal(result)
	if err != nil {
		return nil, errorsWithField("routing", fmt.Sprintf("cannot encode policy: %v", err))
	}
	if len(canonical) > MaxRoutingJSONBytes {
		return nil, errorsWithField("routing", "serialized policy exceeds 1 MiB")
	}
	return result, nil
}

// encoding/json replaces invalid UTF-8 with U+FFFD; reject it rather than
// silently changing connection details. Plugin options are JSON-validated
// separately when cloned.
func validateOutboundUTF8(field string, outbound NodeOutbound) error {
	value, typ := reflect.ValueOf(outbound), reflect.TypeOf(outbound)
	for i := 0; i < value.NumField(); i++ {
		v := value.Field(i)
		switch v.Kind() {
		case reflect.String:
			if !utf8.ValidString(v.String()) {
				return errorsWithField(field+"."+typ.Field(i).Name, "must be valid UTF-8")
			}
		case reflect.Slice:
			if v.Type().Elem().Kind() == reflect.String {
				for j := 0; j < v.Len(); j++ {
					if !utf8.ValidString(v.Index(j).String()) {
						return errorsWithField(field+"."+typ.Field(i).Name, "must be valid UTF-8")
					}
				}
			}
		}
	}
	return nil
}

func validRoutingID(id, prefix string) bool {
	return strings.HasPrefix(id, prefix) && routingUUID.MatchString(strings.TrimPrefix(id, prefix))
}

func routingText(field, text string, max int, emptyAllowed bool) error {
	if !utf8.ValidString(text) || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return errorsWithField(field, "must be valid UTF-8 without control characters")
	}
	if !emptyAllowed && text == "" {
		return errorsWithField(field, "must not be blank")
	}
	if utf8.RuneCountInString(text) > max {
		return errorsWithField(field, fmt.Sprintf("exceeds %d characters", max))
	}
	return nil
}
