package zabbix

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestEvaluationWindowRequiresAnExplicitCurrentPeriod(t *testing.T) {
	for _, test := range []struct {
		name, function, parameter string
		want                      time.Duration
	}{
		{"disk minimum", "min", "$,15m", 15 * time.Minute},
		{"average in seconds", "avg", "$,180", 3 * time.Minute},
		{"legacy period", "max", "2m", 2 * time.Minute},
		{"count filter", "count", "$,5m,gt,20", 5 * time.Minute},
		{"daily sum", "sum", "$,1d", 24 * time.Hour},
		{"macro", "min", "$,{$WINDOW}", 0},
		{"samples", "min", "$,#15", 0},
		{"time shift", "min", "$,15m:now-1d", 0},
		{"latest value", "last", "$,#1", 0},
		{"prediction horizon", "timeleft", "$,5m,,20", 0},
		{"missing function", "", "$,15m", 0},
		{"zero", "min", "$,0", 0},
		{"negative", "min", "$,-15m", 0},
		{"overflow", "min", "$,9223372036854775807w", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var trigger remoteTrigger
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{
				"expression":"{1}>20",
				"functions":[{"functionid":"1","function":%q,"parameter":%q},
				{"functionid":"2","function":"min","parameter":"$,1w"}]
			}`, test.function, test.parameter)), &trigger); err != nil {
				t.Fatal(err)
			}
			if got := triggerEvaluationWindow(trigger); got != test.want {
				t.Fatalf("period = %s, want %s (recovery-only function must be ignored)", got, test.want)
			}
		})
	}
}
