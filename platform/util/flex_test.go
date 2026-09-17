// util/flex_test.go

package util

import (
	"encoding/json"
	"testing"
)

func TestFlexUnmarshalTolerantShapes(t *testing.T) {
	cases := []struct {
		raw       string
		wantInt   FlexInt
		wantFloat FlexFloat
	}{
		{`5`, 5, 5},
		{`"5"`, 5, 5},
		{`"5.7"`, 5, 5.7},
		{`5.7`, 5, 5.7},
		{`null`, 0, 0},
		{`""`, 0, 0},
		{`"NaN"`, 0, 0},
		{`" 12 "`, 12, 12},
		{`-3`, -3, -3},
	}

	for _, c := range cases {
		var i FlexInt
		if err := json.Unmarshal([]byte(c.raw), &i); err != nil {
			t.Errorf("FlexInt(%s): %v", c.raw, err)
		} else if i != c.wantInt {
			t.Errorf("FlexInt(%s) = %d, want %d", c.raw, i, c.wantInt)
		}

		var f FlexFloat
		if err := json.Unmarshal([]byte(c.raw), &f); err != nil {
			t.Errorf("FlexFloat(%s): %v", c.raw, err)
		} else if f != c.wantFloat {
			t.Errorf("FlexFloat(%s) = %v, want %v", c.raw, f, c.wantFloat)
		}
	}
}

func TestFlexRejectsNonNumbers(t *testing.T) {
	var i FlexInt
	if err := json.Unmarshal([]byte(`"abc"`), &i); err == nil {
		t.Error("expected an error for a non-numeric string")
	}
}
