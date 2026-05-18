package verifier

import "fmt"

const (
	// Degradation constants — how much each factor reduces confidence.
	degradePerContradiction = 0.40
	degradePerStaleObs      = 0.15
	degradePerLowTrustObs   = 0.05
	degradeNoSamples        = 0.50
	degradeUnverifiable     = 0.30
)

// ComputeConfidence derives a three-dimensional confidence score from the timeline
// and any detected contradictions.
//
// Confidence starts at 1.0 and degrades based on:
//   - Contradictions: -0.40 per contradiction (ProviderConfidence + VerificationConfidence)
//   - Stale observations: -0.15 per stale obs (ExecutionConfidence + VerificationConfidence)
//   - Low-trust observations: -0.05 per low-trust obs (ProviderConfidence)
//   - No observations at all: -0.50 across all three dimensions
//
// All values are clamped to [0.0, 1.0].
func ComputeConfidence(tl ObservationTimeline, contradictions []Contradiction) ConfidenceScore {
	score := ConfidenceScore{
		ExecutionConfidence:    1.0,
		ProviderConfidence:     1.0,
		VerificationConfidence: 1.0,
	}

	var reasons []string

	if len(tl.Observations) == 0 {
		score.ExecutionConfidence -= degradeNoSamples
		score.ProviderConfidence -= degradeNoSamples
		score.VerificationConfidence -= degradeNoSamples
		reasons = append(reasons, "no provider observations available")
		score.DegradationReasons = reasons
		return clampScore(score)
	}

	// Degrade for contradictions.
	if len(contradictions) > 0 {
		n := float64(len(contradictions))
		score.ProviderConfidence -= degradePerContradiction * n
		score.VerificationConfidence -= degradePerContradiction * n
		reasons = append(reasons, fmt.Sprintf("%d contradiction(s) detected in observation timeline", len(contradictions)))
	}

	// Degrade per stale and per low-trust observation.
	staleCount := 0
	lowTrustCount := 0
	for _, obs := range tl.Observations {
		if obs.StaleFlag {
			staleCount++
		}
		if obs.TrustLevel == TrustLow {
			lowTrustCount++
		}
	}

	if staleCount > 0 {
		score.ExecutionConfidence -= degradePerStaleObs * float64(staleCount)
		score.VerificationConfidence -= degradePerStaleObs * float64(staleCount)
		reasons = append(reasons, fmt.Sprintf("%d stale observation(s) — re-query provider recommended", staleCount))
	}

	if lowTrustCount > 0 {
		score.ProviderConfidence -= degradePerLowTrustObs * float64(lowTrustCount)
		reasons = append(reasons, fmt.Sprintf("%d low-trust observation(s)", lowTrustCount))
	}

	// If latest observation trust is low, further degrade verification confidence.
	if latest, ok := LatestObservation(tl); ok && latest.TrustLevel == TrustLow {
		score.VerificationConfidence -= degradeUnverifiable
		reasons = append(reasons, "most recent observation has low trust")
	}

	score.DegradationReasons = reasons
	return clampScore(score)
}

// clampScore ensures all confidence values are within [0.0, 1.0].
func clampScore(s ConfidenceScore) ConfidenceScore {
	s.ExecutionConfidence = clamp(s.ExecutionConfidence)
	s.ProviderConfidence = clamp(s.ProviderConfidence)
	s.VerificationConfidence = clamp(s.VerificationConfidence)
	return s
}

func clamp(v float64) float64 {
	if v < 0.0 {
		return 0.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

// IsHighConfidence returns true if all three confidence dimensions are ≥ threshold.
func IsHighConfidence(score ConfidenceScore, threshold float64) bool {
	return score.ExecutionConfidence >= threshold &&
		score.ProviderConfidence >= threshold &&
		score.VerificationConfidence >= threshold
}

// MinConfidence returns the lowest of the three confidence dimensions.
func MinConfidence(score ConfidenceScore) float64 {
	min := score.ExecutionConfidence
	if score.ProviderConfidence < min {
		min = score.ProviderConfidence
	}
	if score.VerificationConfidence < min {
		min = score.VerificationConfidence
	}
	return min
}
