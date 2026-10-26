package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Route-rule and persistence limits bound the admin API and keep every value
// safely below MySQL TEXT's 65,535-byte ceiling after JSON escaping.
const (
	MaxRouteRules                 = 1024
	MaxRouteRuleBytes             = 4096
	MaxNodeOutbounds              = 128
	MaxOutboundAggregateRules     = 4096
	MaxOutboundAggregateRuleBytes = 48 * 1024
	MaxPersistedJSONBytes         = 60 * 1024
)

var (
	// These grammars are shared with the node route-rule implementation.
	safeGeoSiteLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.!-]*(?:@[A-Za-z0-9_!-]+)*$`)
	safeGeoIPLabel   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	domainLabel      = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
)

// RouteRuleError identifies the exact API field and one-based array/text line
// that contains an invalid structured route rule.
type RouteRuleError struct {
	Field   string
	Line    int
	Message string
}

func (e *RouteRuleError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s line %d: %s", e.Field, e.Line, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// LegacyOutboundMatchInput is the minimal cross-DTO view used to preserve
// additive outbound fields submitted by clients that only know legacy fields.
type LegacyOutboundMatchInput struct {
	Name  string
	Value any
}

// NormalizeDomainRules validates domain/GeoSite/RE2 rules and returns their
// Xray canonical forms. Bare domains are exact matches, not keywords.
func NormalizeDomainRules(field string, values []string) ([]string, error) {
	return normalizeRouteRules(field, values, normalizeDomainRule)
}

// NormalizeIPRules validates GeoIP labels and netip addresses/prefixes.
func NormalizeIPRules(field string, values []string) ([]string, error) {
	return normalizeRouteRules(field, values, normalizeIPRule)
}

// NormalizeProtocolRules currently allows only Xray's BitTorrent detector.
func NormalizeProtocolRules(field string, values []string) ([]string, error) {
	return normalizeRouteRules(field, values, normalizeProtocolRule)
}

func normalizeRouteRules(field string, values []string, normalize func(string) (string, error)) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if !utf8.ValidString(value) {
			return nil, routeRuleError(field, index, "is not valid UTF-8")
		}
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return nil, routeRuleError(field, index, "contains a control character or embedded newline")
		}
		canonical, err := normalize(value)
		if err != nil {
			return nil, routeRuleError(field, index, err.Error())
		}
		if len(canonical) > MaxRouteRuleBytes {
			return nil, routeRuleError(field, index, fmt.Sprintf("canonical rule exceeds %d bytes", MaxRouteRuleBytes))
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		result = append(result, canonical)
		if len(result) > MaxRouteRules {
			return nil, &RouteRuleError{Field: field, Message: fmt.Sprintf("contains more than %d rules", MaxRouteRules)}
		}
	}
	return result, nil
}

func normalizeDomainRule(value string) (string, error) {
	prefix, payload, hasPrefix := strings.Cut(value, ":")
	if !hasPrefix {
		canonical, err := normalizeDomainValue(value, "exact domain")
		if err != nil {
			return "", err
		}
		return "full:" + canonical, nil
	}

	switch strings.ToLower(prefix) {
	case "full":
		canonical, err := normalizeDomainValue(payload, "full domain")
		if err != nil {
			return "", err
		}
		return "full:" + canonical, nil
	case "keyword":
		if payload == "" {
			return "", fmt.Errorf("keyword is empty")
		}
		if strings.IndexFunc(payload, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return "", fmt.Errorf("keyword must not contain whitespace or control characters")
		}
		return "keyword:" + payload, nil
	case "suffix", "domain":
		canonical, err := normalizeDomainValue(payload, "domain suffix")
		if err != nil {
			return "", err
		}
		return "domain:" + canonical, nil
	case "regex", "regexp":
		if payload == "" {
			return "", fmt.Errorf("regular expression is empty")
		}
		if _, err := regexp.Compile(payload); err != nil {
			return "", fmt.Errorf("regular expression is not supported by RE2: %v", err)
		}
		return "regexp:" + payload, nil
	case "geosite":
		if !safeGeoSiteLabel.MatchString(payload) || len(payload) > 253 {
			return "", fmt.Errorf("GeoSite label %q must match %s", payload, safeGeoSiteLabel.String())
		}
		return "geosite:" + payload, nil
	default:
		return "", fmt.Errorf("unknown domain rule prefix %q", prefix)
	}
}

func normalizeIPRule(value string) (string, error) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		if prefix.Addr().Zone() != "" {
			return "", fmt.Errorf("zoned IP prefixes are not supported")
		}
		return prefix.Masked().String(), nil
	}
	if address, err := netip.ParseAddr(value); err == nil {
		if address.Zone() != "" {
			return "", fmt.Errorf("zoned IP addresses are not supported")
		}
		return address.String(), nil
	}
	prefix, payload, hasPrefix := strings.Cut(value, ":")
	if hasPrefix && strings.EqualFold(prefix, "geoip") {
		if !safeGeoIPLabel.MatchString(payload) || len(payload) > 253 {
			return "", fmt.Errorf("GeoIP label %q must match %s", payload, safeGeoIPLabel.String())
		}
		return "geoip:" + payload, nil
	}
	if strings.Contains(value, "/") {
		return "", fmt.Errorf("%q is not a valid CIDR prefix", value)
	}
	if hasPrefix {
		return "", fmt.Errorf("unknown IP rule prefix %q", prefix)
	}
	return "", fmt.Errorf("%q is not a valid IP address or CIDR prefix", value)
}

func normalizeProtocolRule(value string) (string, error) {
	canonical := strings.ToLower(value)
	if canonical != "bittorrent" {
		return "", fmt.Errorf("unsupported protocol %q (allowed: bittorrent)", value)
	}
	return canonical, nil
}

func normalizeDomainValue(value, kind string) (string, error) {
	original := value
	if value == "" {
		return "", fmt.Errorf("%s is empty", kind)
	}
	if strings.HasSuffix(value, ".") {
		value = strings.TrimSuffix(value, ".")
		if strings.HasSuffix(value, ".") {
			return "", fmt.Errorf("%q has repeated trailing DNS root dots", original)
		}
	}
	if value == "" {
		return "", fmt.Errorf("%s is empty after trimming the DNS root dot", kind)
	}
	if strings.EqualFold(value, "bittorrent") {
		return "", fmt.Errorf("bittorrent is a protocol and belongs in a protocol_rules/block_protocols field")
	}
	if prefix, err := netip.ParsePrefix(value); err == nil && prefix.IsValid() {
		return "", fmt.Errorf("%q is an IP prefix and belongs in an IP rule field", value)
	}
	if address, err := netip.ParseAddr(value); err == nil && address.IsValid() {
		return "", fmt.Errorf("%q is an IP address and belongs in an IP rule field", value)
	}
	if strings.Contains(value, "/") {
		return "", fmt.Errorf("%q looks like an IP prefix and belongs in an IP rule field", value)
	}
	if len(value) > 253 {
		return "", fmt.Errorf("%s exceeds 253 bytes", kind)
	}
	if strings.ContainsAny(value, " \t\r\n:#") {
		return "", fmt.Errorf("%q is not a valid %s", value, kind)
	}
	for _, label := range strings.Split(value, ".") {
		if !domainLabel.MatchString(label) {
			return "", fmt.Errorf("domain label %q in %q is malformed", label, value)
		}
	}
	return value, nil
}

// NormalizeOutboundNames trims and validates the stable outbound identity used
// for omission-safe matching. Names are case-sensitive but trim-equivalent
// duplicates are rejected.
func NormalizeOutboundNames(field string, names []string) ([]string, error) {
	if len(names) > MaxNodeOutbounds {
		return nil, &RouteRuleError{Field: field, Message: fmt.Sprintf("contains more than %d outbounds", MaxNodeOutbounds)}
	}
	result := make([]string, len(names))
	seen := make(map[string]int, len(names))
	for index, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, routeRuleError(field, index, "outbound name must not be blank")
		}
		if first, exists := seen[name]; exists {
			return nil, routeRuleError(field, index, fmt.Sprintf("outbound name %q duplicates line %d after trimming", name, first+1))
		}
		seen[name] = index
		result[index] = name
	}
	return result, nil
}

// NormalizeOutboundProtocols trims legacy-visible outbound protocol values and
// rejects malformed entries before any omission-preservation matching.
func NormalizeOutboundProtocols(field string, protocols []string) ([]string, error) {
	result := make([]string, len(protocols))
	for index, raw := range protocols {
		protocol := strings.TrimSpace(raw)
		if protocol == "" {
			return nil, routeRuleError(field, index, "outbound protocol must not be blank")
		}
		if strings.IndexFunc(protocol, unicode.IsControl) >= 0 {
			return nil, routeRuleError(field, index, "outbound protocol contains a control character")
		}
		result[index] = protocol
	}
	return result, nil
}

// MatchLegacyOutbounds deterministically maps submitted outbounds to existing
// values without ever assigning hidden fields by an ambiguous position.
// Result entries are existing indexes, or -1 for demonstrably new outbounds.
func MatchLegacyOutbounds(field string, next, current []LegacyOutboundMatchInput) ([]int, error) {
	names := make([]string, len(next))
	for index := range next {
		names[index] = next[index].Name
	}
	names, err := NormalizeOutboundNames(field, names)
	if err != nil {
		return nil, err
	}
	for index := range next {
		next[index].Name = names[index]
	}
	matches := make([]int, len(next))
	for index := range matches {
		matches[index] = -1
	}
	if len(next) == 0 || len(current) == 0 {
		return matches, nil
	}

	currentNames := make(map[string][]int, len(current))
	for index, item := range current {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			currentNames[name] = append(currentNames[name], index)
		}
	}
	used := make(map[int]struct{}, len(current))
	for index, item := range next {
		candidates := unusedIndexes(currentNames[strings.TrimSpace(item.Name)], used)
		if len(candidates) == 1 {
			matches[index] = candidates[0]
			used[candidates[0]] = struct{}{}
		}
	}

	nextFingerprints := make(map[string][]int)
	currentFingerprints := make(map[string][]int)
	for index, item := range next {
		if matches[index] >= 0 {
			continue
		}
		fingerprint, err := legacyOutboundFingerprint(item.Value)
		if err != nil {
			return nil, errorsWithField(field, fmt.Sprintf("fingerprint outbound line %d: %v", index+1, err))
		}
		nextFingerprints[fingerprint] = append(nextFingerprints[fingerprint], index)
	}
	for index, item := range current {
		if _, exists := used[index]; exists {
			continue
		}
		fingerprint, err := legacyOutboundFingerprint(item.Value)
		if err != nil {
			return nil, errorsWithField(field, fmt.Sprintf("fingerprint existing outbound line %d: %v", index+1, err))
		}
		currentFingerprints[fingerprint] = append(currentFingerprints[fingerprint], index)
	}
	for fingerprint, nextIndexes := range nextFingerprints {
		currentIndexes := currentFingerprints[fingerprint]
		if len(nextIndexes) != 1 || len(currentIndexes) != 1 {
			continue
		}
		nextIndex, currentIndex := nextIndexes[0], currentIndexes[0]
		matches[nextIndex] = currentIndex
		used[currentIndex] = struct{}{}
	}

	unmatchedNext := make([]int, 0)
	for index, match := range matches {
		if match < 0 {
			unmatchedNext = append(unmatchedNext, index)
		}
	}
	unmatchedCurrent := make([]int, 0)
	for index := range current {
		if _, exists := used[index]; !exists {
			unmatchedCurrent = append(unmatchedCurrent, index)
		}
	}
	if len(unmatchedCurrent) == 0 {
		return matches, nil // every unmatched submitted item is a true insertion
	}
	if len(unmatchedNext) == 0 {
		return matches, nil // unmatched existing items were deleted
	}

	// Position is safe only by elimination: equal-length sequences, at least
	// one deterministic anchor, all anchors stayed at their positions, and one
	// remaining item occupies the same position. A rename plus simultaneous
	// legacy-field edit without those facts is intentionally rejected.
	anchored := len(next) - len(unmatchedNext)
	sequenceUnchanged := len(next) == len(current) && anchored > 0 && len(unmatchedNext) == 1 && len(unmatchedCurrent) == 1
	if sequenceUnchanged {
		for nextIndex, currentIndex := range matches {
			if currentIndex >= 0 && nextIndex != currentIndex {
				sequenceUnchanged = false
				break
			}
		}
	}
	if sequenceUnchanged && unmatchedNext[0] == unmatchedCurrent[0] {
		matches[unmatchedNext[0]] = unmatchedCurrent[0]
		return matches, nil
	}

	index := unmatchedNext[0]
	return nil, routeRuleError(field, index, "cannot safely match this outbound to preserve hidden ip_rules/protocol_rules; submit all structured fields with an upgraded client")
}

func unusedIndexes(indexes []int, used map[int]struct{}) []int {
	result := make([]int, 0, len(indexes))
	for _, index := range indexes {
		if _, exists := used[index]; !exists {
			result = append(result, index)
		}
	}
	return result
}

func legacyOutboundFingerprint(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return "", err
	}
	delete(fields, "name")
	delete(fields, "ip_rules")
	delete(fields, "protocol_rules")
	canonical, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

// ValidatePersistedJSON validates the exact JSON encoding stored in a TEXT
// field, including escaping and object metadata.
func ValidatePersistedJSON(field string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return errorsWithField(field, fmt.Sprintf("cannot be encoded as JSON: %v", err))
	}
	return ValidateSerializedJSON(field, string(encoded))
}

// ValidateSerializedJSON validates an already encoded JSON database value.
func ValidateSerializedJSON(field, value string) error {
	if len(value) >= MaxPersistedJSONBytes {
		return errorsWithField(field, fmt.Sprintf("serialized JSON is %d bytes; it must be below %d bytes", len(value), MaxPersistedJSONBytes))
	}
	return nil
}

// ValidateOutboundAggregate enforces limits across every route category in
// the complete outbound list. Final serialized JSON is still decisive.
func ValidateOutboundAggregate(field string, outboundCount, ruleCount, ruleBytes int) error {
	if outboundCount > MaxNodeOutbounds {
		return errorsWithField(field, fmt.Sprintf("contains more than %d outbounds", MaxNodeOutbounds))
	}
	if ruleCount > MaxOutboundAggregateRules {
		return errorsWithField(field, fmt.Sprintf("contains more than %d aggregate route rules", MaxOutboundAggregateRules))
	}
	if ruleBytes > MaxOutboundAggregateRuleBytes {
		return errorsWithField(field, fmt.Sprintf("route rules contain more than %d aggregate bytes", MaxOutboundAggregateRuleBytes))
	}
	return nil
}

func errorsWithField(field, message string) error {
	return &RouteRuleError{Field: field, Message: message}
}

func routeRuleError(field string, zeroBasedLine int, message string) *RouteRuleError {
	return &RouteRuleError{Field: field, Line: zeroBasedLine + 1, Message: message}
}
