package security

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"time"
)

// TenantIdentity is the verified, runtime-safe representation of a tenant.
// It is never constructed from unvalidated input without calling ValidateTenantID.
type TenantIdentity struct {
	TenantID      string `json:"tenant_id"`
	DisplayName   string `json:"display_name"`
	IsolationHash string `json:"isolation_hash"` // content-identity hash for cross-tenant detection
}

var tenantIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,64}$`)

// ValidateTenantID returns an error if the tenant ID does not meet
// AutoStack naming requirements. Tenant IDs are immutable after creation.
func ValidateTenantID(id string) error {
	if id == "" {
		return fmt.Errorf("tenant ID must not be empty")
	}
	if !tenantIDPattern.MatchString(id) {
		return fmt.Errorf("tenant ID %q must match [a-zA-Z0-9_-]{3,64}", id)
	}
	return nil
}

// NewTenantIdentity constructs a verified TenantIdentity.
// Returns an error if the tenant ID fails validation.
func NewTenantIdentity(tenantID, displayName string) (TenantIdentity, error) {
	if err := ValidateTenantID(tenantID); err != nil {
		return TenantIdentity{}, err
	}
	return TenantIdentity{
		TenantID:      tenantID,
		DisplayName:   displayName,
		IsolationHash: computeTenantIsolationHash(tenantID),
	}, nil
}

// VerifyTenantIsolation checks that two tenant identities do not share
// the same isolation hash. Returns (true, nil) when isolation is intact.
// A hash match indicates a possible cross-tenant data access violation.
func VerifyTenantIsolation(a, b TenantIdentity) (bool, string) {
	if a.TenantID == b.TenantID {
		// Same tenant — not a violation.
		return true, ""
	}
	if a.IsolationHash == b.IsolationHash {
		return false, fmt.Sprintf("isolation violation: tenants %q and %q share the same isolation hash",
			a.TenantID, b.TenantID)
	}
	return true, ""
}

// TenantContext carries the verified tenant claim for a request.
// It is constructed only from validated auth tokens or explicit admin grant.
type TenantContext struct {
	Identity    TenantIdentity
	Role        Role
	GrantedAt   time.Time
	RequestID   string
}

// NewTenantContext creates a TenantContext from a verified AuthResult.
// Returns an error if the claims do not include a valid tenant identity.
func NewTenantContext(auth AuthResult, requestID string) (TenantContext, error) {
	if !auth.Valid {
		return TenantContext{}, fmt.Errorf("cannot create tenant context from invalid auth: %s", auth.Reason)
	}
	identity, err := NewTenantIdentity(auth.Claims.TenantID, "")
	if err != nil {
		return TenantContext{}, fmt.Errorf("invalid tenant in token: %w", err)
	}
	return TenantContext{
		Identity:  identity,
		Role:      auth.Claims.Role,
		GrantedAt: time.Now().UTC(),
		RequestID: requestID,
	}, nil
}

// Authorize checks whether the TenantContext has the required permission.
func (tc TenantContext) Authorize(perm Permission) RBACDecision {
	return CheckPermission(tc.Role, perm)
}

func computeTenantIsolationHash(tenantID string) string {
	h := sha256.Sum256([]byte("isolation:" + tenantID))
	return fmt.Sprintf("%x", h[:16]) // first 16 bytes as hex
}
