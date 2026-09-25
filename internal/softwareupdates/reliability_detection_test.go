package softwareupdates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSuggestReadsTheRepositoryBehindAGitHubAPIAddress(t *testing.T) {
	got := Suggest("https://api.github.com/repos/Sonarr/Sonarr/releases/latest")
	if got == nil || got.Kind != "github" || got.URL != "https://github.com/Sonarr/Sonarr" || got.Software != "Sonarr" {
		t.Fatalf("GitHub API address must designate its repository: %+v", got)
	}
}

func TestRepositoryStopsPagingOnceReleasesPrecedeTheInstallation(t *testing.T) {
	page := func(major, minor int) string {
		rows := make([]string, 0, 100)
		for patch := 99; patch >= 0; patch-- {
			rows = append(rows, fmt.Sprintf(`{"tag_name":"v%d.%d.%d","body":"Notes"}`, major, minor, patch))
		}
		return "[" + strings.Join(rows, ",") + "]"
	}
	releases := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/tags") {
			return response(`[]`), nil
		}
		releases++
		switch r.URL.Query().Get("page") {
		case "1":
			return response(page(2, 1)), nil
		case "2":
			return response(page(1, 9)), nil
		}
		t.Fatalf("requested page %s after reaching releases older than the installation", r.URL.Query().Get("page"))
		return nil, nil
	})}
	c, err := Collect(context.Background(), client, Source{Kind: "github", URL: "https://github.com/example/paged"}, "2.0.0", "2.1.99")
	if err != nil {
		t.Fatal(err)
	}
	if releases != 2 || len(c.Notes) != 100 {
		t.Fatalf("releases requests=%d notes=%d", releases, len(c.Notes))
	}
}

func TestRateLimitSuspendsTheHostUntilItsReset(t *testing.T) {
	calls := 0
	reset := time.Now().Add(10 * time.Minute).Unix()
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		header := http.Header{}
		header.Set("X-RateLimit-Remaining", "0")
		header.Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		return &http.Response{StatusCode: http.StatusForbidden, Header: header, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}
	for attempt := 0; attempt < 2; attempt++ {
		_, err := request(context.Background(), client, "GET", "https://quota.example.test/repos/a/b/releases", "", nil)
		var limited *RateLimitedError
		if !errors.As(err, &limited) || limited.Host != "quota.example.test" || limited.Until.Unix() != reset {
			t.Fatalf("attempt %d: expected a rate limit until the reset, got %v", attempt, err)
		}
	}
	if calls != 1 {
		t.Fatalf("a suspended host must not receive further requests: %d calls", calls)
	}
}

func TestOrdinaryForbiddenResponseIsNotARateLimit(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	_, err := request(context.Background(), client, "GET", "https://forbidden.example.test/changelog", "", nil)
	var limited *RateLimitedError
	if errors.As(err, &limited) || err == nil || err.Error() != "remote HTTP 403" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventsSeparateInstallationsFromNewTargets(t *testing.T) {
	at := func(hour int) time.Time { return time.Date(2026, 9, 21, hour, 0, 0, 0, time.UTC) }
	history := []History{ // du plus récent au plus ancien
		{Installed: "0.1.146", Target: "0.1.147", ObservedAt: at(5)},
		{Installed: "0.1.147", Target: "0.1.147", ObservedAt: at(4)},
		{Installed: "0.1.145", Target: "0.1.147", ObservedAt: at(3)},
		{Installed: "0.1.145", Target: "0.1.146", ObservedAt: at(2)},
		{Installed: "0.1.145", Target: "0.1.145", ObservedAt: at(1)},
	}
	got := events(history, false)
	want := []Event{
		{Kind: "installed", Version: "0.1.146", Previous: "0.1.147", Direction: "rollback", ObservedAt: at(5)},
		{Kind: "installed", Version: "0.1.147", Previous: "0.1.145", Direction: "upgrade", ObservedAt: at(4)},
		{Kind: "target", Version: "0.1.147", Previous: "0.1.146", ObservedAt: at(3)},
		{Kind: "target", Version: "0.1.146", Previous: "0.1.145", ObservedAt: at(2)},
		{Kind: "first", Version: "0.1.145", Target: "0.1.145", ObservedAt: at(1)},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events: got %+v, want %+v", got, want)
	}
	if truncated := events(history, true); len(truncated) != 4 || truncated[3].Kind == "first" {
		t.Fatalf("a truncated history has no first observation: %+v", truncated)
	}
}
