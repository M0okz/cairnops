package argus

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
)

func TestInspectDetectsProtocolWithoutSendingCredentials(t *testing.T) {
	for _, secure := range []bool{true, false} {
		t.Run(fmt.Sprint(secure), func(t *testing.T) {
			probes, authenticated := 0, 0
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "" {
					probes++
					if r.URL.Path != "/argus/api/v1/version" {
						t.Errorf("unexpected probe %s", r.URL.Path)
					}
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				authenticated++
				user, password, ok := r.BasicAuth()
				if !ok || user != "reader" || password != "secret" {
					t.Error("wrong credentials")
				}
				switch r.URL.Path {
				case "/argus/api/v1/version":
					fmt.Fprint(w, `{"version":"0.35.0"}`)
				case "/argus/api/v1/config":
					fmt.Fprint(w, `{"service":{}}`)
				case "/argus/api/v1/counts":
					fmt.Fprint(w, `{}`)
				case "/argus/metrics":
					fmt.Fprint(w, "# empty inventory\n")
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			})
			server := httptest.NewUnstartedServer(handler)
			if secure {
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			address := strings.SplitN(server.URL, "://", 2)[1] + "/argus/api/v1/config"
			inspection, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), "  "+address+"  ", Credentials{Username: "reader", Password: "secret"})
			if err != nil {
				t.Fatal(err)
			}
			if inspection.Endpoint != server.URL+"/argus" || inspection.EncryptedTransport != secure || probes != 1 || authenticated != 4 {
				t.Fatalf("inspection=%+v probes=%d authenticated=%d", inspection, probes, authenticated)
			}
		})
	}
}

type addressTransport func(*http.Request) (*http.Response, error)

func (f addressTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAddressDetectionDoesNotDowngradeTLSErrorsOrHTTPResponses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		status    int
		wantError bool
	}{
		{"certificate", x509.UnknownAuthorityError{}, 0, true},
		{"timeout", context.DeadlineExceeded, 0, true},
		{"unauthorized", nil, 401, false},
		{"unavailable", nil, 503, false},
		{"redirect", nil, 302, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := NewClientWithHTTP(&http.Client{Transport: addressTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Scheme != "https" || r.Header.Get("Authorization") != "" {
					t.Fatal("unsafe probe")
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: http.NoBody, Header: http.Header{"Location": []string{"http://elsewhere.example"}}, Request: r}, nil
			})})
			endpoint, err := client.resolveAddress(context.Background(), "argus.example")
			if (err != nil) != tc.wantError || calls != 1 {
				t.Fatalf("endpoint=%s err=%v calls=%d", endpoint, err, calls)
			}
		})
	}
}

func TestAddressDetectionFallsBackAfterConnectionRefused(t *testing.T) {
	calls := 0
	client := NewClientWithHTTP(&http.Client{Transport: addressTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "" {
			t.Fatal("credentials sent")
		}
		if r.URL.Scheme == "https" {
			return nil, fmt.Errorf("dial: %w", syscall.ECONNREFUSED)
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
	})})
	endpoint, err := client.resolveAddress(context.Background(), "argus.example/path")
	if err != nil || endpoint != "http://argus.example/path" || calls != 2 {
		t.Fatalf("%s %v %d", endpoint, err, calls)
	}
}

func TestAddressDetectionPreservesExplicitSchemesAndRejectsInvalidInput(t *testing.T) {
	calls := 0
	client := NewClientWithHTTP(&http.Client{Transport: addressTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected network call")
	})})
	for _, address := range []string{"http://argus.example/path", "https://argus.example/path"} {
		got, err := client.resolveAddress(context.Background(), address)
		if err != nil || got != address {
			t.Fatalf("%s: %s %v", address, got, err)
		}
	}
	for _, address := range []string{"", "ftp://example.net", "//example.net", "reader:secret@example.net", "example.net?token=secret", "example.net#fragment", "https://user:pass@example.net"} {
		if _, err := client.resolveAddress(context.Background(), address); err == nil {
			t.Errorf("accepted %q", address)
		}
	}
	if calls != 0 {
		t.Fatalf("unexpected requests: %d", calls)
	}
}
