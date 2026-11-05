package smsbao

import "fmt"

// Error is a status code of the SMSBao API.
type Error int

// The status codes the API answers with.
const (
	Success Error = iota
	PasswordError
	AccountNotFount
	InsufficientBalance
	IPAddressRestrictions
	ContentContainsSensitiveWords
	MobileNumberIsIncorrect
)

var errorDescriptions = map[Error]string{
	Success:                       "Success",
	PasswordError:                 "Password error",
	AccountNotFount:               "Account not found",
	InsufficientBalance:           "Insufficient balance",
	IPAddressRestrictions:         "IP address restrictions",
	ContentContainsSensitiveWords: "Content contains sensitive words",
	MobileNumberIsIncorrect:       "Mobile number is incorrect",
}

var errorCodes = map[string]Error{
	"0":  Success,
	"30": PasswordError,
	"40": AccountNotFount,
	"41": InsufficientBalance,
	"43": IPAddressRestrictions,
	"50": ContentContainsSensitiveWords,
	"51": MobileNumberIsIncorrect,
}

// String describes the status code.
func (e Error) String() string {
	for k, v := range errorDescriptions {
		if k == e {
			return v
		}
	}
	return "Unknown error"
}

func parseError(b []byte) error {
	if e, ok := errorCodes[string(b)]; ok {
		return e.Error()
	}
	return fmt.Errorf("unknown error")
}

// Error returns the status as an error, nil for Success.
func (e Error) Error() error {
	if e == Success {
		return nil
	}
	return fmt.Errorf("%s", e.String())
}
