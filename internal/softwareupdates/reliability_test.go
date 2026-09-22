package softwareupdates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGenerateResolvesEvidenceWithoutModelRecopy(t *testing.T) {
	body := "Fix search in shared user queries\r\n\tAdditional details."
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return response(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"overview\":[{\"category\":\"fix\",\"text\":\"Correction des recherches partagées.\",\"evidence_id\":\"e1\"}],\"details\":[]}"}}]}`), nil
	})}
	got, err := Generate(context.Background(), client, AIConfig{Endpoint: "https://provider.example/v1"}, Collection{Installed: "1.28.1", Target: "1.30.0", Notes: []Note{{Version: "1.30.0", Body: body}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Overview) != 1 || got.Overview[0].Version != "1.30.0" || got.Overview[0].Quote != "Fix search in shared user queries" {
		t.Fatalf("evidence lost: %+v", got)
	}
}

func TestWorkerAdoptsArgusSourceWithoutConfirmation(t *testing.T) {
	s, id, _ := fixture(t)
	w := NewWorker(s, nil)
	w.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/repos/example/project/tags" {
			return response(`[]`), nil
		}
		return response(`[{"tag_name":"2.9.1","body":"Fixed database migrations for MySQL."}]`), nil
	})}
	if err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if v.ConfirmedAt == nil || v.Source.URL != "https://github.com/example/project" || v.State != "awaiting_ai" {
		t.Fatalf("Argus source still blocked: state=%s source=%+v", v.State, v.Source)
	}
}

func TestEvidenceRejectsUnknownIDsAndBoundsRepair(t *testing.T) {
	for _, repair := range []bool{false, true} {
		t.Run(fmt.Sprint(repair), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				id := "invented"
				if repair && calls == 2 {
					id = "e1"
				}
				return response(fmt.Sprintf(`{"choices":[{"finish_reason":"stop","message":{"content":%q}}]}`, fmt.Sprintf(`{"overview":[{"category":"fix","text":"Correction documentée","evidence_id":%q}],"details":[]}`, id))), nil
			})}
			result, err := Generate(context.Background(), client, AIConfig{Endpoint: "https://provider.example/v1"}, Collection{Notes: []Note{{Version: "2.0", Body: "Fixed database migrations for MySQL."}}})
			if calls != 2 || (err == nil) != repair {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if repair && result.Overview[0].Version != "2.0" {
				t.Fatal("wrong release")
			}
		})
	}
}

func TestEvidencePreservesUnicodeAndOnlySuppliedReferences(t *testing.T) {
	body := "## Fixes\r\n" + strings.Repeat("é🙂", 1200) + "\r\nCorrection complète des recherches partagées."
	evidence := excerpts([]Note{{Version: "2.0", Body: body}})
	for _, e := range evidence {
		if !utf8.ValidString(e.Quote) || !strings.Contains(body, e.Quote) || len(e.Quote) > 2000 {
			t.Fatalf("invalid excerpt: %s", e.ID)
		}
	}
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return response(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"overview\":[{\"category\":\"fix\",\"text\":\"Correction\",\"evidence_id\":\"e2\"}],\"details\":[]}"}}]}`), nil
	})}
	_, err := generateOne(context.Background(), client, AIConfig{Endpoint: "https://provider.example/v1"}, nil, evidence[:1])
	if err == nil {
		t.Fatal("accepted a reference outside this request")
	}
}

