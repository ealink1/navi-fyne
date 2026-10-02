package domain

import (
	"reflect"
	"testing"
)

func TestDatabaseVisibilityCombinesAllowIncludeAndExclude(t *testing.T) {
	p := Profile{DatabaseAllow: []string{"tenant_a", "tenant_b", "other"}, DatabaseInclude: []string{"tenant_%"}, DatabaseExclude: []string{"*_b"}}
	got := p.VisibleDatabases([]string{"tenant_a", "tenant_b", "tenant_c", "other"})
	if !reflect.DeepEqual(got, []string{"tenant_a"}) {
		t.Fatal(got)
	}
	if !matchName([]string{`literal\_name`}, "literal_name") || matchName([]string{`literal\_name`}, "literalXname") {
		t.Fatal("escaped wildcard changed meaning")
	}
	if matchName([]string{"[unsafe].*"}, "unsafeAAAA") {
		t.Fatal("literal regex metacharacters executed")
	}
}
func TestPresentationRejectsUntrustedAssetValues(t *testing.T) {
	for _, p := range []Profile{{IconColor: "url(http://example.test/)"}, {IconType: "../../file"}, {DatabaseInclude: []string{string(make([]byte, 257))}}} {
		if p.ValidatePresentation() == nil {
			t.Fatal("unbounded presentation accepted")
		}
	}
}
