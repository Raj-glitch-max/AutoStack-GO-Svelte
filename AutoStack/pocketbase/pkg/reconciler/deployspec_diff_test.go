package reconciler

import (
	"testing"

	"github.com/janlauber/one-click/pkg/providers"
)

// Phase 3.10 — DeploySpec semantic diff tests.

func baseSpec() providers.DeploySpec {
	return providers.DeploySpec{
		Image: providers.ImageSpec{
			Registry:   "docker.io",
			Repository: "myapp/api",
			Tag:        "v1.0.0",
			PullPolicy: "IfNotPresent",
		},
		Compute: providers.ComputeSpec{
			CPURequestVCPU:  0.25,
			CPULimitVCPU:    1.0,
			MemoryRequestMB: 256,
			MemoryLimitMB:   512,
		},
		Scale: providers.ScaleSpec{
			MinReplicas:        1,
			MaxReplicas:        3,
			ScaleMetric:        "cpu",
			ScaleTargetPercent: 70,
		},
		Env: []providers.EnvVar{
			{Name: "APP_ENV", Value: "production"},
			{Name: "LOG_LEVEL", Value: "info"},
		},
		Secrets: []providers.SecretRef{
			{Name: "db-creds", Source: "aws-secrets-manager", SecretName: "prod/db", SecretKey: "password"},
		},
		Network: providers.NetworkSpec{
			Interfaces: []providers.NetworkInterface{
				{ContainerPort: 8080, Protocol: "TCP", ServiceType: "LoadBalancer", TLSEnabled: true},
			},
		},
		Health: &providers.HealthSpec{
			LivenessPath: "/health", LivenessPort: 8080,
			ReadinessPath: "/ready", ReadinessPort: 8080,
			InitialDelaySeconds: 10, PeriodSeconds: 30, FailureThreshold: 3,
		},
	}
}

func TestDiffDeploySpecIdentical(t *testing.T) {
	s := baseSpec()
	diff := DiffDeploySpec(s, s)
	if !diff.IsEmpty() {
		t.Errorf("identical specs must produce empty diff, got %d changes: %+v", len(diff.ChangedFields), diff.ChangedFields)
	}
	if diff.OverallSeverity != "" {
		t.Errorf("empty diff must have no OverallSeverity, got %q", diff.OverallSeverity)
	}
}

func TestDiffDeploySpecImageTagChange(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	new.Image.Tag = "v2.0.0"
	diff := DiffDeploySpec(old, new)
	if diff.IsEmpty() {
		t.Fatal("tag change must produce non-empty diff")
	}
	if !diff.ImageChanged {
		t.Error("ImageChanged must be true for tag change")
	}
	if diff.OverallSeverity != SpecDriftSeverityHigh {
		t.Errorf("image tag change must be High severity, got %q", diff.OverallSeverity)
	}
	found := false
	for _, f := range diff.ChangedFields {
		if f.Field == "image.tag" {
			found = true
			if f.OldValue != "v1.0.0" || f.NewValue != "v2.0.0" {
				t.Errorf("tag field old=%q new=%q, want v1.0.0/v2.0.0", f.OldValue, f.NewValue)
			}
		}
	}
	if !found {
		t.Error("image.tag field not found in ChangedFields")
	}
}

func TestDiffDeploySpecSecretRefAdded(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	new.Secrets = append(new.Secrets, providers.SecretRef{
		Name: "api-key", Source: "aws-secrets-manager", SecretName: "prod/api", SecretKey: "key",
	})
	diff := DiffDeploySpec(old, new)
	if !diff.SecretsChanged {
		t.Error("SecretsChanged must be true when a secret ref is added")
	}
	if !diff.SecuritySensitiveChange {
		t.Error("SecuritySensitiveChange must be true when secret ref changes")
	}
	if diff.OverallSeverity != SpecDriftSeverityHigh {
		t.Errorf("secret ref addition must be High severity, got %q", diff.OverallSeverity)
	}
}

