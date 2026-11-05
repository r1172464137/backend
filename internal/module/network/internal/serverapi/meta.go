package serverapi

// RequestMeta is the conditional-request state a node sends with a pull.
type RequestMeta struct {
	// IfNoneMatch is the ETag of the node's cached copy, if any.
	IfNoneMatch string
}

// ResponseMeta is the response headers a pull sets, such as the ETag.
type ResponseMeta struct {
	Headers map[string]string
}

// NewResponseMeta returns a ResponseMeta without headers.
func NewResponseMeta() ResponseMeta {
	return ResponseMeta{Headers: make(map[string]string)}
}

// SetHeader sets the response header key to value.
func (m *ResponseMeta) SetHeader(key, value string) {
	if m.Headers == nil {
		m.Headers = make(map[string]string)
	}
	m.Headers[key] = value
}
