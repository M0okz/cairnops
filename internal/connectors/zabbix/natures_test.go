package zabbix

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestDiscoveredDiskRuleWithZabbixFunctionProjectionBug(t *testing.T) {
	api := testsupport.DiskLatencyAPI(t)
	problems, err := NewClient().Problems(context.Background(), api.URL, "test-token", []string{"10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || problems[0].CanonicalNature != "storage.latency" {
		t.Fatalf("discovered official disk rule was not recognized: %#v", problems)
	}
	if problems[0].EvaluationWindow != 15*time.Minute {
		t.Fatalf("lost the active condition period: %s", problems[0].EvaluationWindow)
	}
}

func TestOfficialStorageLatencyRequiresItsUnmodifiedCondition(t *testing.T) {
	expression := `min(/Linux by Zabbix agent/vfs.dev.read.await[{#DEVNAME}],15m) > {$VFS.DEV.READ.AWAIT.WARN:"{#DEVNAME}"} or min(/Linux by Zabbix agent/vfs.dev.write.await[{#DEVNAME}],15m) > {$VFS.DEV.WRITE.AWAIT.WARN:"{#DEVNAME}"}`
	for _, uuid := range []string{"eb6230f786d04b658ce62c30a9309a34", "fd5732c3cf5249f9a05e3b6cedc2d2fd"} {
		root := remoteTrigger{TriggerID: "root", UUID: uuid, Expression: expression}
		known := map[string]remoteTrigger{"root": root, "child": {TriggerID: "child", TemplateID: "root", Description: "VM 42 / sda"}}
		if got := triggerCanonicalNature("child", known, true); got != "storage.latency" {
			t.Fatalf("official latency template unrecognized: %q", got)
		}
		root.Expression = strings.ReplaceAll(expression, ">", "<")
		known["root"] = root
		if got := triggerCanonicalNature("child", known, true); got != "" {
			t.Fatalf("modified condition misclassified: %q", got)
		}
	}
}

func TestRuntimeProblemsRequestFunctionIdentities(t *testing.T) {
	client := NewClientWithHTTP(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request struct {
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		result := `[{"eventid":"9","objectid":"2","clock":"1786700000","name":"Disk slow","severity":"3"}]`
		if request.Method == "trigger.get" {
			if string(request.Params["selectFunctions"]) != `"extend"` {
				t.Fatal("runtime did not request complete function identities")
			}
			result = `[{"triggerid":"2","uuid":"eb6230f786d04b658ce62c30a9309a34","hosts":[{"hostid":"1"}],
			"expression":"{1}>{$VFS.DEV.READ.AWAIT.WARN:\"{#DEVNAME}\"} or {2}>{$VFS.DEV.WRITE.AWAIT.WARN:\"{#DEVNAME}\"}",
			"items":[{"itemid":"11","key_":"vfs.dev.read.await[{#DEVNAME}]"},{"itemid":"12","key_":"vfs.dev.write.await[{#DEVNAME}]"}],
			"functions":[{"functionid":"1","itemid":"11","function":"min","parameter":"$,15m"},{"functionid":"2","itemid":"12","function":"min","parameter":"$,15m"}]}]`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":` + result + `,"id":1}`))}, nil
	})})
	problems, err := client.Problems(context.Background(), "https://zabbix.example.test", "test-token", []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || problems[0].CanonicalNature != "storage.latency" {
		t.Fatalf("runtime failed to normalize the official condition: %#v", problems)
	}
}

func TestCanonicalTagIsReadOnIntermediateTriggersAndRejectsConflicts(t *testing.T) {
	var child remoteTrigger
	if err := json.Unmarshal([]byte(`{"triggerid":"child","templateid":"root","tags":[{"tag":"cairnops.nature","value":"storage.latency"}]}`), &child); err != nil {
		t.Fatal(err)
	}
	root := remoteTrigger{TriggerID: "root"}
	known := map[string]remoteTrigger{"child": child, "root": root}
	if got := triggerCanonicalNature("child", known, true); got != "storage.latency" {
		t.Fatalf("child declaration ignored: %q", got)
	}
	if err := json.Unmarshal([]byte(`{"triggerid":"root","tags":[{"tag":"cairnops.nature","value":"availability"}]}`), &root); err != nil {
		t.Fatal(err)
	}
	known["root"] = root
	if got := triggerCanonicalNature("child", known, true); got != "" {
		t.Fatalf("conflicting meanings were accepted: %q", got)
	}
}

func TestStorageLatencyAcceptsAPIIdentityExpressions(t *testing.T) {
	var trigger remoteTrigger
	if err := json.Unmarshal([]byte(`{
		"uuid":"eb6230f786d04b658ce62c30a9309a34",
		"expression":"{1}>{$VFS.DEV.READ.AWAIT.WARN:\"{#DEVNAME}\"} or {2}>{$VFS.DEV.WRITE.AWAIT.WARN:\"{#DEVNAME}\"}",
		"items":[{"itemid":"11","key_":"vfs.dev.read.await[{#DEVNAME}]"},{"itemid":"12","key_":"vfs.dev.write.await[{#DEVNAME}]"}],
		"functions":[{"functionid":"1","itemid":"11","function":"min","parameter":"$,15m"},{"functionid":"2","itemid":"12","function":"min","parameter":"$,15m"}]
	}`), &trigger); err != nil {
		t.Fatal(err)
	}
	if got := standardTriggerNature(trigger); got != "storage.latency" {
		t.Fatalf("identity expression not recognized: %q", got)
	}
	trigger.Functions[0].Function = "max"
	if got := standardTriggerNature(trigger); got != "" {
		t.Fatalf("unverified rule accepted: %q", got)
	}
}
