package reconciler

import (
	"fmt"
	"strings"

	"github.com/janlauber/one-click/pkg/providers"
)

// Phase 3.10 — DeploySpec semantic drift detection.
//
// DiffDeploySpec compares two DeploySpec snapshots and returns a structured
// description of what changed, classified by semantic severity.
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. HONEST SCOPE: DeploySpec does not carry an image digest field. Tag
//      equality is the best the current model can assert. If the same tag
//      is reused with a different image layer, this diff will NOT detect it.
//      That limitation is documented explicitly. Do not claim digest-level
//      drift detection.
//
//   2. NO SIDE EFFECTS: DiffDeploySpec is a pure function. It does not write
//      to the DB, open incidents, or record decisions. Callers are responsible
//      for persisting findings via RecordDecision + CreateOrEscalateIncident.
//
//   3. DETERMINISTIC: given the same old/new pair, the output is identical
//      across calls. ChangedFields is sorted alphabetically.
//
//   4. SECURITY CLASSIFICATION: changes to secrets refs, network config, and
//      image source are always High severity. An env var named "SECRET_*"
//      is treated as Medium (we cannot know if it's truly a secret).
//
// ─────────────────────────────────────────────────────────────────────────

// SpecDriftSeverity classifies the operational impact of a spec change.
type SpecDriftSeverity string

const (
	SpecDriftSeverityLow    SpecDriftSeverity = "low"
	SpecDriftSeverityMedium SpecDriftSeverity = "medium"
	SpecDriftSeverityHigh   SpecDriftSeverity = "high"
)

// SpecChangedField describes one semantic change between old and new spec.
type SpecChangedField struct {
	Field     string            `json:"field"`
	OldValue  string            `json:"old_value"`
	NewValue  string            `json:"new_value"`
	Severity  SpecDriftSeverity `json:"severity"`
	Note      string            `json:"note,omitempty"`
}

// DeploySpecDiff is the result of DiffDeploySpec.
// An empty ChangedFields slice means the specs are semantically identical.
type DeploySpecDiff struct {
	// ChangedFields lists each detected change, sorted alphabetically by Field.
	ChangedFields []SpecChangedField `json:"changed_fields"`

	// OverallSeverity is the maximum severity across all changed fields.
	// Empty when ChangedFields is empty.
	OverallSeverity SpecDriftSeverity `json:"overall_severity,omitempty"`

	// ImageChanged is true when any image coordinate (registry, repo, tag,
	// pull policy, or credential) changed.
	ImageChanged bool `json:"image_changed"`

	// SecretsChanged is true when any SecretRef was added, removed, or modified.
	SecretsChanged bool `json:"secrets_changed"`

	// SecuritySensitiveChange is true when the change touches secrets refs,
	// network interfaces, or the image source. Callers should escalate to High
	// severity when this is true, regardless of OverallSeverity.
	SecuritySensitiveChange bool `json:"security_sensitive_change"`

	// ComputeChanged is true when CPU or memory limits changed.
	ComputeChanged bool `json:"compute_changed"`

	// ScaleChanged is true when replica counts or scaling policy changed.
	ScaleChanged bool `json:"scale_changed"`

	// EnvChanged is true when any env var was added, removed, or modified.
	EnvChanged bool `json:"env_changed"`

	// NetworkChanged is true when any network interface was added, removed,
	// or modified (ports, protocol, service type, ingress, TLS).
	NetworkChanged bool `json:"network_changed"`

	// HealthChanged is true when health check config changed.
	HealthChanged bool `json:"health_changed"`

	// ProviderConfigChanged is true when TargetConfig changed.
	ProviderConfigChanged bool `json:"provider_config_changed"`

	// LimitationsNote documents what this diff engine cannot detect.
	LimitationsNote string `json:"limitations_note,omitempty"`
}

// IsEmpty returns true when no changes were detected.
func (d *DeploySpecDiff) IsEmpty() bool {
	return len(d.ChangedFields) == 0
}

