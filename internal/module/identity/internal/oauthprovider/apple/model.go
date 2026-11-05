package apple

// CallbackRequest Apple Callback Request
type CallbackRequest struct {
	// Code is the authorization code received from your application’s user agent.
	// The code is single use only and valid for five minutes.
	Code    string `form:"code"`
	IdToken string `form:"id_token"`
	State   string `form:"state"`
}

// ValidationResponse is based off of https://developer.apple.com/documentation/signinwithapplerestapi/tokenresponse
type ValidationResponse struct {
	// (Reserved for future use) A token used to access allowed data. Currently, no data set has been defined for access.
	AccessToken string `json:"access_token"`

	// The type of access token. It will always be "bearer".
	TokenType string `json:"token_type"`

	// The amount of time, in seconds, before the access token expires. You can revalidate with the "RefreshToken"
	ExpiresIn int `json:"expires_in"`

	// The refresh token used to regenerate new access tokens. Store this token securely on your server.
	RefreshToken string `json:"refresh_token"`

	// A JSON Web Token that contains the user’s identity information.
	IDToken string `json:"id_token"`

	// Used to capture any error returned by the endpoint. Do not trust the response if this error is not nil
	Error string `json:"error"`

	// A more detailed precision about the current error.
	ErrorDescription string `json:"error_description"`
}
