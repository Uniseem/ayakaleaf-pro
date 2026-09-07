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