func TestArgusSourceChangesAndManualOverride(t *testing.T) {
	s, id, actor := fixture(t)
	ctx := context.Background()
	sync := func() Service {
		t.Helper()
		if err := s.syncArgusSources(ctx); err != nil {
			t.Fatal(err)
		}
		v, err := s.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	update := func(url string) {
		t.Helper()
		_, err := s.pool.Exec(ctx, `UPDATE cairnops_connector_bindings SET metadata=jsonb_set(metadata,'{version_url}',to_jsonb($2::text)) WHERE id=$1::uuid`, id, url)
		if err != nil {
			t.Fatal(err)
		}
	}
	first := sync()
	same := sync()
	if same.Revision != first.Revision || same.SourceOrigin != "argus" {
		t.Fatal("adoption churn")
	}
	update("https://github.com/example/next")
	changed, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision <= first.Revision {
		t.Fatal("running analysis was not invalidated immediately")
	}
	next := sync()
	if next.Source.URL != "https://github.com/example/next" {
		t.Fatal("source not refreshed")
	}
	update("https://127.0.0.1/private")
	invalid := sync()
	if invalid.ConfirmedAt != nil || invalid.Source.URL != "" || invalid.State != "awaiting_source" {
		t.Fatal("unsafe source accepted or obsolete source retained")
	}
	manual := Source{Kind: "github", URL: "https://github.com/custom/project", Software: "Custom"}
	if err := s.Confirm(ctx, id, actor, manual); err != nil {
		t.Fatal(err)
	}
	update("https://github.com/example/third")
	v := sync()
	if v.Source != manual || v.SourceOrigin != "manual" {
		t.Fatal("manual override lost")
	}
}

func TestGenerateConsolidatesMultipleChunksWithExactEvidence(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var input struct {
			Evidence []evidenceExcerpt `json:"evidence"`
		}
		if err := json.Unmarshal([]byte(payload.Messages[1].Content), &input); err != nil {
			t.Fatal(err)
		}
		if len(input.Evidence) == 0 {
			t.Fatal("missing evidence")
		}
		return response(fmt.Sprintf(`{"choices":[{"finish_reason":"stop","message":{"content":%q}}]}`, fmt.Sprintf(`{"overview":[{"category":"fix","text":"Correction documentée","evidence_id":%q}],"details":[]}`, input.Evidence[0].ID))), nil
	})}
	c := Collection{Notes: []Note{{Version: "2.0", Body: strings.Repeat("First documented correction.\n", 1600)}, {Version: "3.0", Body: strings.Repeat("Second documented correction.\n", 1600)}}}
	got, err := Generate(context.Background(), client, AIConfig{Endpoint: "https://provider.example/v1"}, c)
	if err != nil || calls != 3 || len(got.Overview) != 1 {
		t.Fatalf("calls=%d summary=%+v err=%v", calls, got, err)
	}
	if err := validateSummary(got, c); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryReducesOversizedPagesWithoutSkippingReleases(t *testing.T) {
	requested := []string{}
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/tags") {
			return response(`[]`), nil
		}
		size, page := r.URL.Query().Get("per_page"), r.URL.Query().Get("page")
		requested = append(requested, size+":"+page)
		if size == "100" && page == "1" {
			rows := make([]map[string]string, 100)
			for i := range rows {
				rows[i] = map[string]string{"tag_name": fmt.Sprintf("3.0.%d", 200-i), "body": "Documented release improvements."}
			}
			return response(string(mustJSON(rows))), nil
		}
		if size == "100" {
			return response(strings.Repeat("x", (2<<20)+1)), nil
		}
		if size != "50" || page != "3" {
			t.Fatalf("pagination skipped entries: %s:%s", size, page)
		}
		return response(`[{"tag_name":"3.0.100","body":"Fixed database migrations for MySQL."}]`), nil
	})}
	c, err := Collect(context.Background(), client, Source{Kind: "github", URL: "https://github.com/example/project", Software: "Project"}, "3.0.99", "3.0.200")
	if err != nil || c.Incomplete || len(c.Notes) != 101 {
		t.Fatalf("notes=%d incomplete=%v err=%v requests=%v", len(c.Notes), c.Incomplete, err, requested)
	}
	if strings.Join(requested, ",") != "100:1,100:2,50:3" {
		t.Fatalf("requests=%v", requested)
	}
}

func TestRepositoryKeepsResponseLimitForOversizedSingleRelease(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return response(strings.Repeat("x", (2<<20)+1)), nil
	})}
	_, err := Collect(context.Background(), client, Source{Kind: "github", URL: "https://github.com/example/project", Software: "Project"}, "1.0", "2.0")
	if !errors.Is(err, errResponseTooLarge) || calls != 5 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
