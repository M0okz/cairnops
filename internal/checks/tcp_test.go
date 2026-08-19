package checks

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

func TestTCPCheckExchangesPayload(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		buffer := make([]byte, 4)
		_, _ = connection.Read(buffer)
		_, _ = connection.Write([]byte("PONG cairnops"))
	}()

	config, _ := json.Marshal(TCPConfig{Address: listener.Addr().String(), Send: "PING", ExpectContains: "PONG"})
	result := (TCP{}).Check(context.Background(), testSource(domain.SourceTCP, config))
	if result.Outcome != domain.OutcomeHealthy {
		t.Fatalf("expected healthy observation, got %#v", result)
	}
}

func TestTCPCheckReturnsWhenExpectedContentArrives(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	release := make(chan struct{})
	defer close(release)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_, _ = connection.Write([]byte("READY"))
		<-release
	}()

	config, _ := json.Marshal(TCPConfig{Address: listener.Addr().String(), ExpectContains: "READY"})
	startedAt := time.Now()
	result := (TCP{}).Check(context.Background(), testSource(domain.SourceTCP, config))
	if result.Outcome != domain.OutcomeHealthy {
		t.Fatalf("expected healthy observation, got %#v", result)
	}
	if time.Since(startedAt) > 500*time.Millisecond {
		t.Fatal("check waited for the peer to close after receiving expected content")
	}
}
