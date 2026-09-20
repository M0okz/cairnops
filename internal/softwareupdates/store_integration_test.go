package softwareupdates

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func fixture(t *testing.T) (*Store, string, string) {
	t.Helper()
	p := testsupport.Pool(t)
	ctx := context.Background()
	box, _ := secretbox.New(make([]byte, 32))
	s := NewStore(p, box)
	var actor, target, connector, id string
	for _, q := range []struct {
		sql string
		out *string
	}{
		{`INSERT INTO cairnops_users(username,display_name,password_hash,role) VALUES('release-admin','Release admin','unused','administrator') RETURNING id::text`, &actor},
		{`INSERT INTO cairnops_targets(name) VALUES('private-service-name') RETURNING id::text`, &target},
		{`INSERT INTO cairnops_connectors(kind,name,endpoint,credential_sealed,status,encrypted_transport) VALUES('argus','Argus','https://argus.int.homeblack.fr','0123456789012345678901234567890123','connected',true) RETURNING id::text`, &connector},
	} {
		if err := p.QueryRow(ctx, q.sql).Scan(q.out); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.QueryRow(ctx, `INSERT INTO cairnops_connector_bindings(connector_id,target_id,external_id,external_name,metadata) VALUES($1::uuid,$2::uuid,'software','private-service-name','{"deployed_version":"2.6.0","latest_version":"2.9.1","version_url":"https://github.com/example/project"}') RETURNING id::text`, connector, target).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return s, id, actor
}
func TestHistoryPreservesRollbackAndUnknownValues(t *testing.T) {
	s, id, _ := fixture(t)
	ctx := context.Background()
	update := func(meta string) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, `UPDATE cairnops_connector_bindings SET metadata=$2::jsonb WHERE id=$1::uuid`, id, meta); err != nil {
			t.Fatal(err)
		}
	}
	update(`{"deployed_version":"2.9.1","latest_version":"2.9.1"}`)
	update(`{"deployed_version":"2.9.1","latest_version":"2.9.1"}`)
	update(`{"deployed_version":"2.6.0","latest_version":"2.9.1"}`)
	update(`{"unknown":true,"deployed_version":"bad","latest_version":"bad"}`)
	v, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Known || v.Installed != "2.6.0" || len(v.History) != 3 {
		t.Fatalf("incorrect history: %+v", v)
	}
	if v.History[1].Installed != "2.9.1" {
		t.Fatal("rollback lost")
	}
}
func TestWorkerCachesNotesAndRetriesAIWithoutTouchingArgus(t *testing.T) {
	s, id, actor := fixture(t)
	ctx := context.Background()
	source := Source{Kind: "github", URL: "https://github.com/example/project", Software: "Project"}
	if err := s.Confirm(ctx, id, actor, source); err != nil {
		t.Fatal(err)
	}
	calls := 0
	w := NewWorker(s, slog.Default())
	w.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/tags") {
			return response(`[]`), nil
		}
		return response(`[{"tag_name":"2.9.1","body":"Fixed database migrations for MySQL."}]`), nil
	})}
	if err := w.tick(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "awaiting_ai" || v.Collection == nil || len(v.Collection.Notes) != 1 || calls != 2 {
		t.Fatalf("notes not retained: %+v calls=%d", v, calls)
	}
	cfg := AIConfig{Enabled: true, Endpoint: "https://provider.example/v1", Model: "test", APIKey: "private-key"}
	if err := s.SaveConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	safe, err := s.Config(ctx)
	if err != nil || safe.APIKey != "" || !safe.KeyConfigured {
		t.Fatal("key disclosure or save failed")
	}
	w.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			return nil, fmt.Errorf("provider offline")
		}
		if strings.HasSuffix(r.URL.Path, "/tags") {
			return response(`[]`), nil
		}
		return response(`[{"tag_name":"2.9.1","body":"Fixed database migrations for MySQL."}]`), nil
	})}
	if err := w.tick(ctx); err != nil {
		t.Fatal(err)
	}
	v, err = s.Get(ctx, id)
	if err != nil || v.State != "retry" || v.Collection == nil || !v.Known {
		t.Fatalf("unexpected failure state: %+v %v", v, err)
	}
	cfg.Endpoint = "https://other.example/v1"
	cfg.APIKey = ""
	if s.SaveConfig(ctx, cfg) == nil {
		t.Fatal("reused credential on new provider")
	}
}
func TestStaleAIResultIsNotPublished(t *testing.T) {
	s, id, actor := fixture(t)
	ctx := context.Background()
	if err := s.Confirm(ctx, id, actor, Source{Kind: "github", URL: "https://github.com/example/project", Software: "Project"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveConfig(ctx, AIConfig{Enabled: true, Endpoint: "https://provider.example/v1", Model: "test", APIKey: "private-key"}); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(s, slog.Default())
	w.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			if _, err := s.pool.Exec(ctx, `UPDATE cairnops_connector_bindings SET metadata=metadata||'{"latest_version":"2.10.0"}' WHERE id=$1::uuid`, id); err != nil {
				t.Fatal(err)
			}
			return response(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"overview\":[{\"category\":\"fix\",\"text\":\"Correction MySQL\",\"version\":\"2.9.1\",\"quote\":\"Fixed database migrations for MySQL.\"}],\"details\":[]}"}}]}`), nil
		}
		if strings.HasSuffix(r.URL.Path, "/tags") {
			return response(`[]`), nil
		}
		return response(`[{"tag_name":"2.9.1","body":"Fixed database migrations for MySQL."}]`), nil
	})}
	if err := w.tick(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Analyses) != 0 || v.Target != "2.10.0" || v.State != "pending" {
		t.Fatalf("stale analysis published: %+v", v)
	}
}

func TestWorkerReusesExactResultAndReanalysesChangedNotes(t *testing.T) {
	s, id, actor := fixture(t)
	ctx := context.Background()
	if err := s.Confirm(ctx, id, actor, Source{Kind: "github", URL: "https://github.com/example/project", Software: "Project"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveConfig(ctx, AIConfig{Enabled: true, Endpoint: "https://provider.example/v1", Model: "test", APIKey: "private-key"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	body := "Fixed database migrations for MySQL."
	w := NewWorker(s, slog.Default())
	w.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			calls++
			return response(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"overview\":[{\"category\":\"fix\",\"text\":\"Correction MySQL\",\"version\":\"2.9.1\",\"quote\":\"Fixed database migrations for MySQL.\"}],\"details\":[]}"}}]}`), nil
		}
		if strings.HasSuffix(r.URL.Path, "/tags") {
			return response(`[]`), nil
		}
		return response(fmt.Sprintf(`[{"tag_name":"2.9.1","body":%q}]`, body)), nil
	})}
	for iteration := 0; iteration < 3; iteration++ {
		if iteration == 2 {
			body += " Added documentation."
		}
		if _, err := s.pool.Exec(ctx, `UPDATE cairnops_software_services SET next_check_at=now() WHERE binding_id=$1::uuid`, id); err != nil {
			t.Fatal(err)
		}
		if err := w.tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	v, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(v.Analyses) != 2 || !v.Analyses[0].Current || v.Analyses[1].Current || len(v.Analyses[1].Notes) != 1 {
		t.Fatalf("incorrect caching: calls=%d result=%+v", calls, v)
	}
	if strings.Contains(v.Analyses[1].Notes[0].Body, "Added documentation") {
		t.Fatal("historical note snapshot was overwritten")
	}
}
