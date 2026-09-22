package incidents

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/alerttext"
	"github.com/M0okz/cairnops/internal/synthesis"
)

// ResolvedPageOptions filters by the resolution instant, not the opening date.
// Before is exclusive so clients can express complete calendar days in their zone.
type ResolvedPageOptions struct {
	Limit     int
	TargetID  string
	NatureKey string
	Severity  Severity
	Query     string
	From      *time.Time
	Before    *time.Time
	Cursor    string
}

type IncidentFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type IncidentFilterOptions struct {
	Targets []IncidentFilterOption `json:"targets"`
	Natures []IncidentFilterOption `json:"natures"`
}

type ResolvedPage struct {
	Incidents  []Incident             `json:"incidents"`
	NextCursor string                 `json:"next_cursor,omitempty"`
	Filters    *IncidentFilterOptions `json:"filters,omitempty"`
}

type resolvedCursor struct {
	Version  int       `json:"v"`
	Filter   string    `json:"filter"`
	Snapshot time.Time `json:"snapshot"`
	At       time.Time `json:"at"`
	ID       string    `json:"id"`
}

var incidentUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (options ResolvedPageOptions) cursor(now time.Time) (resolvedCursor, error) {
	invalid := func(message string) (resolvedCursor, error) {
		return resolvedCursor{}, fmt.Errorf("%w: %s", ErrInvalidInput, message)
	}
	if options.Limit < 1 || options.Limit > 500 {
		return invalid("limit must be between 1 and 500")
	}
	if options.TargetID != "" && !incidentUUID.MatchString(options.TargetID) {
		return invalid("invalid target ID")
	}
	if options.Severity != "" && options.Severity != SeverityCritical && options.Severity != SeverityMajor && options.Severity != SeverityWarning && options.Severity != SeverityInformation {
		return invalid("severity must be critical, major, warning, or information")
	}
	if len([]rune(options.Query)) > 200 || len(options.NatureKey) > 500 {
		return invalid("search or nature filter is too long")
	}
	if options.From != nil && options.Before != nil && !options.From.Before(*options.Before) {
		return invalid("resolved_from must precede resolved_before")
	}
	// Bind the cursor to its filters. The page size can change without changing
	// the result set; date offsets are canonicalized before hashing.
	filters := struct {
		Target, Nature, Severity, Query string
		From, Before                    *time.Time
	}{options.TargetID, options.NatureKey, string(options.Severity), options.Query, utcTime(options.From), utcTime(options.Before)}
	data, _ := json.Marshal(filters)
	hash := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(hash[:])
	if options.Cursor == "" {
		return resolvedCursor{Version: 1, Filter: fingerprint, Snapshot: now.UTC()}, nil
	}
	if len(options.Cursor) > 2048 {
		return invalid("invalid incident cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(options.Cursor)
	if err != nil {
		return invalid("invalid incident cursor")
	}
	var cursor resolvedCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != 1 || cursor.Filter != fingerprint ||
		cursor.Snapshot.IsZero() || cursor.At.IsZero() || cursor.At.After(cursor.Snapshot) || !incidentUUID.MatchString(cursor.ID) {
		return invalid("invalid incident cursor or changed filters")
	}
	return cursor, nil
}

func utcTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func (service *Service) ListResolvedPage(ctx context.Context, options ResolvedPageOptions) (ResolvedPage, error) {
	options.Query = strings.TrimSpace(options.Query)
	options.NatureKey = strings.TrimSpace(options.NatureKey)
	if _, err := options.cursor(time.Now()); err != nil {
		return ResolvedPage{}, err
	}
	return service.store.ListResolvedPage(ctx, options)
}

func (store *PostgresStore) ListResolvedPage(ctx context.Context, options ResolvedPageOptions) (ResolvedPage, error) {
	cursor, err := options.cursor(time.Now())
	if err != nil {
		return ResolvedPage{}, err
	}
	var afterAt *time.Time
	if options.Cursor != "" {
		afterAt = &cursor.At
	}
	presentations, err := store.matchingResolvedPresentations(ctx, options.Query, cursor.Snapshot)
	if err != nil {
		return ResolvedPage{}, err
	}
	// EXISTS keeps the page unit an Incident, including when several matching
	// resources or evidence belong to it. Load all of its impacts afterwards.
	rows, err := store.pool.Query(ctx, incidentProjectionSQL+`
		WHERE incident.status = 'resolved'
		  AND incident.resolved_at <= $1 AND incident.created_at <= $1
		  AND ($2::timestamptz IS NULL OR incident.resolved_at >= $2)
		  AND ($3::timestamptz IS NULL OR incident.resolved_at < $3)
		  AND ($4 = '' OR EXISTS (
		      SELECT 1 FROM cairnops_incident_impacts impact
		      WHERE impact.incident_id = incident.id AND impact.target_id = NULLIF($4, '')::uuid
		  ))
		  AND ($5 = '' OR incident.nature_key = $5)
		  AND ($6 = '' OR incident.severity = $6)
		  AND ($7 = '' OR strpos(lower(incident.nature_label), lower($7)) > 0
		      OR strpos(lower(incident.nature_key), lower($7)) > 0
		      OR EXISTS (
		          SELECT 1 FROM jsonb_to_recordset($11::jsonb) AS title(scope text, key text, kind text)
		          WHERE title.scope = incident.nature_scope AND title.key = incident.nature_key AND title.kind = incident.alert_kind
		      )
		      OR EXISTS (
		          SELECT 1 FROM cairnops_incident_impacts impact
		          JOIN cairnops_targets target ON target.id = impact.target_id
		          WHERE impact.incident_id = incident.id AND strpos(lower(target.name), lower($7)) > 0
		      ) OR EXISTS (
		          SELECT 1 FROM cairnops_incident_evidence evidence
		          WHERE evidence.incident_id = incident.id AND strpos(lower(evidence.name), lower($7)) > 0
		      ))
		  AND ($8::timestamptz IS NULL OR (incident.resolved_at, incident.id) < ($8, NULLIF($9, '')::uuid))
		ORDER BY incident.resolved_at DESC, incident.id DESC
		LIMIT $10
	`, cursor.Snapshot, options.From, options.Before, options.TargetID, options.NatureKey,
		options.Severity, options.Query, afterAt, cursor.ID, options.Limit+1, presentations)
	if err != nil {
		return ResolvedPage{}, fmt.Errorf("list resolved incident page: %w", err)
	}
	items := make([]Incident, 0, options.Limit+1)
	for rows.Next() {
		item, scanErr := scanIncident(rows)
		if scanErr != nil {
			rows.Close()
			return ResolvedPage{}, fmt.Errorf("scan resolved incident: %w", scanErr)
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ResolvedPage{}, fmt.Errorf("iterate resolved incidents: %w", err)
	}
	result := ResolvedPage{}
	if len(items) > options.Limit {
		items = items[:options.Limit]
		last := items[len(items)-1]
		cursor.At, cursor.ID = *last.ResolvedAt, last.ID
		encoded, err := json.Marshal(cursor)
		if err != nil {
			return ResolvedPage{}, fmt.Errorf("encode incident cursor: %w", err)
		}
		result.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	if err := store.loadChildren(ctx, items); err != nil {
		return ResolvedPage{}, err
	}
	result.Incidents = items
	if options.Cursor == "" {
		filters, err := store.resolvedFilterOptions(ctx, cursor.Snapshot)
		if err != nil {
			return ResolvedPage{}, err
		}
		result.Filters = &filters
	}
	return result, nil
}

// Presentation is computed from the canonical nature and/or verified alert
// kind. Reuse that exact catalogue rather than embedding translated strings
// in SQL, and find matching identities before applying the page limit.
func (store *PostgresStore) matchingResolvedPresentations(ctx context.Context, query string, snapshot time.Time) (string, error) {
	if query == "" {
		return "[]", nil
	}
	type identity struct {
		Scope string         `json:"scope"`
		Key   string         `json:"key"`
		Kind  alerttext.Kind `json:"kind"`
	}
	rows, err := store.pool.Query(ctx, `
		SELECT DISTINCT nature_scope, nature_key, alert_kind FROM cairnops_incidents
		WHERE status = 'resolved' AND resolved_at <= $1 AND created_at <= $1
	`, snapshot)
	if err != nil {
		return "", fmt.Errorf("list resolved presentation identities: %w", err)
	}
	defer rows.Close()
	matches := make([]identity, 0)
	query = strings.ToLower(query)
	for rows.Next() {
		var item identity
		if err := rows.Scan(&item.Scope, &item.Key, &item.Kind); err != nil {
			return "", fmt.Errorf("scan resolved presentation identity: %w", err)
		}
		presentation := synthesis.Presentation(synthesis.Situation{NatureScope: item.Scope, NatureKey: item.Key, AlertKind: item.Kind})
		if strings.Contains(strings.ToLower(presentation.FR.Title), query) || strings.Contains(strings.ToLower(presentation.EN.Title), query) {
			matches = append(matches, item)
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate resolved presentation identities: %w", err)
	}
	encoded, err := json.Marshal(matches)
	if err != nil {
		return "", fmt.Errorf("encode resolved presentation identities: %w", err)
	}
	return string(encoded), nil
}

// Options span the retained resolved history, including archived resources,
// rather than only the first page or current date range.
func (store *PostgresStore) resolvedFilterOptions(ctx context.Context, snapshot time.Time) (IncidentFilterOptions, error) {
	filters := IncidentFilterOptions{Targets: []IncidentFilterOption{}, Natures: []IncidentFilterOption{}}
	rows, err := store.pool.Query(ctx, `
		SELECT DISTINCT 'target', target.id::text, target.name
		FROM cairnops_incidents incident
		JOIN cairnops_incident_impacts impact ON impact.incident_id = incident.id
		JOIN cairnops_targets target ON target.id = impact.target_id
		WHERE incident.status = 'resolved' AND incident.resolved_at <= $1 AND incident.created_at <= $1
		UNION ALL
		SELECT 'nature', incident.nature_key, min(incident.nature_label)
		FROM cairnops_incidents incident
		WHERE incident.status = 'resolved' AND incident.resolved_at <= $1 AND incident.created_at <= $1
		GROUP BY incident.nature_key
		ORDER BY 1, 3, 2
	`, snapshot)
	if err != nil {
		return filters, fmt.Errorf("list incident filter options: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var option IncidentFilterOption
		if err := rows.Scan(&kind, &option.Value, &option.Label); err != nil {
			return filters, fmt.Errorf("scan incident filter option: %w", err)
		}
		if kind == "target" {
			filters.Targets = append(filters.Targets, option)
		} else {
			filters.Natures = append(filters.Natures, option)
		}
	}
	return filters, rows.Err()
}
