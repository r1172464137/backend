package supporttest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

// Reply is a handler's answer: the HTTP status, the raw body and the
// envelope decoded from it.
type Reply struct {
	Status int             `json:"-"`
	Body   string          `json:"-"`
	Code   uint32          `json:"code"`
	Msg    string          `json:"msg"`
	Data   json.RawMessage `json:"data"`
}

// Serve performs method target on h, with body as a JSON document or, when
// body is empty, with no body, and decodes the JSON envelope of the answer.
func Serve(t testing.TB, h *server.Hertz, method, target, body string) Reply {
	t.Helper()
	var headers []ut.Header
	var content *ut.Body
	if body != "" {
		headers = append(headers, ut.Header{Key: "Content-Type", Value: "application/json"})
		content = &ut.Body{Body: strings.NewReader(body), Len: len(body)}
	}
	w := ut.PerformRequest(h.Engine, method, target, content, headers...)
	reply := Reply{Status: w.Code, Body: w.Body.String()}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatalf("%s %s answered %d %q, not a JSON envelope: %v", method, target, w.Code, reply.Body, err)
	}
	return reply
}

// OK fails the test unless the reply is the success envelope carrying want
// as its data: a JSON document, or a value to encode. A nil want means the
// envelope has no data.
func (r Reply) OK(t testing.TB, want any) {
	t.Helper()
	if r.Status != http.StatusOK || r.Code != http.StatusOK || r.Msg != "success" {
		t.Fatalf("reply = %d %s, want the success envelope", r.Status, r.Body)
	}
	if want == nil {
		if r.Data != nil {
			t.Fatalf("reply data = %s, want none", r.Data)
		}
		return
	}
	wantJSON, ok := want.(string)
	if !ok {
		encoded, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		wantJSON = string(encoded)
	}
	var got, expected any
	if err := json.Unmarshal(r.Data, &got); err != nil {
		t.Fatalf("data %q: %v", r.Data, err)
	}
	if err := json.Unmarshal([]byte(wantJSON), &expected); err != nil {
		t.Fatalf("want %q: %v", wantJSON, err)
	}
	if !reflect.DeepEqual(got, expected) {
		var compact bytes.Buffer
		_ = json.Compact(&compact, r.Data)
		t.Fatalf("data = %s, want %s", compact.String(), wantJSON)
	}
}

// Decode fails the test unless the reply is the success envelope, and
// decodes its data into v.
func (r Reply) Decode(t testing.TB, v any) {
	t.Helper()
	if r.Status != http.StatusOK || r.Code != http.StatusOK {
		t.Fatalf("reply = %d %s, want the success envelope", r.Status, r.Body)
	}
	if err := json.Unmarshal(r.Data, v); err != nil {
		t.Fatalf("data %s: %v", r.Data, err)
	}
}

// Refused fails the test unless the reply is HTTP 200 with an error envelope
// of code whose message contains msg.
func (r Reply) Refused(t testing.TB, code uint32, msg string) {
	t.Helper()
	if r.Status != http.StatusOK || r.Code != code || !strings.Contains(r.Msg, msg) || r.Data != nil {
		t.Fatalf("reply = %d %s, want code %d with a message containing %q", r.Status, r.Body, code, msg)
	}
}
