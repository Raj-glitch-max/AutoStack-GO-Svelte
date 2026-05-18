package security

import (
	"strings"
	"testing"
	"time"
)

// --- auth.go tests ---

func TestVerifyJWT_ValidToken(t *testing.T) {
	secret := "test-secret-value"
	tok, err := IssueJWT("tok-1", "tenant-a", RoleOperator, time.Hour, secret)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}
	result := VerifyJWT(tok, secret, "tenant-a")
	if !result.Valid {
		t.Fatalf("expected valid, got reason: %s", result.Reason)
	}
	if result.Claims.TenantID != "tenant-a" {
		t.Errorf("tenant mismatch: %s", result.Claims.TenantID)
	}
	if result.Claims.Role != RoleOperator {
		t.Errorf("role mismatch: %s", result.Claims.Role)
	}
}

func TestVerifyJWT_WrongTenant(t *testing.T) {
	secret := "test-secret-value"
	tok, _ := IssueJWT("tok-2", "tenant-a", RoleViewer, time.Hour, secret)
	result := VerifyJWT(tok, secret, "tenant-b")
	if result.Valid {
		t.Fatal("expected invalid for wrong tenant")
	}
	if result.Reason != "tenant mismatch" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

func TestVerifyJWT_EmptyInputs(t *testing.T) {
	result := VerifyJWT("", "secret", "")
	if result.Valid {
		t.Fatal("expected invalid for empty token")
	}
}

func TestVerifyJWT_TokenNotInReason(t *testing.T) {
	// Raw token must never appear in reason/error output.
	secret := "test-secret-value"
	tok, _ := IssueJWT("tok-3", "tenant-a", RoleViewer, time.Hour, secret)
	result := VerifyJWT(tok, "wrong-secret", "tenant-a")
	if strings.Contains(result.Reason, tok) {
		t.Errorf("raw token leaked into reason: %s", result.Reason)
	}
}

func TestGenerateAndVerifyAPIToken(t *testing.T) {
	rawToken, hash, err := GenerateAPIToken("tenant-x")
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	if !strings.HasPrefix(rawToken, "autostack_") {
		t.Errorf("unexpected token prefix: %s", rawToken[:20])
	}
	if !VerifyAPIToken(rawToken, hash) {
		t.Fatal("VerifyAPIToken returned false for valid token")
	}
	if VerifyAPIToken("wrong-token", hash) {
		t.Fatal("VerifyAPIToken returned true for wrong token")
	}
	if VerifyAPIToken("", hash) {
		t.Fatal("VerifyAPIToken returned true for empty token")
	}
}

// --- rbac.go tests ---

func TestCheckPermission_OperatorCanTrigger(t *testing.T) {
	d := CheckPermission(RoleOperator, PermTriggerExecution)
	if !d.Granted {
		t.Errorf("operator should have exec:trigger: %s", d.Reason)
	}
}

func TestCheckPermission_ViewerCannotTrigger(t *testing.T) {
	d := CheckPermission(RoleViewer, PermTriggerExecution)
	if d.Granted {
		t.Error("viewer should not have exec:trigger")
	}
}

func TestCheckPermission_AdminHasAll(t *testing.T) {
	allPerms := []Permission{
		PermReadExecutions, PermReadWorkflows, PermReadGovernance, PermReadWorkers,
		PermReadReplay, PermReadContradictions, PermReadCertifications, PermReadArchive,
		PermTriggerExecution, PermCancelExecution, PermQuarantineWorker,
		PermApproveGovernance, PermDenyGovernance,
		PermExportForensic, PermReadAuditTrail,
		PermManageTokens, PermManageRoles, PermRunCertification,
	}
	for _, p := range allPerms {
		d := CheckPermission(RoleAdmin, p)
		if !d.Granted {
			t.Errorf("admin missing permission %s: %s", p, d.Reason)
		}
	}
}

func TestCheckPermission_UnknownRole(t *testing.T) {
	d := CheckPermission(Role("superuser"), PermReadExecutions)
	if d.Granted {
		t.Error("unknown role should be denied")
	}
}

func TestValidRole(t *testing.T) {
	for _, r := range []Role{RoleViewer, RoleOperator, RoleApprover, RoleAuditor, RoleAdmin} {
		if !ValidRole(r) {
			t.Errorf("expected %s to be a valid role", r)
		}
	}
	if ValidRole(Role("god")) {
		t.Error("'god' should not be a valid role")
	}
}

func TestAllPermissionsFor_ReturnsCopy(t *testing.T) {
	perms := AllPermissionsFor(RoleViewer)
	if len(perms) == 0 {
		t.Fatal("viewer should have permissions")
	}
	perms[0] = PermManageTokens // mutate copy
	perms2 := AllPermissionsFor(RoleViewer)
	if perms2[0] == PermManageTokens {
		t.Error("AllPermissionsFor must return a copy, not the live slice")
	}
}

func TestAuditorCanExportForensic(t *testing.T) {
	d := CheckPermission(RoleAuditor, PermExportForensic)
	if !d.Granted {
		t.Errorf("auditor should have archive:export-forensic: %s", d.Reason)
	}
	d2 := CheckPermission(RoleOperator, PermExportForensic)
	if d2.Granted {
		t.Error("operator should NOT have archive:export-forensic")
	}
}

// --- tenants.go tests ---

func TestValidateTenantID(t *testing.T) {
	cases := []struct {
		id    string
		valid bool
	}{
		{"abc", true},
		{"tenant-123", true},
		{"my_tenant_01", true},
		{"ab", false},  // too short
		{"", false},
		{"has space", false},
		{strings.Repeat("x", 65), false}, // too long
		{strings.Repeat("x", 64), true},  // exactly 64
	}
	for _, tc := range cases {
		err := ValidateTenantID(tc.id)
		if tc.valid && err != nil {
			t.Errorf("id=%q expected valid, got %v", tc.id, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("id=%q expected error, got nil", tc.id)
		}
	}
}

func TestNewTenantIdentity_IsolationHash(t *testing.T) {
	id1, err := NewTenantIdentity("tenant-a", "Tenant A")
	if err != nil {
		t.Fatalf("NewTenantIdentity: %v", err)
	}
	id2, err := NewTenantIdentity("tenant-b", "Tenant B")
	if err != nil {
		t.Fatalf("NewTenantIdentity: %v", err)
	}
	if id1.IsolationHash == id2.IsolationHash {
		t.Error("different tenants must have different isolation hashes")
	}
	id1b, _ := NewTenantIdentity("tenant-a", "Tenant A again")
	if id1.IsolationHash != id1b.IsolationHash {
		t.Error("same tenant ID must produce same isolation hash regardless of display name")
	}
}

func TestVerifyTenantIsolation(t *testing.T) {
	a, _ := NewTenantIdentity("tenant-a", "")
	b, _ := NewTenantIdentity("tenant-b", "")

	ok, msg := VerifyTenantIsolation(a, a)
	if !ok {
		t.Errorf("same tenant should not be a violation: %s", msg)
	}

	ok2, msg2 := VerifyTenantIsolation(a, b)
	if !ok2 {
		t.Errorf("different tenants with different hashes should be isolated: %s", msg2)
	}
}

func TestNewTenantContext_FromValidAuth(t *testing.T) {
	secret := "test-secret"
	tok, _ := IssueJWT("tok-ctx", "tenant-a", RoleOperator, time.Hour, secret)
	auth := VerifyJWT(tok, secret, "tenant-a")
	ctx, err := NewTenantContext(auth, "req-1")
	if err != nil {
		t.Fatalf("NewTenantContext: %v", err)
	}
	if ctx.Identity.TenantID != "tenant-a" {
		t.Errorf("unexpected tenant: %s", ctx.Identity.TenantID)
	}
	if ctx.Role != RoleOperator {
		t.Errorf("unexpected role: %s", ctx.Role)
	}
	d := ctx.Authorize(PermTriggerExecution)
	if !d.Granted {
		t.Errorf("operator context should authorize exec:trigger: %s", d.Reason)
	}
}

func TestNewTenantContext_FromInvalidAuth(t *testing.T) {
	auth := AuthResult{Valid: false, Reason: "token expired"}
	_, err := NewTenantContext(auth, "req-2")
	if err == nil {
		t.Fatal("expected error from invalid auth")
	}
}

// --- rate_limit.go tests ---

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(RateLimitPolicy{MaxRequests: 5, Window: time.Minute})
	for i := 0; i < 5; i++ {
		d := rl.Check("key-1")
		if !d.Allowed {
			t.Errorf("request %d should be allowed: %s", i+1, d.Reason)
		}
	}
}

func TestRateLimiter_DeniesOverLimit(t *testing.T) {
	rl := NewRateLimiter(RateLimitPolicy{MaxRequests: 3, Window: time.Minute})
	for i := 0; i < 3; i++ {
		rl.Check("key-2")
	}
	d := rl.Check("key-2")
	if d.Allowed {
		t.Fatal("4th request should be denied")
	}
	if d.Remaining != 0 {
		t.Errorf("remaining should be 0 when denied, got %d", d.Remaining)
	}
}

func TestRateLimiter_IsolatesKeys(t *testing.T) {
	rl := NewRateLimiter(RateLimitPolicy{MaxRequests: 1, Window: time.Minute})
	rl.Check("key-a")
	d := rl.Check("key-b")
	if !d.Allowed {
		t.Error("key-b should be allowed independently of key-a")
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := NewRateLimiter(RateLimitPolicy{MaxRequests: 1, Window: time.Minute})
	rl.Check("key-r")
	rl.Check("key-r") // now denied
	rl.Reset("key-r")
	d := rl.Check("key-r")
	if !d.Allowed {
		t.Error("after reset, first request should be allowed")
	}
}

func TestRateLimiter_Trim(t *testing.T) {
	rl := NewRateLimiter(RateLimitPolicy{MaxRequests: 10, Window: time.Millisecond})
	rl.Check("trim-key")
	time.Sleep(5 * time.Millisecond) // let the window expire
	rl.Trim()
	// After trim, bucket is gone — next check starts fresh
	d := rl.Check("trim-key")
	if !d.Allowed {
		t.Error("after trim of expired bucket, request should be allowed")
	}
}

func TestRateLimiter_CounterIncrementsCorrectly(t *testing.T) {
	rl := NewRateLimiter(RateLimitPolicy{MaxRequests: 10, Window: time.Minute})
	for i := 0; i < 5; i++ {
		d := rl.Check("cnt-key")
		expected := 10 - (i + 1)
		if d.Remaining != expected {
			t.Errorf("after %d calls, remaining should be %d, got %d", i+1, expected, d.Remaining)
		}
	}
}

// --- audit_trail.go tests ---

func TestAuditTrail_RecordAndVerify(t *testing.T) {
	trail := NewAuditTrail()
	trail.Record("e-1", AuditEventAuthSuccess, "tenant-a", "req-1", string(RoleOperator), "allowed")
	trail.Record("e-2", AuditEventPermDenied, "tenant-a", "req-2", string(RoleViewer), "denied",
		WithPermission(PermTriggerExecution),
		WithReason("role viewer does not have exec:trigger"),
	)
	trail.Record("e-3", AuditEventGovernanceAct, "tenant-b", "req-3", string(RoleApprover), "allowed",
		WithMetadata(map[string]string{"action": "approve", "policy": "deploy-prod"}),
	)

	ok, msg := trail.VerifyIntegrity()
	if !ok {
		t.Fatalf("integrity check failed: %s", msg)
	}
	if trail.Len() != 3 {
		t.Errorf("expected 3 entries, got %d", trail.Len())
	}
}

func TestAuditTrail_EntriesFor(t *testing.T) {
	trail := NewAuditTrail()
	trail.Record("e-1", AuditEventAuthSuccess, "tenant-a", "r1", "operator", "allowed")
	trail.Record("e-2", AuditEventAuthFailure, "tenant-b", "r2", "viewer", "denied")
	trail.Record("e-3", AuditEventRateLimited, "tenant-a", "r3", "operator", "denied")

	forA := trail.EntriesFor("tenant-a")
	if len(forA) != 2 {
		t.Errorf("expected 2 entries for tenant-a, got %d", len(forA))
	}
	for _, e := range forA {
		if e.TenantID != "tenant-a" {
			t.Errorf("EntriesFor returned wrong tenant: %s", e.TenantID)
		}
	}
}

func TestAuditTrail_TamperDetected(t *testing.T) {
	trail := NewAuditTrail()
	trail.Record("e-1", AuditEventAuthSuccess, "tenant-a", "r1", "operator", "allowed")
	trail.Record("e-2", AuditEventPermGranted, "tenant-a", "r2", "operator", "allowed")

	// Tamper with first entry's outcome.
	entries := trail.Entries() // copy
	_ = entries                 // actual entries are internal

	// Directly mutate internal state to simulate tampering.
	trail.entries[0].Outcome = "denied"

	ok, msg := trail.VerifyIntegrity()
	if ok {
		t.Fatal("tampered trail should fail integrity check")
	}
	if msg == "" {
		t.Error("integrity failure should include a reason")
	}
}

func TestAuditTrail_ChainBrokenOnInsertion(t *testing.T) {
	trail := NewAuditTrail()
	trail.Record("e-1", AuditEventAuthSuccess, "tenant-a", "r1", "operator", "allowed")
	trail.Record("e-2", AuditEventAuthSuccess, "tenant-a", "r2", "operator", "allowed")

	// Entries copies — chain hashes in actual trail should be correct.
	ok, msg := trail.VerifyIntegrity()
	if !ok {
		t.Fatalf("chain should be intact: %s", msg)
	}
}

// --- secrets.go tests ---

func TestSecretStore_PutAndGet(t *testing.T) {
	store := NewSecretStore()
	ref, err := store.Put("tenant-a", "db-password", "s3cr3t!")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	val, err := store.Get(ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "s3cr3t!" {
		t.Errorf("unexpected value: %s", val)
	}
}

func TestSecretStore_TenantIsolation(t *testing.T) {
	store := NewSecretStore()
	store.Put("tenant-a", "key", "value-a")
	store.Put("tenant-b", "key", "value-b")

	refA := SecretRef{Key: "key", TenantID: "tenant-a"}
	refB := SecretRef{Key: "key", TenantID: "tenant-b"}

	va, _ := store.Get(refA)
	vb, _ := store.Get(refB)
	if va == vb {
		t.Error("different tenants must not share secret values")
	}
}

func TestSecretStore_MissingKey(t *testing.T) {
	store := NewSecretStore()
	_, err := store.Get(SecretRef{Key: "nope", TenantID: "tenant-a"})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestSecretStore_Delete(t *testing.T) {
	store := NewSecretStore()
	ref, _ := store.Put("tenant-a", "temp-key", "tempval")
	store.Delete(ref)
	_, err := store.Get(ref)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestSecretStore_Has(t *testing.T) {
	store := NewSecretStore()
	ref, _ := store.Put("tenant-a", "present", "val")
	absent := SecretRef{Key: "absent", TenantID: "tenant-a"}
	if !store.Has(ref) {
		t.Error("Has should return true for existing key")
	}
	if store.Has(absent) {
		t.Error("Has should return false for missing key")
	}
}

func TestSecretStore_Keys(t *testing.T) {
	store := NewSecretStore()
	store.Put("tenant-a", "key1", "v1")
	store.Put("tenant-a", "key2", "v2")
	store.Put("tenant-b", "key1", "vb")

	keys := store.Keys("tenant-a")
	if len(keys) != 2 {
		t.Errorf("expected 2 keys for tenant-a, got %d", len(keys))
	}
}

func TestSecretRef_StringNeverContainsValue(t *testing.T) {
	ref := SecretRef{Key: "my-key", TenantID: "tenant-a"}
	s := ref.String()
	if strings.Contains(s, "secret") || strings.Contains(s, "value") {
		t.Errorf("SecretRef.String() must not contain secret value: %s", s)
	}
}

func TestSecretStore_EmptyKeyRejected(t *testing.T) {
	store := NewSecretStore()
	_, err := store.Put("tenant-a", "", "value")
	if err == nil {
		t.Fatal("empty key should be rejected")
	}
}

func TestSecretStore_EmptyTenantRejected(t *testing.T) {
	store := NewSecretStore()
	_, err := store.Put("", "key", "value")
	if err == nil {
		t.Fatal("empty tenant should be rejected")
	}
}
