package random

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// EncodeBase62 generates user invite codes, which are stored and shared:
// these values, taken from the implementation before its clean-up, must
// never change. They include the quirks — the unpadded zero, the padding
// that lets 916132831 and 56800235583 collide, and negative input.
func TestEncodeBase62Golden(t *testing.T) {
	for _, tt := range []struct {
		id   int64
		want string
	}{
		{0, "E"},
		{1, "wT2ey7"},
		{2, "wT2eyg"},
		{61, "wT2eyy"},
		{62, "T2ey7E"},
		{63, "T2ey77"},
		{3843, "T2eyyy"},
		{3844, "2ey7EE"},
		{238327, "2eyyyy"},
		{238328, "ey7EEE"},
		{14776335, "eyyyyy"},
		{14776336, "y7EEEE"},
		{916132831, "yyyyyy"},
		{916132832, "7EEEEE"},
		{1112275807, "75xyJN"},
		{56800235583, "yyyyyy"},
		{56800235584, "7EEEEEE"},
		{1790000000042, "uunMhgV"},
		{3521614606207, "yyyyyyy"},
		{3521614606208, "7EEEEEEE"},
		{9223372036854775807, "ky9SwEZ4SCW"},
		{-1, "JwT2ey"},
		{-62, "JwT2ey"},
	} {
		if got := EncodeBase62(tt.id); got != tt.want {
			t.Errorf("EncodeBase62(%d) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestEncodeBase62(t *testing.T) {
	start := 1112275807
	length := 10000
	n := length + start
	// n := 328564998144
	m := make(map[string]struct{})
	// m := make(map[string]struct{}, length)
	var inviteCode string
	for i := start; i < n; i++ {
		inviteCode = EncodeBase62(int64(i))
		if v, ok := m[inviteCode]; ok {
			t.Fatal(v, inviteCode)
		}
		m[inviteCode] = struct{}{}
	}
	t.Log(inviteCode)

	assert.Equal(t, length, len(m))
}

func TestInt64ToDashedString(t *testing.T) {
	type args struct {
		strNum string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "",
			args: args{
				strNum: "123",
			},
			want: "123",
		},
		{
			name: "",
			args: args{
				strNum: "1234",
			},
			want: "1234",
		},
		{
			name: "",
			args: args{
				strNum: "12345",
			},
			want: "1234-5",
		},
		{
			name: "",
			args: args{
				strNum: "12345678",
			},
			want: "1234-5678",
		},
		{
			name: "",
			args: args{
				strNum: "123456789",
			},
			want: "1234-5678-9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, StrToDashedString(tt.args.strNum), "StrToDashedString(%v)", tt.args.strNum)
		})
	}
}
