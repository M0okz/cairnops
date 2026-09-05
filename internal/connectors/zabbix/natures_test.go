package zabbix

import (
	"encoding/json"
	"strings"
	"testing"
)

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

func TestStorageLatencyAcceptsAPIIdentityExpressions(t *testing.T) {
	var trigger remoteTrigger
	if err := json.Unmarshal([]byte(`{
		"uuid":"eb6230f786d04b658ce62c30a9309a34",
		"expression":"{1}>{$VFS.DEV.READ.AWAIT.WARN:\"{#DEVNAME}\"} or {2}>{$VFS.DEV.WRITE.AWAIT.WARN:\"{#DEVNAME}\"}",
		"items":[{"itemid":"11","key_":"vfs.dev.read.await[{#DEVNAME}]"},{"itemid":"12","key_":"vfs.dev.write.await[{#DEVNAME}]"}],
		"functions":[{"functionid":"1","itemid":"11","function":"min","parameter":"15m"},{"functionid":"2","itemid":"12","function":"min","parameter":"15m"}]
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
