package zabbix

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestItemsReadsOnlyFiniteNumericValuesAndKeepsExactIdentity(t *testing.T) {
	client := NewClientWithHTTP(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing Zabbix token")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":[{"itemid":"42","hostid":"7","name":"CPU utilization","key_":"system.cpu.util","units":"%","value_type":"0","lastvalue":"12.5","lastclock":"1787443200"},{"itemid":"43","hostid":"7","name":"Text","key_":"log","units":"","value_type":"4","lastvalue":"hello","lastclock":"1787443200"}],"id":1}`))}, nil
	})})
	items, err := client.Items(context.Background(), "https://zabbix.example.test/api_jsonrpc.php", "secret", []string{"7"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "42" || items[0].LastValue == nil || *items[0].LastValue != 12.5 || items[0].LastClock == nil || !items[0].LastClock.Equal(time.Unix(1787443200, 0)) {
		t.Fatalf("unexpected items: %#v", items)
	}
}
