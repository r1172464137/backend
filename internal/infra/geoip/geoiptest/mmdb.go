// Package geoiptest builds minimal MaxMind databases for tests, so code that
// reads GeoIP data can be tested without the real (downloaded) databases.
package geoiptest

import (
	"bytes"
	"sort"
)

// MMDB encodes an IPv4 MaxMind DB of databaseType whose search tree has a
// single node: addresses below 128.0.0.0 find low, the others high. A nil
// record leaves its half of the address space without data.
func MMDB(databaseType string, low, high map[string]any) []byte {
	const nodeCount = 1
	var data encoder
	// A record value above the node count points into the data section at
	// offset value - node count - 16 (the separator); the node count itself
	// means "no data".
	pointer := func(record map[string]any) uint32 {
		if record == nil {
			return nodeCount
		}
		offset := uint32(data.Len())
		data.value(record)
		return nodeCount + 16 + offset
	}
	left, right := pointer(low), pointer(high)

	var metadata encoder
	metadata.value(map[string]any{
		"binary_format_major_version": uint16(2),
		"binary_format_minor_version": uint16(0),
		"build_epoch":                 uint64(1700000000),
		"database_type":               databaseType,
		"description":                 map[string]any{"en": "test database"},
		"ip_version":                  uint16(4),
		"languages":                   []string{"en"},
		"node_count":                  uint32(nodeCount),
		"record_size":                 uint16(24),
	})

	var db bytes.Buffer
	db.Write(record24(left))
	db.Write(record24(right))
	db.Write(make([]byte, 16))
	db.Write(data.Bytes())
	db.WriteString("\xAB\xCD\xEFMaxMind.com")
	db.Write(metadata.Bytes())
	return db.Bytes()
}

func record24(v uint32) []byte {
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}

// encoder writes the MaxMind DB data format for the handful of types the
// test databases use; every size stays below 29.
type encoder struct {
	bytes.Buffer
}

func (e *encoder) control(typ byte, size int) {
	if size >= 29 {
		panic("geoiptest encodes sizes below 29 only")
	}
	if typ <= 7 {
		e.WriteByte(typ<<5 | byte(size))
		return
	}
	// Extended type: zero type bits, then the type number minus 7.
	e.WriteByte(byte(size))
	e.WriteByte(typ - 7)
}

func (e *encoder) unsigned(typ byte, v uint64) {
	var raw []byte
	for ; v > 0; v >>= 8 {
		raw = append([]byte{byte(v)}, raw...)
	}
	e.control(typ, len(raw))
	e.Write(raw)
}

func (e *encoder) value(v any) {
	switch v := v.(type) {
	case string:
		e.control(2, len(v))
		e.WriteString(v)
	case uint16:
		e.unsigned(5, uint64(v))
	case uint32:
		e.unsigned(6, uint64(v))
	case uint64:
		e.unsigned(9, v)
	case []string:
		e.control(11, len(v))
		for _, item := range v {
			e.value(item)
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		e.control(7, len(v))
		for _, key := range keys {
			e.value(key)
			e.value(v[key])
		}
	default:
		panic("geoiptest cannot encode this value")
	}
}
