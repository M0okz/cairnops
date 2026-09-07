package zabbix

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/alerttext"
)

func TestOfficialAlertPresentationsAcrossVersions(t *testing.T) {
	versions := map[string]int{}
	for _, fixture := range standardAlerts {
		fixture := fixture
		t.Run(fixture.Version+"/"+fixture.Trigger.UUID, func(t *testing.T) {
			root := fixture.Trigger
			root.TriggerID = "100"
			known := map[string]remoteTrigger{"100": root}
			got := triggerPresentation("100", known, true)
			if got.Kind != fixture.Kind {
				t.Fatalf("official template was not recognized: got %q want %q", got.Kind, fixture.Kind)
			}
			versions[fixture.Version]++
			// Wording and locale are irrelevant when the structured condition is known.
			root.Description = "Un titre personnalisé dans une autre langue"
			known["100"] = root
			if triggerPresentation("100", known, true).Kind != fixture.Kind {
				t.Fatal("classification depends on source wording")
			}
			// A copied UUID alone cannot legitimize a changed condition.
			root.Expression += " and 1=0"
			known["100"] = root
			if triggerPresentation("100", known, true).Kind != "" {
				t.Fatal("modified official template was translated")
			}
			root = fixture.Trigger
			root.UUID, root.TriggerID = "unknown-template", "100"
			known["100"] = root
			if triggerPresentation("100", known, true).Kind != "" {
				t.Fatal("unknown template promoted by matching words/condition")
			}
		})
	}
	for version, count := range map[string]int{"6.0": 22, "7.0": 24, "7.4": 24} {
		if versions[version] != count {
			t.Errorf("documented coverage changed for %s: got %d, want %d", version, versions[version], count)
		}
	}
}

func TestPresentationRequiresCompleteUnmodifiedInheritance(t *testing.T) {
	root := remoteTrigger{TriggerID: "100", UUID: "b4e904559b694df0ad45bcce7930c3a6", Expression: `min(/Linux by Zabbix agent/system.cpu.util,5m)>{$CPU.UTIL.CRIT}`}
	child := remoteTrigger{TriggerID: "200", TemplateID: "100", Expression: `min(/unrelated-host/system.cpu.util,5m)>{$CPU.UTIL.CRIT}`}
	known := map[string]remoteTrigger{"100": root, "200": child}
	if got := triggerPresentation("200", known, true); got.Kind != alerttext.CPUUsage {
		t.Fatalf("same structured condition on another installation: %+v", got)
	}
	if triggerPresentation("200", known, false).Kind != "" {
		t.Fatal("partial API response was interpreted")
	}
	for _, expression := range []string{
		`min(/unrelated-host/system.cpu.util,10m)>{$CPU.UTIL.CRIT}`,
		`min(/unrelated-host/system.cpu.util,5m)<{$CPU.UTIL.CRIT}`,
		`min(/unrelated-host/system.cpu.load,5m)>{$CPU.UTIL.CRIT}`,
	} {
		child.Expression = expression
		known["200"] = child
		if triggerPresentation("200", known, true).Kind != "" {
			t.Fatalf("modified child condition: %s", expression)
		}
	}
	child.Expression = root.Expression
	child.RecoveryExpression = "1=1"
	known["200"] = child
	if triggerPresentation("200", known, true).Kind != "" {
		t.Fatal("custom recovery silently ignored")
	}
	child.RecoveryExpression = ""
	child.TemplateID = "300"
	known["200"] = child
	if triggerPresentation("200", known, true).Kind != "" {
		t.Fatal("missing ancestor accepted")
	}
	child.TemplateID = "200"
	known["200"] = child
	if triggerPresentation("200", known, true).Kind != "" {
		t.Fatal("ancestry cycle accepted")
	}
}

