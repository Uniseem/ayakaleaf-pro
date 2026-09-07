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

// MongoURL mirrors:
//
//	process.env.MONGO_CONNECTION_STRING ||
//	  `mongodb://${process.env.MONGO_HOST || '127.0.0.1'}/sharelatex`
func MongoURL() string {
	if v := os.Getenv("MONGO_CONNECTION_STRING"); v != "" {
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