func TestDiffDeploySpecEnvVarChanged(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	new.Env[1].Value = "debug"
	diff := DiffDeploySpec(old, new)
	if !diff.EnvChanged {
		t.Error("EnvChanged must be true")
	}
	if diff.OverallSeverity != SpecDriftSeverityMedium {
		t.Errorf("env var change must be Medium severity, got %q", diff.OverallSeverity)
	}
}

func TestDiffDeploySpecSensitiveEnvVarRedacted(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	old.Env = append(old.Env, providers.EnvVar{Name: "DB_PASSWORD", Value: "secret123"})
	new.Env = append(new.Env, providers.EnvVar{Name: "DB_PASSWORD", Value: "newsecret456"})
	diff := DiffDeploySpec(old, new)
	for _, f := range diff.ChangedFields {
		if f.Field == "env.DB_PASSWORD" {
			if f.OldValue != "<redacted>" || f.NewValue != "<redacted>" {
				t.Errorf("sensitive env var must be redacted, got old=%q new=%q", f.OldValue, f.NewValue)
			}
		}
	}
}

func TestDiffDeploySpecNetworkChange(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	new.Network.Interfaces[0].TLSEnabled = false
	diff := DiffDeploySpec(old, new)
	if !diff.NetworkChanged {
		t.Error("NetworkChanged must be true")
	}
	if !diff.SecuritySensitiveChange {
		t.Error("SecuritySensitiveChange must be true for network change")
	}
}

func TestDiffDeploySpecComputeChange(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	new.Compute.MemoryLimitMB = 1024
	diff := DiffDeploySpec(old, new)
	if !diff.ComputeChanged {
		t.Error("ComputeChanged must be true")
	}
	if diff.OverallSeverity != SpecDriftSeverityMedium {
		t.Errorf("compute change must be Medium severity, got %q", diff.OverallSeverity)
	}
}

func TestDiffDeploySpecChangedFieldsSorted(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	// Change multiple fields to exercise sorting.
	new.Image.Tag = "v2.0.0"
	new.Compute.MemoryLimitMB = 1024
	new.Scale.MinReplicas = 2
	diff := DiffDeploySpec(old, new)
	for i := 1; i < len(diff.ChangedFields); i++ {
		if diff.ChangedFields[i].Field < diff.ChangedFields[i-1].Field {
			t.Errorf("ChangedFields not sorted: [%d]=%q < [%d]=%q",
				i, diff.ChangedFields[i].Field, i-1, diff.ChangedFields[i-1].Field)
		}
	}
}

func TestDiffDeploySpecLimitationsNotePresent(t *testing.T) {
	s := baseSpec()
	diff := DiffDeploySpec(s, s)
	if diff.LimitationsNote == "" {
		t.Error("LimitationsNote must always be set to document what the diff engine cannot detect")
	}
}

func TestDiffDeploySpecHealthNilVsNonNil(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	new.Health = nil
	diff := DiffDeploySpec(old, new)
	if !diff.HealthChanged {
		t.Error("HealthChanged must be true when health spec is removed")
	}
}

func TestDiffDeploySpecScaleChange(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	new.Scale.MaxReplicas = 10
	diff := DiffDeploySpec(old, new)
	if !diff.ScaleChanged {
		t.Error("ScaleChanged must be true")
	}
}

func TestDiffDeploySpecEnvVarRemoved(t *testing.T) {
	old := baseSpec()
	new := baseSpec()
	new.Env = new.Env[:1] // remove LOG_LEVEL
	diff := DiffDeploySpec(old, new)
	if !diff.EnvChanged {
		t.Error("EnvChanged must be true when env var removed")
	}
	found := false
	for _, f := range diff.ChangedFields {
		if f.Field == "env.LOG_LEVEL" && f.NewValue == "<removed>" {
			found = true
		}
	}
	if !found {
		t.Error("removed env var must appear as <removed> in ChangedFields")
	}
}
