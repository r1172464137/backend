package config

// asV3 completes a test policy with the format 3 fields its test does not
// exercise: version 3, empty account scopes and defaults, the system DNS
// default and no IP preference.
func asV3(p *NodeRouting) *NodeRouting {
	p.Version = RoutingVersion
	for i := range p.Rules {
		if p.Rules[i].UserIDs == nil {
			p.Rules[i].UserIDs = []int64{}
		}
	}
	if p.UserDefaults == nil {
		p.UserDefaults = []NodeUserDefault{}
	}
	if p.DNS == nil {
		p.DNS = DefaultDNSPolicy("test")
	}
	if p.TargetIPPreference == "" {
		p.TargetIPPreference = "none"
	}
	return p
}
