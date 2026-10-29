package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// ValidatePluginOptionsWire rejects numbers that protobuf Value would round,
// so every accepted policy is semantically identical in JSON and protobuf.
func ValidatePluginOptionsWire(options any) error {
	if options == nil {
		return nil
	}
	original, err := json.Marshal(options)
	if err != nil {
		return err
	}
	value := &structpb.Value{}
	if err := protojson.Unmarshal(original, value); err != nil {
		return err
	}
	wire, err := protojson.Marshal(value)
	if err != nil {
		return err
	}
	decode := func(raw []byte) (any, error) {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var out any
		err := d.Decode(&out)
		return out, err
	}
	a, err := decode(original)
	if err != nil {
		return err
	}
	b, err := decode(wire)
	if err != nil {
		return err
	}
	if !equalJSONValues(a, b) {
		return fmt.Errorf("cannot be represented losslessly as protobuf Value")
	}
	return nil
}
func equalJSONValues(left, right any) bool {
	switch a := left.(type) {
	case json.Number:
		b, ok := right.(json.Number)
		if !ok || len(a) > 128 || len(b) > 128 {
			return false
		}
		x, xok := new(big.Rat).SetString(string(a))
		y, yok := new(big.Rat).SetString(string(b))
		return xok && yok && x.Cmp(y) == 0
	case []any:
		b, ok := right.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !equalJSONValues(a[i], b[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		b, ok := right.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for key, v := range a {
			other, ok := b[key]
			if !ok || !equalJSONValues(v, other) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(left, right)
	}
}
