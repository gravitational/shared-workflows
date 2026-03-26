// Package kvstore provides functionality for retrieving secrets from AWS Secrets Manager.
// Retrieved values are used to populate environment variables for subsequent steps in the
// GitHub Actions workflow, and secret values are masked to prevent them from being exposed in logs.

package kvstore

type KVStoreStringReader interface {
	GetValue(key string) (string, bool)
	IsEmpty() bool
}

type RepoOrEnvKVStore struct {
	repoStore KVStoreStringReader
	envStore  KVStoreStringReader
}

func (s *RepoOrEnvKVStore) GetValue(key string) (string, bool) {
	// first check environment-specific store, then repo-level store
	envValue, ok := s.envStore.GetValue(key)
	if ok {
		return envValue, true
	}
	return s.repoStore.GetValue(key)
}

func (s *RepoOrEnvKVStore) IsEmpty() bool {
	return s.repoStore.IsEmpty() && s.envStore.IsEmpty()
}

type MapBackedKVStore struct {
	store map[string]string
}

func (m *MapBackedKVStore) GetValue(key string) (string, bool) {
	value, ok := m.store[key]
	if !ok {
		return "", false
	}
	return value, true
}

func (m *MapBackedKVStore) IsEmpty() bool {
	return len(m.store) == 0
}
