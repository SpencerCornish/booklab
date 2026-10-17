package config

import (
	"reflect"
	"testing"
)

func TestTrustedProxyPrefixes(t *testing.T) {
	field, _ := reflect.TypeOf(Config{}).FieldByName("TrustedProxies")
	def := field.Tag.Get("default")
	prefixes, err := (&Config{TrustedProxies: def}).TrustedProxyPrefixes()
	if err != nil || len(prefixes) != 6 {
		t.Fatalf("default %q: got %v, %v", def, prefixes, err)
	}

	prefixes, err = (&Config{TrustedProxies: " 203.0.113.9 , 2001:db8::/32,"}).TrustedProxyPrefixes()
	if err != nil {
		t.Fatal(err)
	}
	if len(prefixes) != 2 || prefixes[0].String() != "203.0.113.9/32" || prefixes[1].String() != "2001:db8::/32" {
		t.Fatalf("got %v", prefixes)
	}

	if prefixes, err := (&Config{}).TrustedProxyPrefixes(); err != nil || len(prefixes) != 0 {
		t.Fatalf("empty: got %v, %v", prefixes, err)
	}
	if _, err := (&Config{TrustedProxies: "10.0.0.0/8,nope"}).TrustedProxyPrefixes(); err == nil {
		t.Fatal("want error for invalid entry")
	}
}
