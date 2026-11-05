package httpx

// ResponseSuccessBean is the body of a successful response.
type ResponseSuccessBean struct {
	Code uint32 `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
} // @name result.ResponseSuccessBean

// Success wraps data in the success envelope: code 200, message "success".
func Success(data any) *ResponseSuccessBean {
	return &ResponseSuccessBean{200, "success", data}
}

// ResponseErrorBean is the body of a failed response.
type ResponseErrorBean struct {
	Code uint32 `json:"code"`
	Msg  string `json:"msg"`
} // @name result.ResponseErrorBean

// Error builds the failure envelope for errCode and errMsg.
func Error(errCode uint32, errMsg string) *ResponseErrorBean {
	return &ResponseErrorBean{errCode, errMsg}
}
