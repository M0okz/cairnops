package softwareupdates

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestVersionOrdering(t *testing.T) {
	for _, tt := range []struct {
		a, b  string
		want  int
		valid bool
	}{{"2.6.0", "2.9.1", -1, true}, {"v2.9.1", "2.9.1", 0, true}, {"2.10.0", "2.9.1", 1, true}, {"2.0.0-rc.2", "2.0.0-rc.10", -1, true}, {"2.0.0-rc.1", "2.0.0", -1, true}, {"latest", "2.0.0", 0, false}, {"2.0.0+build", "2.0.0", 0, true}} {
		got, ok := compareVersions(tt.a, tt.b)
		if got != tt.want || ok != tt.valid {
			t.Fatalf("%s vs %s = %d %v", tt.a, tt.b, got, ok)
		}
	}
	if inRange("3.0.0", "2.0.0", "2.9.0") || inRange("2.8.0-beta", "2.0.0", "2.9.0") || inRange("2.0.0", "2.0.0", "2.9.0") {
		t.Fatal("wrong release range")
	}
}
func TestRepositoryIncludesMissingNotesWithoutInventingVersions(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/releases") {
			return response(`[{"tag_name":"v2.9.1","body":"Fixed a documented issue"},{"tag_name":"v3.0.0","body":"Future release"},{"tag_name":"v2.8.0-beta","body":"Preview"}]`), nil
		}
		return response(`[{"name":"v2.7.0"},{"name":"v2.6.0"},{"name":"v2.9.1"}]`), nil
	})}
	c, err := Collect(context.Background(), client, Source{Kind: "github", URL: "https://github.com/example/project"}, "2.6.0", "2.9.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Notes) != 2 || c.Notes[0].Missing || !c.Notes[1].Missing || c.Notes[1].Version != "v2.7.0" {
		t.Fatalf("unexpected notes: %+v", c)
	}
}
func TestChangelogExtractsOnlyReleaseHeadingsAndSameOriginLinks(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return response("# Changelog\n## 2.9.1\nFixed database migrations.\n### Security\nFixed authentication issue.\n## 2.6.0\nOld release."), nil
	})}
	c, err := Collect(context.Background(), client, Source{Kind: "changelog", URL: "https://example.com/changelog"}, "2.6.0", "2.9.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Notes) != 1 || !strings.Contains(c.Notes[0].Body, "Fixed authentication") {
		t.Fatalf("release subsection lost: %+v", c)
	}
}
func TestPublicNetworkBoundary(t *testing.T) {
	for _, raw := range []string{"http://github.com/a/b", "https://127.0.0.1/a", "https://10.0.0.1/a", "https://x.int.homeblack.fr/a", "https://user:secret@example.com/a", "https://example.com:8443/a"} {
		if _, err := publicURL(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254", "::1", "::ffff:127.0.0.1", "fd00::1"} {
		if publicIP(netip.MustParseAddr(ip)) {
			t.Fatalf("accepted %s", ip)
		}
	}
	if _, err := publicURL("https://github.com/example/project"); err != nil {
		t.Fatal(err)
	}
}
func TestAICitationsMustExistInTheSuppliedRelease(t *testing.T) {
	c := Collection{Notes: []Note{{Version: "2.9.1", Body: "Fixed database migrations for MySQL."}}}
	p := Point{Category: "fix", Text: "Correction des migrations.", Version: "2.9.1", Quote: "Fixed database migrations"}
	if err := validateSummary(Summary{Overview: []Point{p}}, c); err != nil {
		t.Fatal(err)
	}
	p.Version = "2.8.0"
	if validateSummary(Summary{Overview: []Point{p}}, c) == nil {
		t.Fatal("accepted fabricated version")
	}
	p.Version = "2.9.1"
	p.Quote = "An invented security issue"
	if validateSummary(Summary{Overview: []Point{p}}, c) == nil {
		t.Fatal("accepted fabricated evidence")
	}
}
func TestGenerateUsesOnlyCollectedNotesAndRejectsTruncation(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "test-model" {
			t.Fatal("model ignored")
		}
		return response(`{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`), nil
	})}
	_, err := Generate(context.Background(), client, AIConfig{Endpoint: "https://provider.example/v1", Model: "test-model"}, Collection{Installed: "1.0", Target: "2.0", Notes: []Note{{Version: "2.0", Body: "Fixed a documented issue"}}})
	if err == nil {
		t.Fatal("accepted truncated AI output")
	}
}
