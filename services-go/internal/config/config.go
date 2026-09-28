// Package config reads the same environment variables as the Node services'
// config/settings.defaults files, so a Go service can be dropped in behind the
// existing *_HOST env vars with no deployment changes.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Env returns the value of key, or def when unset or empty.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnvInt returns key parsed as an int, or def when unset or unparseable.
func EnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// MongoURL resolves the connection string the way a service does in each of
// the environments it runs in.
//
// The services' own settings.defaults read MONGO_CONNECTION_STRING, falling
// back to MONGO_HOST. A server-ce deployment sets neither: OVERLEAF_CONFIG
// points at /etc/overleaf/settings.js, which takes the URL from
// OVERLEAF_MONGO_URL instead. Missing that variable is not a quiet
// misconfiguration -- the service starts, cannot reach 127.0.0.1:27017, and
// runit restarts it forever.
func MongoURL() string {
	if v := os.Getenv("MONGO_CONNECTION_STRING"); v != "" {
		return v
	}
	if v := os.Getenv("OVERLEAF_MONGO_URL"); v != "" {
		return v
	}
	return "mongodb://" + Env("MONGO_HOST", "127.0.0.1") + "/sharelatex"
}

// MongoDatabase extracts the default database from a connection string, the
// way the Node driver's client.db() with no argument does.
func MongoDatabase(uri string) string {
	rest := uri
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	// Credentials may contain '/', so skip past the userinfo section first.
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}
	i := strings.Index(rest, "/")
	if i < 0 {
		return ""
	}
	rest = rest[i+1:]
	if i := strings.IndexAny(rest, "?"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// ListenAddress mirrors `process.env.LISTEN_ADDRESS || '127.0.0.1'`, the bind
// host used by the Node services.
func ListenAddress() string {
	return Env("LISTEN_ADDRESS", "127.0.0.1")
}

// RedisAddr resolves the host:port of a Redis instance for one purpose, such
// as "PUBSUB" or "REAL_TIME".
//
// The precedence follows the services' own settings.defaults -- a
// purpose-specific variable, then the shared REDIS_HOST -- and then adds the
// server-ce layer, which sets OVERLEAF_REDIS_HOST and nothing else. Missing
// that last fallback is not a quiet misconfiguration: the service starts, all
// pub/sub silently goes nowhere, and every collaborator sees a document that
// never updates.
func RedisAddr(purpose string) string {
	host := Env(purpose+"_REDIS_HOST", Env("REDIS_HOST", Env("OVERLEAF_REDIS_HOST", "127.0.0.1")))
	port := Env(purpose+"_REDIS_PORT", Env("REDIS_PORT", Env("OVERLEAF_REDIS_PORT", "6379")))
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}

// RedisPassword resolves the password for one purpose, with the same
// precedence as RedisAddr.
func RedisPassword(purpose string) string {
	return Env(purpose+"_REDIS_PASSWORD", Env("REDIS_PASSWORD", os.Getenv("OVERLEAF_REDIS_PASS")))
}

// RedisTLS reports whether to connect over TLS, which only the server-ce
// settings expose.
func RedisTLS() bool { return os.Getenv("OVERLEAF_REDIS_TLS") == "true" }

// SessionSecrets lists the cookie signing secrets in the order cookie-parser
// is given them, so a cookie signed with any of them still verifies while a
// rotation is in progress.
//
// server-ce sets neither SESSION_SECRET nor its rotation partners: it puts
// OVERLEAF_SESSION_SECRET in the settings file, falling back to the
// CRYPTO_RANDOM the container generates on first boot. Without that fallback
// no session cookie verifies and nobody can open a project.
func SessionSecrets() []string {
	var secrets []string
	for _, key := range []string{
		"SESSION_SECRET", "SESSION_SECRET_UPCOMING", "SESSION_SECRET_FALLBACK",
	} {
		if v := os.Getenv(key); v != "" {
			secrets = append(secrets, v)
		}
	}
	if len(secrets) > 0 {
		return secrets
	}
	for _, key := range []string{"OVERLEAF_SESSION_SECRET", "CRYPTO_RANDOM"} {
		if v := os.Getenv(key); v != "" {
			return []string{v}
		}
	}
	return nil
}
