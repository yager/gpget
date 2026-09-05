package config

import "testing"

func TestNormalizeSidecars(t *testing.T) {
	ok := []struct{ in, want string }{
		{"gpr,lrv", "gpr,lrv"},
		{"all", "all"},
		{"none", "none"},
		{" lrv , gpr ", "lrv,gpr"},
	}
	for _, tc := range ok {
		got, err := normalizeSidecars(tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q → %q, want %q", tc.in, got, tc.want)
		}
	}
	if _, err := normalizeSidecars("gpr.lrv"); err == nil {
		t.Fatal("typo gpr.lrv should be rejected")
	}
	c := Defaults()
	if err := c.Set("general.sidecars", "gpr.lrv"); err == nil {
		t.Fatal("Set should reject gpr.lrv")
	}
}
