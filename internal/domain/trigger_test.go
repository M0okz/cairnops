package domain

import "testing"

func TestTriggerPolicyRequiresRepeatedEvidence(t *testing.T) {
	t.Parallel()

	policy := TriggerPolicy{FailureThreshold: 3, RecoveryThreshold: 2}
	streaks := TriggerStreaks{}

	for attempt := 1; attempt <= 2; attempt++ {
		decision := policy.Evaluate(streaks, OutcomeUnhealthy)
		if decision.Triggered {
			t.Fatalf("observation %d should not trigger below the failure threshold", attempt)
		}
		streaks = decision.Streaks
	}

	decision := policy.Evaluate(streaks, OutcomeUnhealthy)
	if !decision.Triggered {
		t.Fatal("the third unhealthy observation should trigger")
	}
	if decision.Streaks.Unhealthy != 3 || decision.Streaks.Healthy != 0 {
		t.Fatalf("unexpected streaks %+v", decision.Streaks)
	}
}

func TestTriggerPolicyResetsOnOppositeEvidence(t *testing.T) {
	t.Parallel()

	policy := TriggerPolicy{FailureThreshold: 3, RecoveryThreshold: 2}
	streaks := policy.Evaluate(policy.Evaluate(TriggerStreaks{}, OutcomeUnhealthy).Streaks, OutcomeUnhealthy).Streaks
	if streaks.Unhealthy != 2 {
		t.Fatalf("expected two unhealthy observations, got %+v", streaks)
	}

	decision := policy.Evaluate(streaks, OutcomeHealthy)
	if decision.Streaks.Unhealthy != 0 || decision.Streaks.Healthy != 1 {
		t.Fatalf("a healthy observation must restart the unhealthy streak, got %+v", decision.Streaks)
	}
	if decision.Recovered {
		t.Fatal("a single healthy observation should not confirm recovery")
	}

	decision = policy.Evaluate(decision.Streaks, OutcomeHealthy)
	if !decision.Recovered {
		t.Fatal("the second healthy observation should confirm recovery")
	}
}

func TestTriggerPolicyTreatsUnknownAsInconclusive(t *testing.T) {
	t.Parallel()

	policy := TriggerPolicy{FailureThreshold: 2, RecoveryThreshold: 2}
	streaks := policy.Evaluate(TriggerStreaks{}, OutcomeUnhealthy).Streaks

	decision := policy.Evaluate(streaks, OutcomeUnknown)
	if decision.Triggered || decision.Recovered {
		t.Fatal("an unknown observation must conclude nothing")
	}
	if decision.Streaks != streaks {
		t.Fatalf("an unknown observation must leave the streaks intact, got %+v", decision.Streaks)
	}
}

func TestTriggerPolicyKeepsStreaksBounded(t *testing.T) {
	t.Parallel()

	policy := TriggerPolicy{FailureThreshold: 2, RecoveryThreshold: 1}
	streaks := TriggerStreaks{}
	for range 50 {
		streaks = policy.Evaluate(streaks, OutcomeUnhealthy).Streaks
	}
	if streaks.Unhealthy != policy.FailureThreshold {
		t.Fatalf("expected the unhealthy streak to stay capped at %d, got %d", policy.FailureThreshold, streaks.Unhealthy)
	}
}

func TestTriggerPolicyValidation(t *testing.T) {
	t.Parallel()

	for _, policy := range []TriggerPolicy{
		{FailureThreshold: 0, RecoveryThreshold: 2},
		{FailureThreshold: 11, RecoveryThreshold: 2},
		{FailureThreshold: 3, RecoveryThreshold: 0},
		{FailureThreshold: 3, RecoveryThreshold: 11},
	} {
		if err := policy.Validate(); err == nil {
			t.Fatalf("expected %+v to be rejected", policy)
		}
	}
	if err := (TriggerPolicy{FailureThreshold: 1, RecoveryThreshold: 10}).Validate(); err != nil {
		t.Fatalf("expected a valid policy, got %v", err)
	}
}
