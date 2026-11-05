package password

import "testing"

// Every stored format other than argon2id is legacy: PPanel's PBKDF2, the
// imported md5, sha256 and bcrypt hashes and an empty hash alike.
func TestIsLegacyHash(t *testing.T) {
	for hash, want := range map[string]bool{
		EncodePassWord("password"):                false,
		"$pbkdf2-sha512$salt$hash":                true,
		"5f4dcc3b5aa765d61d8327deb882cf99":        true,
		"$2y$10$abcdefghijklmnopqrstuvwxyz012345": true,
		"": true,
	} {
		if got := IsLegacyHash(hash); got != want {
			t.Errorf("IsLegacyHash(%q) = %v, want %v", hash, got, want)
		}
	}
}
