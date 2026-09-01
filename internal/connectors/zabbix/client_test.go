package zabbix

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestInspectUsesVersionProbeAndBearerDiscovery(t *testing.T) {
	t.Parallel()
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		var body struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
		switch body.Method {
		case "apiinfo.version":
			if r.Header.Get("Authorization") != "" {
				t.Fatal("version probe must not send the token")
			}
			response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":"7.4.2","id":1}`))
		case "host.get":
			if r.Header.Get("Authorization") != "Bearer token-one" {
				t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
			}
			response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[{"hostid":"10084","host":"zbx-server","name":"Zabbix Server","interfaces":[{"type":"1","useip":"1","ip":"10.0.0.5","dns":"","port":"10050","main":"1"}]}],"id":1}`))
		case "problem.get":
			if r.Header.Get("Authorization") != "Bearer token-one" {
				t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
			}
			response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[],"id":1}`))
		case "trigger.get":
			response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[],"id":1}`))
		default:
			t.Fatalf("unexpected method %q", body.Method)
		}
		return response, nil
	})}

	inspection, err := NewClientWithHTTP(client).Inspect(context.Background(), "https://zabbix.example.net/zabbix", "token-one")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 4 || inspection.Version != "7.4.2" || len(inspection.Hosts) != 1 {
		t.Fatalf("unexpected inspection: requests=%d %#v", requests, inspection)
	}
	if inspection.Hosts[0].Interfaces[0].Address != "10.0.0.5" || !inspection.Hosts[0].Interfaces[0].Main {
		t.Fatalf("unexpected interface projection: %#v", inspection.Hosts[0].Interfaces)
	}
}

func TestProblemsResolveHostsAndAcknowledgeEvent(t *testing.T) {
	t.Parallel()
	acknowledged := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
		switch body.Method {
		case "problem.get":
			response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[{"eventid":"20427","objectid":"15112","clock":"1786700000","name":"Database unavailable","acknowledged":"0","severity":"4","suppressed":"0"}],"id":1}`))
		case "trigger.get":
			response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[{"triggerid":"15112","hosts":[{"hostid":"10084"},{"hostid":"99999"}]}],"id":1}`))
		case "event.acknowledge":
			if body.Params["action"] != float64(6) {
				t.Fatalf("unexpected acknowledgement action: %#v", body.Params)
			}
			acknowledged = true
			response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":{"eventids":["20427"]},"id":1}`))
		default:
			t.Fatalf("unexpected method %q", body.Method)
		}
		return response, nil
	})}
	zabbixClient := NewClientWithHTTP(client)
	problems, err := zabbixClient.Problems(context.Background(), "https://zabbix.example.net", "token-one", []string{"10084"})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || problems[0].EventID != "20427" || len(problems[0].HostIDs) != 1 || problems[0].HostIDs[0] != "10084" {
		t.Fatalf("unexpected problems: %#v", problems)
	}
	if err := zabbixClient.Acknowledge(context.Background(), "https://zabbix.example.net", "token-one", "20427", "Acquitté depuis CairnOps"); err != nil {
		t.Fatal(err)
	}
	if !acknowledged {
		t.Fatal("event acknowledgement was not sent")
	}
}

