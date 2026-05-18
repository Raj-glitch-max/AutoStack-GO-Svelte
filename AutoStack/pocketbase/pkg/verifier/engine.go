package verifier

import (
	"fmt"
	"time"
)

// defaultLimitations documents invariant limitations of this verification engine.
// Every VerificationResult includes these to prevent overconfidence.
func defaultLimitations() []string {
	return []string{
		"Verification is based on polling observations — push-based events are not yet integrated.",
		"Provider state may lag behind actual infrastructure state by up to the policy PropagationDelay.",
		"A verified state does not guarantee traffic is flowing; endpoint health is not checked here.",
		"Replay reconstructs verification results from persisted observations — clock drift between nodes may affect staleness calculations.",
	}
}

// VerifyExecutionConsistency is the core verification engine.
//
// It is a PURE FUNCTION: given the same VerificationContext, it always returns
// the same VerificationResult. No side effects, no DB writes, no provider calls.
//
// Decision tree:
//  1. No observations → Unverifiable
//  2. Contradictions detected → Contradicted (operator review required)
//  3. All observations stale → Stale (re-query required)
//  4. Stabilization window not met → Stabilizing (or Acknowledged if very early)
//  5. Latest state is a failure → Unverifiable (with low confidence)
//  6. Stabilization met + high confidence → Verified
//  7. Stabilization met + low confidence → PartiallyVerified
//
// The caller decides what to do with the result. The engine NEVER:
//   - retries the operation
//   - modifies the timeline
//   - triggers rollback
//   - auto-advances the execution
func VerifyExecutionConsistency(ctx VerificationContext) VerificationResult {
	result := VerificationResult{
		ExecutionID: ctx.ExecutionID,
		StageID:     ctx.StageID,
		NodeID:      ctx.NodeID,
		Limitations: defaultLimitations(),
	}

	// ── Step 1: No observations at all ───────────────────────────────────
	if len(ctx.Timeline.Observations) == 0 {
		result.State = VerificationUnverifiable
		result.Confidence = ComputeConfidence(ctx.Timeline, nil)
		result.Recommendation = "no observations available — submit the operation and poll provider state before calling VerifyExecutionConsistency"
		return result
	}

	// ── Step 2: Mark stale observations using the reference time ─────────
	refTime := time.Now().UTC()
	if ctx.RefTime != "" {
		if t, err := time.Parse(time.RFC3339, ctx.RefTime); err == nil {
			refTime = t
		}
	}
	MarkStaleObservations(&ctx.Timeline, refTime, time.Duration(ctx.Policy.MaxStaleness))

	// ── Step 3: Detect contradictions ────────────────────────────────────
	contradictions := DetectContradictions(ctx.Timeline)
	result.Contradictions = contradictions
	if len(contradictions) > 0 {
		result.State = VerificationContradicted
		result.Confidence = ComputeConfidence(ctx.Timeline, contradictions)
		result.Recommendation = fmt.Sprintf(
			"%d contradictory observation(s) detected — operator review required before proceeding",
			len(contradictions),
		)
		result.Evidence = contradictionEvidence(contradictions)
		return result
	}

	// ── Step 4: All observations stale ───────────────────────────────────
	if AllStale(ctx.Timeline) {
		result.State = VerificationStale
		result.Confidence = ComputeConfidence(ctx.Timeline, nil)
		result.Recommendation = "all observations exceed the policy MaxStaleness — re-query provider before proceeding"
		result.Evidence = []string{
			fmt.Sprintf("all %d observation(s) are stale", len(ctx.Timeline.Observations)),
			fmt.Sprintf("policy MaxStaleness: %s", time.Duration(ctx.Policy.MaxStaleness)),
		}
		return result
	}

	// ── Step 5: Compute confidence from current observations ─────────────
	result.Confidence = ComputeConfidence(ctx.Timeline, nil)

	// ── Step 6: Check stabilization window ───────────────────────────────
	stabMet := ctx.StabilizationWindow.IsSatisfied(ctx.Timeline, ctx.SubmittedAt)
	result.StabilizationMet = stabMet

	if !stabMet {
		samplesMet := ctx.StabilizationWindow.SamplesSatisfied(ctx.Timeline)
		timeMet := ctx.StabilizationWindow.TimeSatisfied(ctx.Timeline, ctx.SubmittedAt)

		if !samplesMet && !timeMet {
			// Very early — just acknowledged.
			result.State = VerificationAcknowledged
			result.Recommendation = fmt.Sprintf(
				"waiting for %d observations (have %d) and window duration %s",
				ctx.StabilizationWindow.RequiredSamples,
				len(ctx.Timeline.Observations),
				time.Duration(ctx.StabilizationWindow.WindowDuration),
			)
		} else {
			// Some progress — actively stabilizing.
			result.State = VerificationStabilizing
			if !samplesMet {
				result.Recommendation = fmt.Sprintf(
					"stabilizing: need %d observations, have %d",
					ctx.StabilizationWindow.RequiredSamples, len(ctx.Timeline.Observations),
				)
			} else {
				result.Recommendation = fmt.Sprintf(
					"stabilizing: waiting for window duration %s — trailing observations must remain consistent",
					time.Duration(ctx.StabilizationWindow.WindowDuration),
				)
			}
		}
		result.Evidence = observationEvidence(ctx.Timeline)
		return result
	}

	// ── Step 7: Stabilization window met — determine convergence verdict ──
	verdict := ConvergenceVerdict(ctx.Timeline)
	switch verdict {
	case "success":
		// High-confidence verified vs. partially verified.
		if IsHighConfidence(result.Confidence, 0.70) {
			result.State = VerificationVerified
			result.VerifiedAt = time.Now().UTC().Format(time.RFC3339)
			result.Recommendation = "execution verified — provider state is stable and consistent"
		} else {
			result.State = VerificationPartiallyVerified
			result.Recommendation = "provider state is success but confidence is below threshold — additional observations recommended"
		}
	case "failure":
		result.State = VerificationUnverifiable
		result.Confidence.ProviderConfidence = clamp(result.Confidence.ProviderConfidence - 0.20)
		result.Recommendation = "provider reports a failure state — operator review required before determining next action"
	case "transitional":
		// Still in progress — re-evaluate after more observations.
		result.State = VerificationStabilizing
		result.Recommendation = "provider still in transitional state — continue polling and re-verify"
	default:
		result.State = VerificationUnverifiable
		result.Recommendation = "provider state is uninterpretable — additional observations required"
	}

	result.Evidence = observationEvidence(ctx.Timeline)
	return result
}

func contradictionEvidence(contradictions []Contradiction) []string {
	ev := make([]string, 0, len(contradictions))
	for _, c := range contradictions {
		ev = append(ev, fmt.Sprintf(
			"contradiction %s: %s=%q vs %s=%q — %s",
			c.ContradictionID,
			c.ObservationA.ObservedAt, c.ObservationA.ProviderState,
			c.ObservationB.ObservedAt, c.ObservationB.ProviderState,
			c.Reason,
		))
	}
	return ev
}

func observationEvidence(tl ObservationTimeline) []string {
	ev := make([]string, 0, len(tl.Observations))
	for _, obs := range tl.Observations {
		staleNote := ""
		if obs.StaleFlag {
			staleNote = " [STALE]"
		}
		ev = append(ev, fmt.Sprintf(
			"[%s] state=%q trust=%s source=%s%s",
			obs.ObservedAt, obs.ProviderState, obs.TrustLevel, obs.VerificationSource, staleNote,
		))
	}
	return ev
}
