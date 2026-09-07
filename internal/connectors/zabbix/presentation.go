package zabbix

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/M0okz/cairnops/internal/alerttext"
)

// These are reviewed extracts of pinned official templates, not user messages.
// Descriptions are retained for provenance/tests, but never used to recognize
// a runtime alert. UUID AND condition AND the complete inheritance chain must fit.
//
//go:embed standard_alerts.json
var standardAlertsJSON []byte

type standardAlert struct {
	Version string         `json:"version"`
	Source  string         `json:"source"`
	Kind    alerttext.Kind `json:"kind"`
	Trigger remoteTrigger  `json:"trigger"`
}

var standardAlerts = func() []standardAlert {
	var result []standardAlert
	if err := json.Unmarshal(standardAlertsJSON, &result); err != nil {
		panic(err)
	}
	return result
}()

var presentationHistoryHost = regexp.MustCompile(`([a-z][a-z0-9_]*\()/([^/]+)/`)
var presentationFunctionName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var presentationDevice = regexp.MustCompile(`vfs\.dev\.(?:read|write)\.await\[([^\],]+)\]`)
var presentationCertificate = regexp.MustCompile(`cert\.(?:not_after|validation)\[([^\]]+)\]`)
var presentationFilesystem = regexp.MustCompile(`vfs\.fs\.(?:dependent\.)?(?:size|inode)\[([^\],]+),(?:pused|pfree)\]`)

func triggerPresentation(triggerID string, known map[string]remoteTrigger, detailed bool) alerttext.Fact {
	if !detailed {
		return alerttext.Fact{}
	}
	chain := []remoteTrigger{}
	visited := map[string]bool{}
	for len(chain) < 32 {
		node, ok := known[triggerID]
		if !ok || visited[triggerID] {
			return alerttext.Fact{}
		}
		visited[triggerID] = true
		chain = append(chain, node)
		parent := discoveryParent(node.DiscoveryData)
		if parent == "" || parent == "0" {
			parent = strings.TrimSpace(node.TemplateID)
		}
		if parent == "" || parent == "0" {
			break
		}
		triggerID = parent
	}
	root := chain[len(chain)-1]
	if root.TemplateID != "" && root.TemplateID != "0" {
		return alerttext.Fact{}
	}
	if parent := discoveryParent(root.DiscoveryData); parent != "" && parent != "0" {
		return alerttext.Fact{}
	}
	for _, rule := range standardAlerts {
		if !strings.EqualFold(root.UUID, rule.Trigger.UUID) {
			continue
		}
		matches := true
		for _, node := range chain {
			if !matchesPresentationRule(node, rule.Trigger) {
				matches = false
				break
			}
		}
		if matches {
			resource, _ := presentationResource(chain[0], rule.Trigger.Expression)
			if strings.Contains(resource, "{") {
				resource = ""
			}
			return (alerttext.Fact{Kind: rule.Kind, Resource: resource}).Normalize()
		}
	}
	return alerttext.Fact{}
}

func matchesPresentationRule(node, rule remoteTrigger) bool {
	hosts := map[string]bool{}
	for _, host := range node.Hosts {
		hosts[host.HostID] = true
	}
	if node.CorrelationTag != "" || len(hosts) > 1 {
		return false
	}
	expected, recovery := rule.Expression, rule.RecoveryExpression
	resource, macro := presentationResource(node, expected)
	if macro != "" && resource != "" {
		expected = strings.ReplaceAll(expected, macro, resource)
		recovery = strings.ReplaceAll(recovery, macro, resource)
	}
	actual, ok := presentationExpression(node, node.Expression)
	if !ok || actual == "" {
		return false
	}
	want, ok := presentationExpression(rule, expected)
	if !ok || actual != want {
		return false
	}
	actualRecovery, ok := presentationExpression(node, node.RecoveryExpression)
	wantRecovery, expectedOK := presentationExpression(rule, recovery)
	return ok && expectedOK && actualRecovery == wantRecovery
}

// Expand function IDs only from their documented item/function bindings.
// Arbitrary unresolved IDs, extra conditions and altered windows stay unknown.
func presentationExpression(trigger remoteTrigger, expression string) (string, bool) {
	hosts := map[string]bool{}
	for _, match := range presentationHistoryHost.FindAllStringSubmatch(expression, -1) {
		hosts[match[2]] = true
	}
	if len(hosts) > 1 {
		return "", false
	}
	functions := map[string]string{}
	for _, function := range trigger.Functions {
		binding := function.ItemID + "\x00" + function.Function + "\x00" + function.Parameter
		if previous, ok := functions[function.FunctionID]; ok && previous != binding {
			return "", false
		}
		functions[function.FunctionID] = binding
		identity := "{" + function.FunctionID + "}"
		if function.FunctionID == "" || !strings.Contains(expression, identity) {
			continue
		}
		if !presentationFunctionName.MatchString(function.Function) {
			return "", false
		}
		key := ""
		for _, item := range trigger.Items {
			if item.ItemID == function.ItemID {
				if item.HostID == "" {
					return "", false
				}
				hosts["id:"+item.HostID] = true
				if len(hosts) > 1 {
					return "", false
				}
				if key != "" && key != item.Key {
					return "", false
				}
				key = item.Key
			}
		}
		if key == "" {
			return "", false
		}
		parameter := strings.TrimPrefix(function.Parameter, "$,")
		if parameter == "$" {
			parameter = ""
		}
		call := function.Function + "(/*/" + key
		if parameter != "" {
			call += "," + parameter
		}
		expression = strings.ReplaceAll(expression, identity, call+")")
	}
	if functionIdentity.MatchString(expression) {
		return "", false
	}
	expression = presentationHistoryHost.ReplaceAllString(expression, `${1}/*/`)
	var normalized strings.Builder
	quoted, escaped := false, false
	for _, r := range expression {
		if r == '"' && !escaped {
			quoted = !quoted
		}
		if quoted || !unicode.IsSpace(r) {
			normalized.WriteRune(r)
		}
		escaped = r == '\\' && !escaped
	}
	return normalized.String(), !quoted
}

func presentationResource(node remoteTrigger, expected string) (string, string) {
	macro := ""
	var matcher *regexp.Regexp
	if strings.Contains(expected, "{#DEVNAME}") {
		macro, matcher = "{#DEVNAME}", presentationDevice
	}
	if strings.Contains(expected, "{#FSNAME}") {
		macro, matcher = "{#FSNAME}", presentationFilesystem
	}
	if strings.Contains(expected, "{#CERT.WEBSITE.ITEMNAME}") {
		macro, matcher = "{#CERT.WEBSITE.ITEMNAME}", presentationCertificate
	}
	if matcher == nil {
		return "", ""
	}
	keys := node.Expression
	for _, item := range node.Items {
		keys += "\n" + item.Key
	}
	resource := ""
	for _, match := range matcher.FindAllStringSubmatch(keys, -1) {
		if resource != "" && resource != match[1] {
			return "", macro
		}
		resource = match[1]
	}
	return resource, macro
}
