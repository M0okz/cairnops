package testsupport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// DiskLatencyAPI rejoue la forme de règle lue le 6 septembre 2026 : trigger
// découvert, prototype hérité puis prototype officiel. Les identités sont
// anonymisées. ZBX-23578 omet « function » quand ce champ est sélectionné
// explicitement ; « extend » et l'ancien nom « name » rendent bien sa valeur.
func DiskLatencyAPI(t testing.TB) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params struct {
				TriggerIDs []string        `json:"triggerids"`
				Functions  json.RawMessage `json:"selectFunctions"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid fixture request", http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "problem.get":
			result = []map[string]string{{"eventid": "9", "objectid": "1", "clock": "1788668709", "name": "Linux: sda: Disk read/write request responses are too high", "severity": "2"}}
		case "trigger.get", "triggerprototype.get":
			triggers := []map[string]any{}
			for _, id := range request.Params.TriggerIDs {
				if id != "1" && id != "2" && id != "3" {
					continue
				}
				device, parent, uuid := "{#DEVNAME}", "0", ""
				if id == "1" {
					device = "sda"
				} else if id == "2" {
					parent = "3"
				} else {
					uuid = "fd5732c3cf5249f9a05e3b6cedc2d2fd"
				}
				functions := []map[string]string{
					{"functionid": "11", "itemid": "21", "function": "min", "parameter": "$,15m"},
					{"functionid": "12", "itemid": "22", "function": "min", "parameter": "$,15m"},
				}
				var fields []string
				_ = json.Unmarshal(request.Params.Functions, &fields)
				if string(request.Params.Functions) != `"extend"` && !slices.Contains(fields, "name") {
					for _, function := range functions {
						delete(function, "function")
					}
				}
				trigger := map[string]any{
					"triggerid": id, "templateid": parent, "uuid": uuid,
					"expression": `{11} > {$VFS.DEV.READ.AWAIT.WARN:"` + device + `"} or {12} > {$VFS.DEV.WRITE.AWAIT.WARN:"` + device + `"}`,
					"items": []map[string]string{
						{"itemid": "21", "key_": "vfs.dev.read.await[" + device + "]"},
						{"itemid": "22", "key_": "vfs.dev.write.await[" + device + "]"},
					},
					"functions": functions,
				}
				if id == "1" {
					trigger["hosts"] = []map[string]string{{"hostid": "10"}}
					trigger["discoveryData"] = map[string]string{"parent_triggerid": "2"}
				}
				triggers = append(triggers, trigger)
			}
			result = triggers
		default:
			t.Errorf("unexpected fixture method: %s", request.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": 1})
	}))
	t.Cleanup(server.Close)
	return server
}
