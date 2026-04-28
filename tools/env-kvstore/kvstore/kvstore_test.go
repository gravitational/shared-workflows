package kvstore

import (
	"testing"
)

func TestRepoOrEnvStore(t *testing.T) {
	kvstore := &RepoOrEnvKVStore{
		repoStore: &MapBackedKVStore{
			store: map[string]string{
				"repoOnly": "repoValue",
				"overlap":  "repoOverlapValue",
			},
		},
		envStore: &MapBackedKVStore{
			store: map[string]string{
				"envOnly": "envValue",
				"overlap": "envOverlapValue",
			},
		},
	}

	if value, ok := kvstore.GetValue("envOnly"); !ok || value != "envValue" {
		t.Errorf("expected to get 'envValue' for 'envOnly', got '%s'", value)
	}

	if value, ok := kvstore.GetValue("repoOnly"); !ok || value != "repoValue" {
		t.Errorf("expected to get 'repoValue' for 'repoOnly', got '%s'", value)
	}

	if value, ok := kvstore.GetValue("overlap"); !ok || value != "envOverlapValue" {
		t.Errorf("expected to get 'envOverlapValue' for 'overlap', got '%s'", value)
	}

	if _, ok := kvstore.GetValue("nonExistent"); ok {
		t.Errorf("expected to not find 'nonExistent' key, but it was found")
	}
}
