package security

import (
	"fmt"
	"sync"
)

// SecretRef is an opaque reference to a managed secret.
// It carries only the key name — the value is never stored in this struct.
type SecretRef struct {
	Key      string `json:"key"`
	TenantID string `json:"tenant_id"`
}

// String returns a safe representation — never the secret value.
func (r SecretRef) String() string {
	return fmt.Sprintf("SecretRef{key=%q tenant=%q}", r.Key, r.TenantID)
}

// SecretStore is a thread-safe, in-process secret store abstraction.
// It provides key-based lookup so callers never hold raw secret values
// in structs, logs, or error messages.
//
// In production, wire this to a real secrets backend (Vault, AWS Secrets Manager,
// GCP Secret Manager). This in-process implementation is for testing and local dev.
type SecretStore struct {
	mu      sync.RWMutex
	secrets map[string]string // key → encrypted or raw value; never logged
}

// NewSecretStore creates an empty in-process SecretStore.
func NewSecretStore() *SecretStore {
	return &SecretStore{secrets: make(map[string]string)}
}

// Put stores a secret under key for tenantID.
// key must be non-empty. value is never logged at any level.
// Overwrites any existing secret at that key.
func (s *SecretStore) Put(tenantID, key, value string) (SecretRef, error) {
	if key == "" {
		return SecretRef{}, fmt.Errorf("secret key must not be empty")
	}
	if tenantID == "" {
		return SecretRef{}, fmt.Errorf("tenant ID must not be empty for secret storage")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[storeKey(tenantID, key)] = value
	return SecretRef{Key: key, TenantID: tenantID}, nil
}

// Get retrieves the raw secret value for the given ref.
// Returns an error if the secret does not exist.
// Callers must not log the returned value.
func (s *SecretStore) Get(ref SecretRef) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.secrets[storeKey(ref.TenantID, ref.Key)]
	if !ok {
		return "", fmt.Errorf("secret not found: key=%q tenant=%q", ref.Key, ref.TenantID)
	}
	return v, nil
}

// Delete removes a secret. Silently succeeds if the key does not exist.
func (s *SecretStore) Delete(ref SecretRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.secrets, storeKey(ref.TenantID, ref.Key))
}

// Has returns true if a secret exists for the given ref.
func (s *SecretStore) Has(ref SecretRef) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.secrets[storeKey(ref.TenantID, ref.Key)]
	return ok
}

// Keys returns the list of secret keys for a tenant. Values are never returned.
func (s *SecretStore) Keys(tenantID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := tenantID + ":"
	var keys []string
	for k := range s.secrets {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k[len(prefix):])
		}
	}
	return keys
}

func storeKey(tenantID, key string) string {
	return tenantID + ":" + key
}
