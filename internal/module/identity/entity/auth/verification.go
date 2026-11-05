package auth

// VerifyType identifies the purpose shared by identity's verification codes
// and the corresponding notification task.
type VerifyType uint8

const (
	// Register codes prove an identifier no account holds yet.
	Register VerifyType = iota + 1
	// Security codes prove an identifier an account holds.
	Security
)

// ParseVerifyType converts the type number of a request into its purpose.
func ParseVerifyType(i uint8) VerifyType {
	return VerifyType(i)
}

func (v VerifyType) String() string {
	switch v {
	case Register:
		return "register"
	case Security:
		return "security"
	default:
		return "unknown"
	}
}