// DiffDeploySpec compares old and new DeploySpec snapshots. Pure function.
// The returned diff is deterministic: identical inputs always produce identical output.
func DiffDeploySpec(old, new providers.DeploySpec) DeploySpecDiff {
	var diff DeploySpecDiff
	diff.LimitationsNote = "image digest comparison not supported — tag equality is the only image identity check available in the current model"

	var fields []SpecChangedField

	// ── Image ──────────────────────────────────────────────────────────────
	if old.Image.Registry != new.Image.Registry {
		fields = append(fields, SpecChangedField{
			Field: "image.registry", OldValue: old.Image.Registry, NewValue: new.Image.Registry,
			Severity: SpecDriftSeverityHigh,
			Note:     "image source registry changed — deployment will pull from a different origin",
		})
		diff.ImageChanged = true
		diff.SecuritySensitiveChange = true
	}
	if old.Image.Repository != new.Image.Repository {
		fields = append(fields, SpecChangedField{
			Field: "image.repository", OldValue: old.Image.Repository, NewValue: new.Image.Repository,
			Severity: SpecDriftSeverityHigh,
			Note:     "image repository changed — different container binary",
		})
		diff.ImageChanged = true
		diff.SecuritySensitiveChange = true
	}
	if old.Image.Tag != new.Image.Tag {
		fields = append(fields, SpecChangedField{
			Field: "image.tag", OldValue: old.Image.Tag, NewValue: new.Image.Tag,
			Severity: SpecDriftSeverityHigh,
			Note:     "image tag changed; note: same tag with different digest is NOT detected by this engine",
		})
		diff.ImageChanged = true
	}
	if old.Image.PullPolicy != new.Image.PullPolicy {
		fields = append(fields, SpecChangedField{
			Field: "image.pull_policy", OldValue: old.Image.PullPolicy, NewValue: new.Image.PullPolicy,
			Severity: SpecDriftSeverityMedium,
		})
		diff.ImageChanged = true
	}
	if old.Image.RegistryCredential != new.Image.RegistryCredential {
		fields = append(fields, SpecChangedField{
			Field: "image.registry_credential", OldValue: old.Image.RegistryCredential, NewValue: new.Image.RegistryCredential,
			Severity: SpecDriftSeverityHigh,
			Note:     "registry credential reference changed",
		})
		diff.ImageChanged = true
		diff.SecuritySensitiveChange = true
	}

	// ── Compute ────────────────────────────────────────────────────────────
	if old.Compute.CPURequestVCPU != new.Compute.CPURequestVCPU {
		fields = append(fields, SpecChangedField{
			Field: "compute.cpu_request_vcpu",
			OldValue: fmt.Sprintf("%.3f", old.Compute.CPURequestVCPU),
			NewValue: fmt.Sprintf("%.3f", new.Compute.CPURequestVCPU),
			Severity: SpecDriftSeverityMedium,
		})
		diff.ComputeChanged = true
	}
	if old.Compute.CPULimitVCPU != new.Compute.CPULimitVCPU {
		fields = append(fields, SpecChangedField{
			Field: "compute.cpu_limit_vcpu",
			OldValue: fmt.Sprintf("%.3f", old.Compute.CPULimitVCPU),
			NewValue: fmt.Sprintf("%.3f", new.Compute.CPULimitVCPU),
			Severity: SpecDriftSeverityMedium,
		})
		diff.ComputeChanged = true
	}
	if old.Compute.MemoryRequestMB != new.Compute.MemoryRequestMB {
		fields = append(fields, SpecChangedField{
			Field: "compute.memory_request_mb",
			OldValue: fmt.Sprintf("%d", old.Compute.MemoryRequestMB),
			NewValue: fmt.Sprintf("%d", new.Compute.MemoryRequestMB),
			Severity: SpecDriftSeverityMedium,
		})
		diff.ComputeChanged = true
	}
	if old.Compute.MemoryLimitMB != new.Compute.MemoryLimitMB {
		fields = append(fields, SpecChangedField{
			Field: "compute.memory_limit_mb",
			OldValue: fmt.Sprintf("%d", old.Compute.MemoryLimitMB),
			NewValue: fmt.Sprintf("%d", new.Compute.MemoryLimitMB),
			Severity: SpecDriftSeverityMedium,
		})
		diff.ComputeChanged = true
	}

	// ── Scale ──────────────────────────────────────────────────────────────
	if old.Scale.MinReplicas != new.Scale.MinReplicas {
		fields = append(fields, SpecChangedField{
			Field: "scale.min_replicas",
			OldValue: fmt.Sprintf("%d", old.Scale.MinReplicas),
			NewValue: fmt.Sprintf("%d", new.Scale.MinReplicas),
			Severity: SpecDriftSeverityMedium,
		})
		diff.ScaleChanged = true
	}
	if old.Scale.MaxReplicas != new.Scale.MaxReplicas {
		fields = append(fields, SpecChangedField{
			Field: "scale.max_replicas",
			OldValue: fmt.Sprintf("%d", old.Scale.MaxReplicas),
			NewValue: fmt.Sprintf("%d", new.Scale.MaxReplicas),
			Severity: SpecDriftSeverityMedium,
		})
		diff.ScaleChanged = true
	}
	if old.Scale.ScaleMetric != new.Scale.ScaleMetric {
		fields = append(fields, SpecChangedField{
			Field: "scale.scale_metric", OldValue: old.Scale.ScaleMetric, NewValue: new.Scale.ScaleMetric,
			Severity: SpecDriftSeverityMedium,
		})
		diff.ScaleChanged = true
	}
	if old.Scale.ScaleTargetPercent != new.Scale.ScaleTargetPercent {
		fields = append(fields, SpecChangedField{
			Field: "scale.scale_target_percent",
			OldValue: fmt.Sprintf("%d", old.Scale.ScaleTargetPercent),
			NewValue: fmt.Sprintf("%d", new.Scale.ScaleTargetPercent),
			Severity: SpecDriftSeverityMedium,
		})
		diff.ScaleChanged = true
	}

	// ── Env vars ───────────────────────────────────────────────────────────
	oldEnv := indexEnvVars(old.Env)
	newEnv := indexEnvVars(new.Env)
	for name, oldVal := range oldEnv {
		if newVal, exists := newEnv[name]; !exists {
			fields = append(fields, SpecChangedField{
				Field: "env." + name, OldValue: redactIfSensitive(name, oldVal), NewValue: "<removed>",
				Severity: SpecDriftSeverityMedium,
			})
			diff.EnvChanged = true
		} else if oldVal != newVal {
			fields = append(fields, SpecChangedField{
				Field: "env." + name,
				OldValue: redactIfSensitive(name, oldVal),
				NewValue: redactIfSensitive(name, newVal),
				Severity: SpecDriftSeverityMedium,
			})
			diff.EnvChanged = true
		}
	}
	for name, newVal := range newEnv {
		if _, exists := oldEnv[name]; !exists {
			fields = append(fields, SpecChangedField{
				Field: "env." + name, OldValue: "<not set>", NewValue: redactIfSensitive(name, newVal),
				Severity: SpecDriftSeverityMedium,
			})
			diff.EnvChanged = true
		}
	}

	// ── Secrets refs ───────────────────────────────────────────────────────
	oldSecrets := indexSecretRefs(old.Secrets)
	newSecrets := indexSecretRefs(new.Secrets)
	for name, oldRef := range oldSecrets {
		if newRef, exists := newSecrets[name]; !exists {
			fields = append(fields, SpecChangedField{
				Field: "secrets." + name, OldValue: secretRefSummary(oldRef), NewValue: "<removed>",
				Severity: SpecDriftSeverityHigh,
				Note:     "secret reference removed",
			})
			diff.SecretsChanged = true
			diff.SecuritySensitiveChange = true
		} else if oldRef != newRef {
			fields = append(fields, SpecChangedField{
				Field: "secrets." + name,
				OldValue: secretRefSummary(oldRef),
				NewValue: secretRefSummary(newRef),
				Severity: SpecDriftSeverityHigh,
				Note:     "secret reference changed",
			})
			diff.SecretsChanged = true
			diff.SecuritySensitiveChange = true
		}
	}
	for name, newRef := range newSecrets {
		if _, exists := oldSecrets[name]; !exists {
			fields = append(fields, SpecChangedField{
				Field: "secrets." + name, OldValue: "<not set>", NewValue: secretRefSummary(newRef),
				Severity: SpecDriftSeverityHigh,
				Note:     "secret reference added",
			})
			diff.SecretsChanged = true
			diff.SecuritySensitiveChange = true
		}
	}

	// ── Network interfaces ─────────────────────────────────────────────────
	oldNet := indexNetworkInterfaces(old.Network.Interfaces)
	newNet := indexNetworkInterfaces(new.Network.Interfaces)
	for key, oldIface := range oldNet {
		if newIface, exists := newNet[key]; !exists {
			fields = append(fields, SpecChangedField{
				Field: "network." + key, OldValue: netIfaceSummary(oldIface), NewValue: "<removed>",
				Severity: SpecDriftSeverityHigh,
				Note:     "network interface removed",
			})
			diff.NetworkChanged = true
			diff.SecuritySensitiveChange = true
		} else if netIfaceSummary(oldIface) != netIfaceSummary(newIface) {
			fields = append(fields, SpecChangedField{
				Field: "network." + key,
				OldValue: netIfaceSummary(oldIface),
				NewValue: netIfaceSummary(newIface),
				Severity: SpecDriftSeverityHigh,
			})
			diff.NetworkChanged = true
			diff.SecuritySensitiveChange = true
		}
	}
	for key, newIface := range newNet {
		if _, exists := oldNet[key]; !exists {
			fields = append(fields, SpecChangedField{
				Field: "network." + key, OldValue: "<not configured>", NewValue: netIfaceSummary(newIface),
				Severity: SpecDriftSeverityHigh,
				Note:     "network interface added",
			})
			diff.NetworkChanged = true
			diff.SecuritySensitiveChange = true
		}
	}

	// ── Health checks ──────────────────────────────────────────────────────
	oldH := healthSummary(old.Health)
	newH := healthSummary(new.Health)
	if oldH != newH {
		fields = append(fields, SpecChangedField{
			Field: "health", OldValue: oldH, NewValue: newH,
			Severity: SpecDriftSeverityMedium,
		})
		diff.HealthChanged = true
	}

	// ── Provider config ────────────────────────────────────────────────────
	oldPC := providerConfigSummary(old.TargetConfig)
	newPC := providerConfigSummary(new.TargetConfig)
	if oldPC != newPC {
		fields = append(fields, SpecChangedField{
			Field: "provider_config", OldValue: oldPC, NewValue: newPC,
			Severity: SpecDriftSeverityMedium,
			Note:     "provider-specific config changed; review for IAM or security impact",
		})
		diff.ProviderConfigChanged = true
	}

	// Sort fields deterministically.
	sortSpecFields(fields)
	diff.ChangedFields = fields

	// Compute overall severity.
	diff.OverallSeverity = overallSeverity(fields)

	return diff
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func indexEnvVars(vars []providers.EnvVar) map[string]string {
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		m[v.Name] = v.Value
	}
	return m
}

