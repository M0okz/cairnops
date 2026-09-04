package migrations_test

import (
	"context"
	"testing"

	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestIncidentEvidenceCutoverInstallsOnlyTheNewRuntimeTables(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var incidents, impacts, evidence bool
	var oldSignals, oldBursts bool
	if err := pool.QueryRow(ctx, `
		SELECT to_regclass('cairnops_incidents') IS NOT NULL,
		       to_regclass('cairnops_incident_impacts') IS NOT NULL,
		       to_regclass('cairnops_incident_evidence') IS NOT NULL,
		       to_regclass('cairnops_incident_signals') IS NOT NULL,
		       to_regclass('cairnops_incident_bursts') IS NOT NULL
	`).Scan(&incidents, &impacts, &evidence, &oldSignals, &oldBursts); err != nil {
		t.Fatal(err)
	}
	if !incidents || !impacts || !evidence || oldSignals || oldBursts {
		t.Fatalf("unexpected incident cutover schema: incidents=%v impacts=%v evidence=%v old_signals=%v old_bursts=%v",
			incidents, impacts, evidence, oldSignals, oldBursts)
	}
}
