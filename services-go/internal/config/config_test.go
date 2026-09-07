package config

import "testing"

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