func indexSecretRefs(refs []providers.SecretRef) map[string]providers.SecretRef {
	m := make(map[string]providers.SecretRef, len(refs))
	for _, r := range refs {
		m[r.Name] = r
	}
	return m
}

func indexNetworkInterfaces(ifaces []providers.NetworkInterface) map[string]providers.NetworkInterface {
	m := make(map[string]providers.NetworkInterface, len(ifaces))
	for _, iface := range ifaces {
		key := fmt.Sprintf("%d/%s", iface.ContainerPort, iface.Protocol)
		m[key] = iface
	}
	return m
}

func secretRefSummary(r providers.SecretRef) string {
	return fmt.Sprintf("source=%s secret=%s key=%s", r.Source, r.SecretName, r.SecretKey)
}

func netIfaceSummary(iface providers.NetworkInterface) string {
	tls := ""
	if iface.TLSEnabled {
		tls = "+tls"
	}
	return fmt.Sprintf("port=%d proto=%s type=%s host=%s%s",
		iface.ContainerPort, iface.Protocol, iface.ServiceType, iface.IngressHost, tls)
}

func healthSummary(h *providers.HealthSpec) string {
	if h == nil {
		return "<none>"
	}
	return fmt.Sprintf("liveness=%s:%d readiness=%s:%d delay=%ds period=%ds threshold=%d",
		h.LivenessPath, h.LivenessPort, h.ReadinessPath, h.ReadinessPort,
		h.InitialDelaySeconds, h.PeriodSeconds, h.FailureThreshold)
}