func TestProblemsUseTemplateRootUUIDAsTargetIndependentNature(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
		switch body.Method {
		case "problem.get":
			response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[
				{"eventid":"1","objectid":"101","clock":"1786700000","name":"Disk latency","acknowledged":"0","severity":"3","suppressed":"0"},
				{"eventid":"2","objectid":"102","clock":"1786700001","name":"Disk latency","acknowledged":"0","severity":"3","suppressed":"0"}
			],"id":1}`))
		case "trigger.get":
			ids, _ := body.Params["triggerids"].([]any)
			ancestor := len(ids) == 1 && ids[0] == "900"
			if ancestor {
				response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[{"triggerid":"900","templateid":"0","uuid":"67cbf118-c331-4ecf-9137-c4b02c2c4190","description":"Disk latency","expression":"last(/template/vfs.dev.read.await)>20","tags":[{"tag":"cairnops.nature","value":"availability"}]}],"id":1}`))
			} else {
				response.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[
					{"triggerid":"101","templateid":"900","hosts":[{"hostid":"11"}]},
					{"triggerid":"102","templateid":"900","hosts":[{"hostid":"12"}]}
				],"id":1}`))
			}
		default:
			t.Fatalf("unexpected method %q", body.Method)
		}
		return response, nil
	})}
	problems, err := NewClientWithHTTP(client).Problems(
		context.Background(), "https://zabbix.example.net", "token", []string{"11", "12"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 2 || problems[0].NatureFingerprint == "" ||
		problems[0].NatureFingerprint != problems[1].NatureFingerprint ||
		!strings.HasPrefix(problems[0].NatureFingerprint, "uuid:") ||
		problems[0].CanonicalNature != "availability" ||
		problems[1].CanonicalNature != "availability" {
		t.Fatalf("template siblings must share their root Nature: %#v", problems)
	}
}

func TestDirectTriggerFingerprintExcludesRenderedHostIdentity(t *testing.T) {
	t.Parallel()
	left := remoteTrigger{
		TriggerID: "101", Description: "Disk latency",
		Expression: "last(/vm-one/vfs.dev.read.await[sda])>20",
	}
	right := remoteTrigger{
		TriggerID: "102", Description: "Disk latency",
		Expression: "last(/vm-two/vfs.dev.read.await[sda])>20",
	}
	leftFingerprint := triggerFingerprint("101", map[string]remoteTrigger{"101": left}, true)
	rightFingerprint := triggerFingerprint("102", map[string]remoteTrigger{"102": right}, true)
	if leftFingerprint != rightFingerprint || !strings.HasPrefix(leftFingerprint, "rule:") {
		t.Fatalf("host identity leaked into direct trigger fingerprint: %q != %q", leftFingerprint, rightFingerprint)
	}
}

func TestDirectTriggerFingerprintKeepsRulePathLiteralsDistinct(t *testing.T) {
	t.Parallel()
	left := remoteTrigger{
		TriggerID: "101", Description: "API log failure",
		Expression: `find(/vm-one/log[/var/log/app.log],,"regexp","/api/v1/")=1`,
	}
	right := remoteTrigger{
		TriggerID: "102", Description: "API log failure",
		Expression: `find(/vm-two/log[/var/log/app.log],,"regexp","/admin/v1/")=1`,
	}
	leftFingerprint := triggerFingerprint("101", map[string]remoteTrigger{"101": left}, true)
	rightFingerprint := triggerFingerprint("102", map[string]remoteTrigger{"102": right}, true)
	if leftFingerprint == rightFingerprint {
		t.Fatalf("rule path literals were erased from the Nature fingerprint: %q", leftFingerprint)
	}
}

func TestCanonicalNatureRequiresTheReservedCairnOpsTag(t *testing.T) {
	t.Parallel()
	generic := remoteTrigger{TriggerID: "101"}
	generic.Tags = append(generic.Tags, struct {
		Tag   string `json:"tag"`
		Value string `json:"value"`
	}{Tag: "scope", Value: "availability"})
	if got := triggerCanonicalNature("101", map[string]remoteTrigger{"101": generic}, true); got != "" {
		t.Fatalf("a generic Zabbix tag must not declare a CairnOps Nature, got %q", got)
	}
	generic.Tags[0].Tag = "cairnops.nature"
	if got := triggerCanonicalNature("101", map[string]remoteTrigger{"101": generic}, true); got != "availability" {
		t.Fatalf("reserved tag did not declare canonical availability, got %q", got)
	}
}

func TestNormalizeEndpointRejectsCredentialsAndBuildsAPIPath(t *testing.T) {
	t.Parallel()
	endpoint, err := NormalizeEndpoint("https://zabbix.example.net/zabbix/")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://zabbix.example.net/zabbix/api_jsonrpc.php" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
	if _, err := NormalizeEndpoint("https://admin:secret@zabbix.example.net"); err == nil {
		t.Fatal("expected credentials in URL to be rejected")
	}
}

func TestInspectNeverReturnsRemoteBodyContainingToken(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("secret-token")),
		}, nil
	})}

	_, err := NewClientWithHTTP(client).Inspect(context.Background(), "https://zabbix.example.net", "secret-token")
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("expected a redacted remote error, got %v", err)
	}
}
