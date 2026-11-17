package modules

import "testing"

func TestValidateProxyTarget_Modules(t *testing.T) {
	if !validateProxyTarget("http://127.0.0.1:3000") {
		t.Fatalf("expected localhost upstream to pass")
	}
	for _, bad := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:3000; evil;",
		"http://127.0.0.1:3000\nproxy_pass http://evil",
	} {
		if validateProxyTarget(bad) {
			t.Errorf("validateProxyTarget(%q) = true, want false", bad)
		}
	}
}
