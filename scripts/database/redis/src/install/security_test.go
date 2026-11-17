package install

import (
	"strings"
	"testing"
)

func TestBuildRedisDockerConf(t *testing.T) {
	conf := BuildRedisDockerConf(true, "s\"ecret\n\\pw")
	if !strings.Contains(conf, "requirepass ") {
		t.Fatalf("expected requirepass directive in redis.conf, got:\n%s", conf)
	}
	if strings.Contains(conf, "\npw") {
		t.Fatalf("newline in password was not stripped from redis.conf: %q", conf)
	}

	noAuth := BuildRedisDockerConf(false, "ignored")
	if strings.Contains(noAuth, "requirepass") {
		t.Fatalf("unexpected requirepass when requirePass=false: %q", noAuth)
	}
}
