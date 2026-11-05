// Package handlertest drives the platform's Hertz handlers in their
// behaviour tests: it performs a request on a registered route and decodes
// the JSON envelope the handlers answer with, and it records the calls a
// fake facade port receives. Only tests import it.
package handlertest

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
// as its data; a nil want means the envelope has no data.
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
	JSONEqual(t, r.Data, want)
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

// JSONEqual fails the test unless got is the JSON encoding of want, or the
// JSON document want when it is a string.
func JSONEqual(t testing.TB, got json.RawMessage, want any) {
	t.Helper()
	wantJSON, ok := want.(string)
	if !ok {
		encoded, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		wantJSON = string(encoded)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("data %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(wantJSON), &wantValue); err != nil {
		t.Fatalf("want %q: %v", wantJSON, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		var compact bytes.Buffer
		_ = json.Compact(&compact, got)
		t.Fatalf("data = %s, want %s", compact.String(), wantJSON)
	}
}

// Call is one call a fake port received: the method and its request, nil
// for a method without one.
type Call struct {
	Method  string
	Request any
}

// Recorder is the state of a fake port: the calls it received, the answer
// of each method and the error every call fails with when set.
type Recorder struct {
	Calls   []Call
	Answers map[string]any
	Err     error
}

// Answer records the call of method with req and answers the recorder's
// answer for method, or its error.
func Answer[T any](r *Recorder, method string, req any) (*T, error) {
	r.Calls = append(r.Calls, Call{Method: method, Request: req})
	if r.Err != nil {
		return nil, r.Err
	}
	answer, _ := r.Answers[method].(*T)
	return answer, nil
}

// Do records the call of a method that answers only an error.
func (r *Recorder) Do(method string, req any) error {
	r.Calls = append(r.Calls, Call{Method: method, Request: req})
	return r.Err
}

// Called fails the test unless the port received exactly one call, of
// method with a request equal to req (nil for a method without one).
func (r *Recorder) Called(t testing.TB, method string, req any) {
	t.Helper()
	if len(r.Calls) != 1 || r.Calls[0].Method != method {
		t.Fatalf("calls = %+v, want one call of %s", r.Calls, method)
	}
	if got := r.Calls[0].Request; !reflect.DeepEqual(got, req) {
		t.Fatalf("%s request = %#v, want %#v", method, got, req)
	}
}
