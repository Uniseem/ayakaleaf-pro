package config

import (
	"testing"
)

func TestMongoDatabase(t *testing.T) {
	cases := map[string]string{
		"mongodb://127.0.0.1/sharelatex":                       "sharelatex",
		"mongodb://mongo/sharelatex":                           "sharelatex",
		"mongodb://mongo:27017/sharelatex?replicaSet=overleaf": "sharelatex",
		"mongodb://user:p@ss/word@mongo:27017/sharelatex":      "sharelatex",
		"mongodb://a:27017,b:27017/sharelatex?replicaSet=rs0":  "sharelatex",
		"mongodb+srv://user:pw@cluster.example.com/sharelatex": "sharelatex",
		"mongodb://127.0.0.1":                                  "",
	}
	for uri, want := range cases {
		if got := MongoDatabase(uri); got != want {
			t.Errorf("MongoDatabase(%q) = %q, want %q", uri, got, want)
		}
	}
}

// server-ce sets neither MONGO_CONNECTION_STRING nor MONGO_HOST; its
// settings.js takes the URL from OVERLEAF_MONGO_URL, and a service that does
// not read it fails to reach Mongo at all.
func TestMongoURLPrecedence(t *testing.T) {
	t.Setenv("MONGO_CONNECTION_STRING", "")
	t.Setenv("OVERLEAF_MONGO_URL", "")
	t.Setenv("MONGO_HOST", "")
	if got, want := MongoURL(), "mongodb://127.0.0.1/sharelatex"; got != want {
		t.Errorf("no variables set: got %q, want %q", got, want)
	}

	t.Setenv("MONGO_HOST", "mongo")
	if got, want := MongoURL(), "mongodb://mongo/sharelatex"; got != want {
		t.Errorf("MONGO_HOST only: got %q, want %q", got, want)
	}

	t.Setenv("OVERLEAF_MONGO_URL", "mongodb://mongo-live/sharelatex")
	if got, want := MongoURL(), "mongodb://mongo-live/sharelatex"; got != want {
		t.Errorf("OVERLEAF_MONGO_URL should win over MONGO_HOST: got %q, want %q", got, want)
	}

	t.Setenv("MONGO_CONNECTION_STRING", "mongodb://explicit/db?directConnection=true")
	if got, want := MongoURL(), "mongodb://explicit/db?directConnection=true"; got != want {
		t.Errorf("MONGO_CONNECTION_STRING should win: got %q, want %q", got, want)
	}
}

// server-ce sets OVERLEAF_REDIS_HOST and nothing else, so a service that only
// knows the upstream variable names starts, connects to nothing, and looks
// healthy while every editor session silently stops updating.
func TestRedisAddr(t *testing.T) {
	t.Setenv("PUBSUB_REDIS_HOST", "")
	t.Setenv("REDIS_HOST", "")
	t.Setenv("OVERLEAF_REDIS_HOST", "")
	t.Setenv("PUBSUB_REDIS_PORT", "")
	t.Setenv("REDIS_PORT", "")
	t.Setenv("OVERLEAF_REDIS_PORT", "")

	if got := RedisAddr("PUBSUB"); got != "127.0.0.1:6379" {
		t.Errorf("with nothing set: %q", got)
	}

	t.Setenv("OVERLEAF_REDIS_HOST", "redis-live")
	if got := RedisAddr("PUBSUB"); got != "redis-live:6379" {
		t.Errorf("with only the server-ce variable set: %q", got)
	}

	t.Setenv("REDIS_HOST", "shared")
	if got := RedisAddr("PUBSUB"); got != "shared:6379" {
		t.Errorf("the shared variable should win over the server-ce one: %q", got)
	}

	t.Setenv("PUBSUB_REDIS_HOST", "specific")
	t.Setenv("PUBSUB_REDIS_PORT", "6380")
	if got := RedisAddr("PUBSUB"); got != "specific:6380" {
		t.Errorf("the purpose-specific variable should win: %q", got)
	}

	// An IPv6 literal has to be bracketed or the port cannot be parsed.
	t.Setenv("PUBSUB_REDIS_HOST", "::1")
	if got := RedisAddr("PUBSUB"); got != "[::1]:6380" {
		t.Errorf("an IPv6 host should be bracketed: %q", got)
	}
}

func TestSessionSecrets(t *testing.T) {
	for _, key := range []string{
		"SESSION_SECRET", "SESSION_SECRET_UPCOMING", "SESSION_SECRET_FALLBACK",
		"OVERLEAF_SESSION_SECRET", "CRYPTO_RANDOM",
	} {
		t.Setenv(key, "")
	}
	if got := SessionSecrets(); len(got) != 0 {
		t.Errorf("with nothing set: %v", got)
	}

	// The container generates CRYPTO_RANDOM on first boot and the settings
	// file falls back to it, so it has to be the last resort here too.
	t.Setenv("CRYPTO_RANDOM", "generated")
	if got := SessionSecrets(); len(got) != 1 || got[0] != "generated" {
		t.Errorf("falling back to CRYPTO_RANDOM: %v", got)
	}

	t.Setenv("OVERLEAF_SESSION_SECRET", "configured")
	if got := SessionSecrets(); len(got) != 1 || got[0] != "configured" {
		t.Errorf("the configured secret should win over the generated one: %v", got)
	}

	// A rotation lists every accepted secret, in the order cookie-parser is
	// given them.
	t.Setenv("SESSION_SECRET", "current")
	t.Setenv("SESSION_SECRET_FALLBACK", "previous")
	got := SessionSecrets()
	if len(got) != 2 || got[0] != "current" || got[1] != "previous" {
		t.Errorf("during a rotation: %v", got)
	}
}
