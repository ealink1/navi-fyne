package sqlparam

import (
	"math"
	"reflect"
	"testing"
)

func TestPostgresEscapedStringAndMultipleTemplateNames(t *testing.T) {
	names := MissingParameterNames(`SELECT E'it\'s :fake', :real`, "postgres", nil)
	if !reflect.DeepEqual(names, []string{"real"}) {
		t.Fatal(names)
	}
	names = MissingParameterNames(`SELECT '{first}-{second}'`, "postgres", nil)
	if !reflect.DeepEqual(names, []string{"first", "second"}) {
		t.Fatal(names)
	}
}

func TestNumbersDoNotSilentlyRoundLargeIntegersOrAcceptNonFinite(t *testing.T) {
	got, err := ConvertTypedValue(TypeNumber, "18446744073709551615")
	if err != nil || got != "18446744073709551615" {
		t.Fatal(got, err)
	}
	for _, value := range []any{"null", "true", "[]", "{}", "NaN", "Infinity", math.NaN(), math.Inf(1)} {
		if _, err = ConvertTypedValue(TypeNumber, value); err == nil {
			t.Fatal("invalid numeric accepted")
		}
	}
}