func providerConfigSummary(cfg map[string]interface{}) string {
	if len(cfg) == 0 {
		return "<empty>"
	}
	// Stable iteration for determinism.
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sortStrings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, cfg[k]))
	}
	return strings.Join(parts, " ")
}

// redactIfSensitive replaces the value with "<redacted>" when the env var
// name suggests it may contain a credential. We never log credentials.
func redactIfSensitive(name, value string) string {
	upper := strings.ToUpper(name)
	for _, kw := range []string{"SECRET", "PASSWORD", "TOKEN", "KEY", "CREDENTIAL", "PASS", "AUTH"} {
		if strings.Contains(upper, kw) {
			return "<redacted>"
		}
	}
	return value
}

func sortSpecFields(fields []SpecChangedField) {
	// Insertion sort — slice is small (bounded by spec field count).
	for i := 1; i < len(fields); i++ {
		for j := i; j > 0 && fields[j].Field < fields[j-1].Field; j-- {
			fields[j], fields[j-1] = fields[j-1], fields[j]
		}
	}
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

func overallSeverity(fields []SpecChangedField) SpecDriftSeverity {
	sev := SpecDriftSeverity("")
	for _, f := range fields {
		if f.Severity == SpecDriftSeverityHigh {
			return SpecDriftSeverityHigh
		}
		if f.Severity == SpecDriftSeverityMedium {
			sev = SpecDriftSeverityMedium
		} else if sev == "" {
			sev = SpecDriftSeverityLow
		}
	}
	return sev
}
