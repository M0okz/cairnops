package identity

import "testing"

func TestValidateIdentityInput(t *testing.T) {
	for name, input := range map[string]InitializeInput{
		"valid":        {Username: "gregory", DisplayName: "Gregory", Password: "a-long-password"},
		"short user":   {Username: "gn", DisplayName: "Gregory", Password: "a-long-password"},
		"display name": {Username: "gregory", DisplayName: "", Password: "a-long-password"},
		"password":     {Username: "gregory", DisplayName: "Gregory", Password: "short"},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateIdentityInput(input.Username, input.DisplayName, input.Password)
			if name == "valid" && err != nil {
				t.Fatalf("expected valid input, got %v", err)
			}
			if name != "valid" && err == nil {
				t.Fatal("expected invalid input")
			}
		})
	}
}

// Une instance se nomme à la mise en service : sans nom, deux instances
// ouvertes côte à côte ne se distinguent nulle part dans l'écran.
func TestValidateInstanceName(t *testing.T) {
	for name, value := range map[string]string{
		"valid":   "Astreinte Nord",
		"empty":   "",
		"toolong": string(make([]rune, 81)),
	} {
		t.Run(name, func(t *testing.T) {
			err := validateInstanceName(value)
			if name == "valid" && err != nil {
				t.Fatalf("expected valid instance name, got %v", err)
			}
			if name != "valid" && err == nil {
				t.Fatal("expected invalid instance name")
			}
		})
	}
}

func TestSessionTokenIsStrictAndStable(t *testing.T) {
	token, digest, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := sessionTokenDigest(token)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != digest {
		t.Fatal("expected token digest to be stable")
	}
	if _, err := sessionTokenDigest(token + "="); err == nil {
		t.Fatal("expected padded token to be rejected")
	}
}

func TestNormalizeUsername(t *testing.T) {
	if got := normalizeUsername("  Gregory.N  "); got != "gregory.n" {
		t.Fatalf("unexpected normalized username: %q", got)
	}
}
