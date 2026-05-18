package verifier

import (
	"time"
)

const (
	// Stabilization window durations (in nanoseconds stored as int64).
	windowCloudRun = int64(2 * time.Minute)
	windowECS      = int64(5 * time.Minute)
	windowAzureACA = int64(3 * time.Minute)
	windowDefault  = int64(2 * time.Minute)

	// Consistency period durations.
	periodCloudRun = int64(30 * time.Second)
	periodECS      = int64(60 * time.Second)
	periodAzureACA = int64(45 * time.Second)
	periodDefault  = int64(30 * time.Second)
)

// Provider-specific stabilization windows.
// The window must be satisfied BEFORE convergence can be declared.
// These values are conservative estimates based on provider documentation.
var (
	// CloudRunStabilizationWindow: revision propagation requires 2 min + 3 samples.
	CloudRunStabilizationWindow = StabilizationWindow{
		Provider:          "gcp-cloudrun",
		WindowDuration:    windowCloudRun,
		RequiredSamples:   3,
		ConsistencyPeriod: periodCloudRun,
		Description:       "Cloud Run revision propagation requires a 2-minute window with 3 consistent observations.",
		Limitations: []string{
			"Traffic split operations may take longer than the base window.",
			"Multi-region deployments require per-region verification.",
		},
	}

	// ECSStabilizationWindow: Fargate task stabilization requires 5 min + 3 samples.
	ECSStabilizationWindow = StabilizationWindow{
		Provider:          "aws-ecs",
		WindowDuration:    windowECS,
		RequiredSamples:   3,
		ConsistencyPeriod: periodECS,
		Description:       "ECS Fargate task RUNNING state is not immediate; 5-minute stabilization window required.",
		Limitations: []string{
			"Task replacement during rolling update extends the window.",
			"VPC networking propagation can add 60–90s to stabilization.",
		},
	}

	// AzureACAStabilizationWindow: Container Apps require 3 min + 3 samples.
	AzureACAStabilizationWindow = StabilizationWindow{
		Provider:          "azure-aca",
		WindowDuration:    windowAzureACA,
		RequiredSamples:   3,
		ConsistencyPeriod: periodAzureACA,
		Description:       "Azure Container Apps deployment propagation requires a 3-minute window.",
		Limitations: []string{
			"Azure ARM eventual consistency may delay status visibility by 45–90s.",
		},
	}

	// DefaultStabilizationWindow: used when no provider-specific window is available.
	DefaultStabilizationWindow = StabilizationWindow{
		Provider:          "unknown",
		WindowDuration:    windowDefault,
		RequiredSamples:   3,
		ConsistencyPeriod: periodDefault,
		Description:       "Default stabilization window: 2 minutes with 3 samples. Provider-specific windows are preferred.",
		Limitations: []string{
			"Default window does not account for provider-specific propagation delays.",
			"Use a provider-specific stabilization window where available.",
		},
	}
)

// StabilizationWindowForProvider returns the canonical stabilization window for a provider.
// Returns DefaultStabilizationWindow for unknown providers.
func StabilizationWindowForProvider(provider string) StabilizationWindow {
	switch provider {
	case "gcp-cloudrun", "cloudrun":
		return CloudRunStabilizationWindow
	case "aws-ecs", "ecs":
		return ECSStabilizationWindow
	case "azure-aca", "aca":
		return AzureACAStabilizationWindow
	default:
		return DefaultStabilizationWindow
	}
}

// IsSatisfied returns true if ALL three convergence pre-conditions are met:
//  1. Enough time has elapsed since submittedAt (WindowDuration).
//  2. Enough observations exist (RequiredSamples).
//  3. The trailing ConsistencyPeriod of observations all agree on ProviderState.
//
// Time is measured from submittedAt to the LATEST observation timestamp,
// not to time.Now() — this keeps the function deterministic and replay-safe.
func (w StabilizationWindow) IsSatisfied(tl ObservationTimeline, submittedAt string) bool {
	if len(tl.Observations) < w.RequiredSamples {
		return false
	}

	submitted, err := time.Parse(time.RFC3339, submittedAt)
	if err != nil {
		return false
	}

	latest, ok := LatestObservation(tl)
	if !ok {
		return false
	}

	latestTime, err := time.Parse(time.RFC3339Nano, latest.ObservedAt)
	if err != nil {
		latestTime, err = time.Parse(time.RFC3339, latest.ObservedAt)
		if err != nil {
			return false
		}
	}

	// Condition 1: enough time since submission.
	if latestTime.Sub(submitted) < time.Duration(w.WindowDuration) {
		return false
	}

	// Condition 3: trailing consistency period — all observations within the
	// consistency window must report the same ProviderState.
	if w.ConsistencyPeriod > 0 {
		cutoff := latestTime.Add(-time.Duration(w.ConsistencyPeriod))
		var recentStates []string
		for _, obs := range tl.Observations {
			obsTime, err := time.Parse(time.RFC3339Nano, obs.ObservedAt)
			if err != nil {
				obsTime, err = time.Parse(time.RFC3339, obs.ObservedAt)
				if err != nil {
					continue
				}
			}
			if !obsTime.Before(cutoff) {
				recentStates = append(recentStates, obs.ProviderState)
			}
		}
		if len(recentStates) > 1 {
			first := recentStates[0]
			for _, s := range recentStates[1:] {
				if s != first {
					return false // inconsistent trailing window
				}
			}
		}
	}

	return true
}

// SamplesSatisfied returns true if enough observations have been recorded.
func (w StabilizationWindow) SamplesSatisfied(tl ObservationTimeline) bool {
	return len(tl.Observations) >= w.RequiredSamples
}

// TimeSatisfied returns true if the window duration has elapsed between
// submittedAt and the latest observation.
func (w StabilizationWindow) TimeSatisfied(tl ObservationTimeline, submittedAt string) bool {
	submitted, err := time.Parse(time.RFC3339, submittedAt)
	if err != nil {
		return false
	}
	latest, ok := LatestObservation(tl)
	if !ok {
		return false
	}
	latestTime, err := time.Parse(time.RFC3339Nano, latest.ObservedAt)
	if err != nil {
		latestTime, err = time.Parse(time.RFC3339, latest.ObservedAt)
		if err != nil {
			return false
		}
	}
	return latestTime.Sub(submitted) >= time.Duration(w.WindowDuration)
}
