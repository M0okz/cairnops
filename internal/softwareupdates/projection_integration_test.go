package softwareupdates

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func setMetadata(t *testing.T, s *Store, id, meta string) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), `UPDATE cairnops_connector_bindings SET metadata=$2::jsonb WHERE id=$1::uuid`, id, meta); err != nil {
		t.Fatal(err)
	}
}

func listOne(t *testing.T, s *Store) Service {
	t.Helper()
	services, err := s.List(context.Background(), "")
	if err != nil || len(services) != 1 {
		t.Fatalf("list: %v %+v", err, services)
	}
	return services[0]
}

func TestListClassifiesEachServiceOnce(t *testing.T) {
	s, id, _ := fixture(t)
	v := listOne(t, s)
	if v.Group != GroupApply || v.Situation != "update" || v.Level != "minor" || v.ResourceName != "private-service-name" {
		t.Fatalf("stable newer target must be applied: %+v", v)
	}
	setMetadata(t, s, id, `{"deployed_version":"2.6.0","latest_version":"3.0.0-rc1"}`)
	if v = listOne(t, s); v.Group != GroupReview || v.Situation != "prerelease" {
		t.Fatalf("prerelease target must be reviewed: %+v", v)
	}
	setMetadata(t, s, id, `{"deployed_version":"2.6.1","latest_version":"2.6.0"}`)
	if v = listOne(t, s); v.Group != GroupReview || v.Situation != "target_older" {
		t.Fatalf("older target must be reviewed: %+v", v)
	}
	setMetadata(t, s, id, `{"deployed_version":"v2.6.1","latest_version":"2.6.1"}`)
	if v = listOne(t, s); v.Group != GroupCurrent || v.Situation != "current" {
		t.Fatalf("equivalent versions are current: %+v", v)
	}
	setMetadata(t, s, id, `{"deployed_version":"2.6.1","latest_version":"2.7.0","skipped":true}`)
	if v = listOne(t, s); v.Group != GroupCurrent || !v.Skipped || v.Situation != "update" {
		t.Fatalf("a version skipped in Argus requires no action: %+v", v)
	}
	setMetadata(t, s, id, `{"unknown":true,"unknown_reason":"deployed_version_query_failed","deployed_version":"2.6.1","latest_version":"2.8.0","deployed_version_query_ok":false}`)
	v = listOne(t, s)
	if v.Known || v.Group != GroupReview || v.VerificationIssue != IssueInstalledUnreadable || v.Installed != "2.6.1" || v.Target != "2.7.0" {
		t.Fatalf("unverified versions keep their last valid values and reason: %+v", v)
	}
}

func TestWorkerSkipsComparisonsWithoutNewerTarget(t *testing.T) {
	s, id, actor := fixture(t)
	ctx := context.Background()
	setMetadata(t, s, id, `{"deployed_version":"0.1.147","latest_version":"0.1.146","version_url":"https://github.com/example/project"}`)
	if err := s.Confirm(ctx, id, actor, Source{Kind: "github", URL: "https://github.com/example/project", Software: "Project"}); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(s, slog.Default())
	w.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("no notes exist between %s and an older target", r.URL)
		return nil, nil
	})}
	if err := w.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get(ctx, id); err != nil || v.State != "not_applicable" || v.LastError != "" {
		t.Fatalf("older target must settle without retrying: %+v %v", v, err)
	}
}

func TestWorkerWaitsDailyForUnpublishedNotes(t *testing.T) {
	s, id, actor := fixture(t)
	ctx := context.Background()
	if err := s.Confirm(ctx, id, actor, Source{Kind: "github", URL: "https://github.com/example/tags-only", Software: "Project"}); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(s, slog.Default())
	w.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/tags") {
			return response(`[{"name":"2.9.1"},{"name":"2.7.0"}]`), nil
		}
		return response(`[]`), nil
	})}
	if err := w.tick(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get(ctx, id)
	if err != nil || v.State != "notes_unavailable" || v.NextCheckAt == nil || time.Until(*v.NextCheckAt) < 23*time.Hour {
		t.Fatalf("missing notes must be checked daily: %+v %v", v, err)
	}
}
