package notifications

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type fakeChannelStore struct{ input PersistMattermostInput }

func (*fakeChannelStore) List(context.Context) ([]Channel, error) { return nil, nil }

func (*fakeChannelStore) Inbox(context.Context, string, int) (Inbox, error) { return Inbox{}, nil }

func (*fakeChannelStore) MarkRead(context.Context, string, []int64) (int, error) { return 0, nil }
func (store *fakeChannelStore) CreateMattermost(_ context.Context, input PersistMattermostInput) (Channel, error) {
	store.input = input
	return Channel{Name: input.Name, Endpoint: input.Endpoint, Severities: input.Severities}, nil
}

type fakeMattermost struct {
	webhook string
	err     error
}

func (client *fakeMattermost) Test(_ context.Context, webhook string) error {
	client.webhook = webhook
	return client.err
}

func TestCreateMattermostTestsThenSealsWebhookAndAppliesRecommendedRouting(t *testing.T) {
	t.Parallel()
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeChannelStore{}
	client := &fakeMattermost{}
	service := NewService(store, client, box)
	channel, err := service.CreateMattermost(context.Background(), "actor", CreateMattermostInput{
		Name: "  Exploitation  ", WebhookURL: "https://mattermost.example.test/hooks/secret-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.webhook != "https://mattermost.example.test/hooks/secret-token" {
		t.Fatalf("connection was not tested with the complete webhook: %q", client.webhook)
	}
	if channel.Endpoint != "https://mattermost.example.test" || store.input.CredentialSealed == client.webhook {
		t.Fatalf("public endpoint must be redacted and credential sealed: %#v", store.input)
	}
	plaintext, err := box.Open(store.input.CredentialSealed, mattermostSecretPurpose)
	if err != nil || string(plaintext) != client.webhook {
		t.Fatalf("sealed webhook mismatch: %q, %v", plaintext, err)
	}
	want := []incidents.Severity{incidents.SeverityWarning, incidents.SeverityMajor, incidents.SeverityCritical}
	if !reflect.DeepEqual(channel.Severities, want) {
		t.Fatalf("unexpected recommended severities: %#v", channel.Severities)
	}
}

func TestCreateMattermostRejectsUnencryptedWebhook(t *testing.T) {
	t.Parallel()
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(&fakeChannelStore{}, &fakeMattermost{}, box)
	_, err = service.CreateMattermost(context.Background(), "actor", CreateMattermostInput{
		WebhookURL: "http://mattermost.example.test/hooks/token",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestCreateMattermostDoesNotPersistAFailedConnection(t *testing.T) {
	t.Parallel()
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeChannelStore{}
	service := NewService(store, &fakeMattermost{err: errors.New("forbidden")}, box)
	_, err = service.CreateMattermost(context.Background(), "actor", CreateMattermostInput{
		WebhookURL: "https://mattermost.example.test/hooks/token",
	})
	if !errors.Is(err, ErrConnection) || store.input.Name != "" {
		t.Fatalf("expected a connection failure before persistence, got %v, %#v", err, store.input)
	}
}
