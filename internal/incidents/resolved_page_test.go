package incidents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestResolvedCursorRejectsInvalidAndChangedFilters(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	base := ResolvedPageOptions{Limit: 50, Query: "storage"}
	cursor, err := base.cursor(now)
	if err != nil {
		t.Fatal(err)
	}
	cursor.At = now.Add(-time.Hour)
	cursor.ID = "10000000-0000-0000-0000-000000000001"
	data, _ := json.Marshal(cursor)
	base.Cursor = base64.RawURLEncoding.EncodeToString(data)
	if got, err := base.cursor(now.Add(time.Hour)); err != nil || !got.Snapshot.Equal(now) {
		t.Fatalf("cursor must keep its original snapshot: %#v %v", got, err)
	}
	changed := base
	changed.Query = "different"
	tooLate := cursor
	tooLate.At = now.Add(time.Second)
	invalidDate, _ := json.Marshal(tooLate)
	for name, options := range map[string]ResolvedPageOptions{
		"changed filter":        changed,
		"invalid encoding":      {Limit: 50, Cursor: "not!base64"},
		"oversized cursor":      {Limit: 50, Cursor: strings.Repeat("a", 2049)},
		"cursor after snapshot": {Limit: 50, Query: "storage", Cursor: base64.RawURLEncoding.EncodeToString(invalidDate)},
		"invalid severity":      {Limit: 50, Severity: "catastrophic"},
		"invalid target":        {Limit: 50, TargetID: "not-a-uuid"},
		"empty page":            {Limit: 0},
		"oversized query":       {Limit: 50, Query: strings.Repeat("é", 201)},
		"reversed dates":        {Limit: 50, From: &now, Before: &cursor.At},
		"empty date range":      {Limit: 50, From: &now, Before: &now},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewService(&serviceStore{}, nil).ListResolvedPage(context.Background(), options); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
	// Limit affects transport, not membership, so increasing it is safe.
	base.Limit = 100
	if _, err := base.cursor(now); err != nil {
		t.Fatalf("changing page size must preserve the cursor: %v", err)
	}
}

func TestResolvedCursorCanonicalizesTimeZoneOffsets(t *testing.T) {
	t.Parallel()
	from, _ := time.Parse(time.RFC3339, "2026-09-01T00:00:00+02:00")
	options := ResolvedPageOptions{Limit: 50, From: &from}
	now := time.Now()
	cursor, err := options.cursor(now)
	if err != nil {
		t.Fatal(err)
	}
	cursor.At, cursor.ID = now.Add(-time.Minute), "10000000-0000-0000-0000-000000000001"
	encoded, _ := json.Marshal(cursor)
	options.Cursor = base64.RawURLEncoding.EncodeToString(encoded)
	utc := from.UTC()
	options.From = &utc
	if _, err := options.cursor(now); err != nil {
		t.Fatalf("equivalent instants must accept the same cursor: %v", err)
	}
}