func TestDiscoveredDiskPresentationUsesItemFunctionsNotTheMessage(t *testing.T) {
	var root, child remoteTrigger
	rootJSON := `{"triggerid":"100","uuid":"eb6230f786d04b658ce62c30a9309a34","expression":"{1}>{$VFS.DEV.READ.AWAIT.WARN:\"{#DEVNAME}\"} or {2}>{$VFS.DEV.WRITE.AWAIT.WARN:\"{#DEVNAME}\"}","items":[{"itemid":"11","hostid":"1","key_":"vfs.dev.read.await[{#DEVNAME}]"},{"itemid":"12","hostid":"1","key_":"vfs.dev.write.await[{#DEVNAME}]"}],"functions":[{"functionid":"1","itemid":"11","function":"min","parameter":"$,15m"},{"functionid":"2","itemid":"12","function":"min","parameter":"$,15m"}]}`
	if err := json.Unmarshal([]byte(rootJSON), &root); err != nil {
		t.Fatal(err)
	}
	childJSON := strings.ReplaceAll(rootJSON, "{#DEVNAME}", "nvme0n1")
	if err := json.Unmarshal([]byte(childJSON), &child); err != nil {
		t.Fatal(err)
	}
	child.TriggerID, child.UUID = "200", ""
	child.DiscoveryData = json.RawMessage(`{"parent_triggerid":"100"}`)
	known := map[string]remoteTrigger{"100": root, "200": child}
	got := triggerPresentation("200", known, true)
	if got.Kind != alerttext.DiskLatency || got.Resource != "nvme0n1" {
		t.Fatalf("disk parameters lost: %+v", got)
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*remoteTrigger)
	}{
		{"cross-host-items", func(node *remoteTrigger) { node.Items[1].HostID = "2" }},
		{"missing-item-host", func(node *remoteTrigger) { node.Items[0].HostID = "" }},
		{"ambiguous-function-id", func(node *remoteTrigger) {
			duplicate := node.Functions[0]
			duplicate.Function = "avg"
			node.Functions = append(node.Functions, duplicate)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, _ := json.Marshal(child)
			var modified remoteTrigger
			if err := json.Unmarshal(encoded, &modified); err != nil {
				t.Fatal(err)
			}
			testCase.mutate(&modified)
			if triggerPresentation("200", map[string]remoteTrigger{"100": root, "200": modified}, true).Kind != "" {
				t.Fatal("ambiguous item/function data accepted")
			}
		})
	}
	child.Functions[0].Function = "max"
	known["200"] = child
	if triggerPresentation("200", known, true).Kind != "" {
		t.Fatal("different aggregation accepted")
	}
	child.Functions = nil
	known["200"] = child
	if triggerPresentation("200", known, true).Kind != "" {
		t.Fatal("unresolved function identities accepted")
	}
}

func TestPresentationRejectsMultipleHosts(t *testing.T) {
	for _, rule := range standardAlerts {
		if rule.Kind != alerttext.SystemLoad {
			continue
		}
		root := rule.Trigger
		root.TriggerID = "100"
		// A CPU count from another host changes the meaning of load per CPU.
		index := strings.LastIndex(root.Expression, "/Linux by Zabbix agent/")
		if index < 0 {
			continue
		}
		root.Expression = root.Expression[:index] + strings.Replace(root.Expression[index:], "/Linux by Zabbix agent/", "/another-host/", 1)
		if triggerPresentation("100", map[string]remoteTrigger{"100": root}, true).Kind != "" {
			t.Fatal("cross-host expression normalized into a single-host rule")
		}
		return
	}
	t.Fatal("missing passive Linux load fixture")
}

func TestDiscoveredCertificatePresentation(t *testing.T) {
	for _, fixture := range standardAlerts {
		if fixture.Version != "7.4" || fixture.Kind != alerttext.CertificateExpiry {
			continue
		}
		root := fixture.Trigger
		root.TriggerID = "100"
		child := root
		child.TriggerID, child.UUID = "200", ""
		child.DiscoveryData = json.RawMessage(`{"parent_triggerid":"100"}`)
		child.Expression = strings.ReplaceAll(child.Expression, "{#CERT.WEBSITE.ITEMNAME}", "service.example:8443")
		got := triggerPresentation("200", map[string]remoteTrigger{"100": root, "200": child}, true)
		if got.Kind != alerttext.CertificateExpiry || got.Resource != "service.example:8443" {
			t.Fatalf("certificate discovery lost: %+v", got)
		}
		return
	}
	t.Fatal("missing 7.4 certificate fixture")
}

func TestProblemsCarryPresentationWithoutChangingNatureOrSeverity(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		result := "[]"
		switch body.Method {
		case "problem.get":
			result = `[{"eventid":"1","objectid":"200","clock":"1786700000","name":"Operator's own wording","acknowledged":"0","severity":"3","suppressed":"0"}]`
		case "trigger.get":
			ids, _ := body.Params["triggerids"].([]any)
			if len(ids) == 1 && ids[0] == "100" {
				result = `[{"triggerid":"100","templateid":"0","uuid":"b4e904559b694df0ad45bcce7930c3a6","expression":"{7}>{$CPU.UTIL.CRIT}","items":[{"itemid":"11","hostid":"1","key_":"system.cpu.util"}],"functions":[{"functionid":"7","itemid":"11","function":"min","parameter":"$,5m"}]}]`
			} else {
				result = `[{"triggerid":"200","templateid":"100","hosts":[{"hostid":"42"}],"expression":"{8}>{$CPU.UTIL.CRIT}","items":[{"itemid":"12","hostid":"42","key_":"system.cpu.util"}],"functions":[{"functionid":"8","itemid":"12","function":"min","parameter":"$,5m"}]}]`
			}
		default:
			t.Fatalf("unexpected call %s", body.Method)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))}, nil
	})}
	problems, err := NewClientWithHTTP(client).Problems(context.Background(), "https://zabbix.example.test", "test-token", []string{"42"})
	if err != nil || len(problems) != 1 {
		t.Fatalf("problems: %+v %v", problems, err)
	}
	got := problems[0]
	if got.Severity != 3 || got.Alert.Kind != alerttext.CPUUsage || got.Name != "Operator's own wording" || got.CanonicalNature != "" || got.NatureFingerprint != "uuid:b4e904559b694df0ad45bcce7930c3a6" {
		t.Fatalf("presentation changed the source or Nature: %+v", got)
	}
}
