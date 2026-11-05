package slicesx

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseInt64CSV(t *testing.T) {
	for input, want := range map[string][]int64{
		"":             nil,
		"   ":          nil,
		"7":            {7},
		"1,2,3":        {1, 2, 3},
		" 1 , 2 ,3 ":   {1, 2, 3},
		"-4,0":         {-4, 0},
		"9,9":          {9, 9},
		"123456789012": {123456789012},
	} {
		got, err := ParseInt64CSV(input)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("ParseInt64CSV(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
}

// One damaged element fails the whole list: returning the rest would turn a
// coupon limited to some plans into one for every plan.
func TestParseInt64CSVRejectsDamagedLists(t *testing.T) {
	for _, input := range []string{"abc", "1,x", "1,,2", "1,2,", ",1", "1.5", "99999999999999999999"} {
		if got, err := ParseInt64CSV(input); err == nil || got != nil {
			t.Fatalf("ParseInt64CSV(%q) = %v, %v; want an error and no ids", input, got, err)
		}
	}
	_, err := ParseInt64CSV("1,x")
	if err == nil || !strings.Contains(err.Error(), "element 2") {
		t.Fatalf("error = %v, want the position of the bad element", err)
	}
}

func TestInt64ListRoundTrip(t *testing.T) {
	ids := []int64{3, 1, 2}
	got, err := ParseInt64CSV(Int64SliceToString(ids))
	if err != nil || !reflect.DeepEqual(got, ids) {
		t.Fatalf("round trip = %v, %v; want %v", got, err, ids)
	}
	if !reflect.DeepEqual(Int64SliceToStringSlice(ids), []string{"3", "1", "2"}) {
		t.Fatal("Int64SliceToStringSlice changed the ids")
	}
}

func TestSliceHelpersPreserveOrderAndDoNotMutateInput(t *testing.T) {
	input := []string{"b", "", "a", "b", "c"}
	if got := RemoveDuplicateElements(input...); !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("deduplication changed semantics: %v", got)
	}
	if !reflect.DeepEqual(input, []string{"b", "", "a", "b", "c"}) {
		t.Fatal("input was modified")
	}
	if got := RemoveDuplicateElements(0, 1, 0, 2); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("zero integer removed: %v", got)
	}
}
