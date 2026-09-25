package versions

import "testing"

// Cas relevés dans le suivi Argus réel : la différence des chaînes ne suffit
// pas à établir une mise à jour à appliquer.
func TestAssessDistinguishesActionableUpdates(t *testing.T) {
	for _, test := range []struct {
		installed, target string
		want              Situation
		level             Level
	}{
		{"12.4.9", "13.2.2", Update, Major},
		{"2.14.0", "2.16.3", Update, Minor},
		{"1.37.0", "1.37.3", Update, Patch},
		{"0.24.2315", "0.24.2668", Update, Patch},
		{"6.3.0.10514", "6.4.4.10685", Update, Minor},
		{"1.43.3.10828", "1.43.4.10903", Update, Patch},
		{"v1.2.3", "1.2.4", Update, Patch},
		{"0.16.2-beta", "0.24.0-beta", Update, Minor},
		{"1.0.0-beta.8", "1.0.1-preview.11", Update, Patch},
		{"2026.8.1", "2026.9.3", Update, Minor},
		{"11.6.0", "12.0.0-rc1", Prerelease, Major},
		{"4.14.5", "5.0.0-beta5", Prerelease, Major},
		{"0.27.4", "2026.9.23-canary.909", Prerelease, Major},
		{"0.1.147", "0.1.146", TargetOlder, ""},
		{"24.04", "0.80.0-canary.pr-7535.1", TargetOlder, ""},
		{"1.2.3", "v1.2.3", Current, ""},
		{"1.2.3", "1.2.3+build.5", Current, ""},
		{"2024.10.22", "2024.10.22-7ca5933", Current, ""},
		{"2022.12.14-r222.5d447b6035", "2022.12.14-r222.5d447b6035", Current, ""},
		{"latest", "stable", Unordered, ""},
		{"1.2.3", "nightly", Unordered, ""},
	} {
		got := Assess(test.installed, test.target)
		if got.Situation != test.want || got.Level != test.level {
			t.Errorf("Assess(%q, %q) = %+v, want %s/%s", test.installed, test.target, got, test.want, test.level)
		}
	}
}

func TestComparePrereleasePrecedence(t *testing.T) {
	ordered := []string{"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0"}
	for index := 1; index < len(ordered); index++ {
		if order, ok := Compare(ordered[index-1], ordered[index]); !ok || order >= 0 {
			t.Fatalf("%s must precede %s", ordered[index-1], ordered[index])
		}
	}
	if order, ok := Compare("1.2", "1.2.0"); !ok || order != 0 {
		t.Fatal("missing components are zero")
	}
}
