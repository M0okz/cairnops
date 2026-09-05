package proxmox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

var managedUser = regexp.MustCompile(`^cairnops-[0-9a-f]{16}@pve$`)

type ManagedCredential struct {
	UserID      string
	Credentials Credentials
}

func (client *Client) CheckProvisioning(ctx context.Context, address string, installer Credentials) error {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return err
	}
	for path, required := range map[string][]string{"/access": {"User.Modify", "Permissions.Modify"}, "/access/realm/pve": {"Realm.AllocateUser"}} {
		var permissions map[string]map[string]int
		if err := client.request(ctx, endpoint, http.MethodGet, "/access/permissions?path="+url.QueryEscape(path), installer, nil, &permissions); err != nil {
			return err
		}
		for _, privilege := range required {
			if permissions[path][privilege] != 1 {
				return fmt.Errorf("temporary installer requires %s at %s; use existing-token mode for an auditor credential", privilege, path)
			}
		}
	}
	return nil
}

// Provision uses the temporary installer token only within this call. The new
// pve user has no password and no access beyond a propagated auditor role.
func (client *Client) Provision(ctx context.Context, address string, installer Credentials) (result ManagedCredential, err error) {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return result, err
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return result, fmt.Errorf("name Proxmox VE account: %w", err)
	}
	userID := "cairnops-" + hex.EncodeToString(nonce[:]) + "@pve"
	if err := client.request(ctx, endpoint, http.MethodPost, "/access/users", installer, url.Values{"userid": {userID}, "comment": {"CairnOps managed read-only integration"}, "enable": {"1"}}, nil); err != nil {
		return result, err
	}
	defer func() {
		if err == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if cleanupErr := client.RemoveManaged(cleanupCtx, endpoint, installer, userID); cleanupErr != nil {
			err = fmt.Errorf("%w; remote account %s could not be removed: %v", err, userID, cleanupErr)
		}
	}()
	acl := url.Values{"path": {"/"}, "roles": {"PVEAuditor"}, "propagate": {"1"}, "users": {userID}}
	if err = client.request(ctx, endpoint, http.MethodPut, "/access/acl", installer, acl, nil); err != nil {
		return result, err
	}
	var token struct {
		Value  string `json:"value"`
		FullID string `json:"full-tokenid"`
	}
	path := "/access/users/" + url.PathEscape(userID) + "/token/observer"
	if err = client.request(ctx, endpoint, http.MethodPost, path, installer, url.Values{"privsep": {"1"}, "comment": {"CairnOps runtime"}}, &token); err != nil {
		return result, err
	}
	if token.Value == "" || token.FullID != userID+"!observer" {
		return result, fmt.Errorf("Proxmox VE returned an invalid managed token")
	}
	delete(acl, "users")
	acl.Set("tokens", token.FullID)
	if err = client.request(ctx, endpoint, http.MethodPut, "/access/acl", installer, acl, nil); err != nil {
		return result, err
	}
	result = ManagedCredential{UserID: userID, Credentials: Credentials{TokenID: token.FullID, Secret: token.Value, Fingerprint: installer.Fingerprint}}
	if _, err = client.Resources(ctx, endpoint, result.Credentials); err != nil {
		return ManagedCredential{}, fmt.Errorf("verify Proxmox VE runtime token: %w", err)
	}
	return result, nil
}

// Removing an identity needs administrative permission in Proxmox VE. Do not
// grant that permission to the runtime token just to enable self-cleanup.
func (client *Client) RemoveManaged(ctx context.Context, address string, installer Credentials, userID string) error {
	if !managedUser.MatchString(userID) {
		return fmt.Errorf("refuse removal of a non-CairnOps Proxmox VE account")
	}
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return err
	}
	// Checking the user list makes retries idempotent when the preceding DELETE
	// succeeded but its response or the local transaction was lost.
	var users []struct {
		ID      string `json:"userid"`
		Comment string `json:"comment"`
	}
	if err := client.request(ctx, endpoint, http.MethodGet, "/access/users", installer, nil, &users); err != nil {
		return err
	}
	for _, user := range users {
		if user.ID != userID {
			continue
		}
		if user.Comment != "CairnOps managed read-only integration" {
			return fmt.Errorf("Proxmox VE account ownership changed; remote cleanup requires review")
		}
		return client.request(ctx, endpoint, http.MethodDelete, "/access/users/"+url.PathEscape(userID), installer, nil, nil)
	}
	// User lists can be filtered. Prove the installer can administer identities
	// before concluding that an absent user was already removed.
	var permissions map[string]map[string]int
	if err := client.request(ctx, endpoint, http.MethodGet, "/access/permissions?path=/access", installer, nil, &permissions); err != nil {
		return err
	}
	if permissions["/access"]["User.Modify"] != 1 {
		return fmt.Errorf("administrative Proxmox VE authorization is required to confirm account removal")
	}
	return nil
}
