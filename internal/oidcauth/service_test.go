package oidcauth

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestRoleForGroupsUsesExactCaseSensitiveHighestPrivilege(t *testing.T) {
	t.Parallel()
	mappings := GroupMappings{
		Administrator: []string{"cairnops-admin"},
		Operator:      []string{"cairnops-ops"},
		Observer:      []string{"cairnops-read"},
	}
	if got := roleForGroups([]string{"cairnops-read", "cairnops-admin"}, mappings); got != "administrator" {
		t.Fatalf("highest privilege should win, got %q", got)
	}
	if got := roleForGroups([]string{"CairnOps-Admin"}, mappings); got != "" {
		t.Fatalf("group matching must be case-sensitive, got %q", got)
	}
}

func TestGroupsClaimMustBePresentArrayOfStringsAndMapped(t *testing.T) {
	t.Parallel()
	configuration := Configuration{GroupsClaim: "groups", Groups: GroupMappings{Operator: []string{"ops"}}}
	for name, claims := range map[string]map[string]json.RawMessage{
		"missing":   {},
		"null":      {"groups": json.RawMessage(`null`)},
		"not array": {"groups": json.RawMessage(`"ops"`)},
		"mixed":     {"groups": json.RawMessage(`["ops", 4]`)},
		"unmapped":  {"groups": json.RawMessage(`["other"]`)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := groupsAndRole(claims, configuration); !errors.Is(err, ErrNotAuthorized) {
				t.Fatalf("invalid groups produced %v", err)
			}
		})
	}
	_, role, err := groupsAndRole(map[string]json.RawMessage{"groups": json.RawMessage(`["ops"]`)}, configuration)
	if err != nil || role != "operator" {
		t.Fatalf("mapped string array was refused: role=%q error=%v", role, err)
	}
}

func TestConfigurationNormalizationRejectsLooseIdentifiersAndInsecureIssuer(t *testing.T) {
	t.Parallel()
	base := ConfigurationInput{
		Label: "Authentik", Issuer: "https://auth.example.net/", ClientID: "cairnops",
		GroupsClaim: "groups", Groups: GroupMappings{Observer: []string{"cairnops-read"}},
	}
	normalized, err := normalizeConfiguration(base)
	if err != nil || normalized.Issuer != "https://auth.example.net/" {
		t.Fatalf("valid configuration was not normalized: %#v, %v", normalized, err)
	}

	insecure := base
	insecure.Issuer = "http://auth.example.net"
	if _, err := normalizeConfiguration(insecure); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("insecure remote issuer produced %v", err)
	}
	loose := base
	loose.Groups.Observer = []string{" cairnops-read"}
	if _, err := normalizeConfiguration(loose); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("group with hidden whitespace produced %v", err)
	}
}

func TestReturnPathCannotEscapeTheInstance(t *testing.T) {
	t.Parallel()
	for _, hostile := range []string{
		"https://hostile.example", "//hostile.example/path", "javascript:alert(1)",
		"/%2f%2fhostile.example/path", "/%5chostile.example/path",
	} {
		if got := safeReturnTo(hostile); got != "/" {
			t.Fatalf("unsafe return %q became %q", hostile, got)
		}
	}
	if got := safeReturnTo("/reglages?tab=oidc"); got != "/reglages?tab=oidc" {
		t.Fatalf("local return path was lost: %q", got)
	}
}

func TestSynchronizationJitterStaysAroundFiveMinutes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	delay := nextSync(now, "stable-subject").Sub(now)
	if delay < 4*time.Minute+30*time.Second || delay > 5*time.Minute+30*time.Second {
		t.Fatalf("unexpected synchronization delay: %s", delay)
	}
}

func TestOnlyExplicitProviderRefusalsBypassTheGracePeriod(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		status    int
		immediate bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusRequestTimeout, false},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
	} {
		err := &oauth2.RetrieveError{Response: &http.Response{StatusCode: test.status}}
		if got := explicitProviderRefusal(err); got != test.immediate {
			t.Fatalf("status %d: immediate=%t, want %t", test.status, got, test.immediate)
		}
	}
}
