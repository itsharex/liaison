package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/manager/iam"
	"github.com/liaisonio/liaison/pkg/liaison/repo/model"
	"gorm.io/gorm"
)

func TestWebSFTPTargetRequiresFilePermission(t *testing.T) {
	cp, r := newTestControlPlane(t)
	defer r.Close()
	_, app := createTestEdgeApplication(t, r)
	app.ApplicationType = model.ApplicationTypeSSH
	if err := r.UpdateApplication(app); err != nil {
		t.Fatal(err)
	}
	p := &model.Proxy{Name: "sftp", ApplicationID: app.ID, AccessProtocol: model.AccessProtocolWebSFTP, Status: model.ProxyStatusRunning}
	if err := r.CreateProxy(p); err != nil {
		t.Fatal(err)
	}
	grantTestResourceToUsers(t, r, resourceAccess, p.ID, 1)
	ctx := context.WithValue(context.Background(), "user_id", uint(1))
	cp.authorizeFeature = nil
	if _, err := cp.GetWebSSHTarget(ctx, p.ID); !errors.Is(err, iam.ErrForbidden) {
		t.Fatalf("missing authorizer must deny, got %v", err)
	}
	cp.authorizeFeature = func(context.Context, string) error { return iam.ErrForbidden }
	if _, err := cp.GetWebSSHTarget(ctx, p.ID); !errors.Is(err, iam.ErrForbidden) {
		t.Fatalf("disabled feature must deny, got %v", err)
	}
	cp.authorizeFeature = func(_ context.Context, feature string) error {
		if feature != iam.FeatureFilesRead {
			t.Fatalf("wrong feature %q", feature)
		}
		return nil
	}
	target, err := cp.GetWebSSHTarget(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if target.AccessProtocol != model.AccessProtocolWebSFTP {
		t.Fatal("access protocol lost")
	}
	other := context.WithValue(context.Background(), "user_id", uint(2))
	if _, err := cp.GetWebSSHTarget(other, p.ID); err == nil {
		t.Fatal("another user must not access this target")
	}
}

func TestWebSFTPConnectionProfiles(t *testing.T) {
	cp, r := newTestControlPlane(t)
	defer r.Close()
	_, app := createTestEdgeApplication(t, r)
	app.ApplicationType = model.ApplicationTypeSSH
	if err := r.UpdateApplication(app); err != nil {
		t.Fatal(err)
	}
	p := &model.Proxy{Name: "files", ApplicationID: app.ID, AccessProtocol: model.AccessProtocolWebSFTP, Status: model.ProxyStatusRunning}
	if err := r.CreateProxy(p); err != nil {
		t.Fatal(err)
	}
	grantTestResourceToUsers(t, r, resourceAccess, p.ID, 1, 2)
	cp.authorizeFeature = func(context.Context, string) error { return nil }
	ctx := context.WithValue(context.Background(), "user_id", uint(1))
	save := func(name, secret, nonce string, remember, create bool) error {
		return cp.SaveWebSFTPConnection(ctx, p.ID, name, "demo", secret, nonce, remember, create)
	}
	if err := save("Files", "", "", false, true); err != nil {
		t.Fatal(err)
	}
	rows, err := cp.GetWebSSHCredentials(ctx, p.ID)
	if err != nil || len(rows) != 1 || rows[0].Saved || rows[0].Name != "Files" {
		t.Fatal("unsaved profile missing", err)
	}
	if err := save("Duplicate", "", "", false, true); err == nil {
		t.Fatal("duplicate must not overwrite")
	}
	if err := save("Files", "", "", true, false); err == nil {
		t.Fatal("cannot retain nonexistent password")
	}
	if err := save("Files", "encrypted-fixture", "nonce-fixture", true, false); err != nil {
		t.Fatal(err)
	}
	if err := save("Renamed", "", "", true, false); err != nil {
		t.Fatal(err)
	}
	secret, err := cp.GetWebSSHCredentialSecret(ctx, p.ID, "demo")
	if err != nil || secret.EncryptedPassword != "encrypted-fixture" {
		t.Fatal("rename lost secret", err)
	}
	other := context.WithValue(context.Background(), "user_id", uint(2))
	rows, err = cp.GetWebSSHCredentials(other, p.ID)
	if err != nil || len(rows) != 0 {
		t.Fatal("profile leaked", err)
	}
	if err := cp.SaveWebSFTPConnection(other, p.ID, "Other", "demo", "", "", false, false); err == nil {
		t.Fatal("other user edited profile")
	}
	if err := save("Renamed", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	secret, err = cp.GetWebSSHCredentialSecret(ctx, p.ID, "demo")
	if err != nil || secret.EncryptedPassword != "" || secret.Nonce != "" {
		t.Fatal("clear failed", err)
	}
	cp.authorizeFeature = func(context.Context, string) error { return iam.ErrForbidden }
	if err := save("Denied", "", "", false, false); !errors.Is(err, iam.ErrForbidden) {
		t.Fatal("feature gate bypassed", err)
	}
	// The access creation form can save an SSH profile without opening a PTY.
	cp.authorizeFeature = func(context.Context, string) error { return nil }
	sshProxy := &model.Proxy{Name: "shell", ApplicationID: app.ID, AccessProtocol: model.AccessProtocolWebSSH, Status: model.ProxyStatusRunning}
	if err := r.CreateProxy(sshProxy); err != nil {
		t.Fatal(err)
	}
	grantTestResourceToUsers(t, r, resourceAccess, sshProxy.ID, 1, 2)
	if err := cp.SaveWebSFTPConnection(ctx, sshProxy.ID, "Shell", "demo", "encrypted-fixture", "nonce-fixture", true, true); err != nil {
		t.Fatal(err)
	}
	rows, err = cp.GetWebSSHCredentials(ctx, sshProxy.ID)
	if err != nil || len(rows) != 1 || !rows[0].Saved {
		t.Fatal("SSH profile missing", err)
	}
	rows, err = cp.GetWebSSHCredentials(other, sshProxy.ID)
	if err != nil || len(rows) != 0 {
		t.Fatal("SSH profile leaked", err)
	}
}

func TestWebSSHTargetRequiresSSHApplication(t *testing.T) {
	cp, r := newTestControlPlane(t)
	defer r.Close()
	_, application := createTestEdgeApplication(t, r)
	proxy := &model.Proxy{
		Name:          "proxy-1",
		ApplicationID: application.ID,
		Port:          39022,
		Status:        model.ProxyStatusRunning,
	}
	if err := r.CreateProxy(proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	if _, err := cp.GetWebSSHTarget(context.Background(), proxy.ID); err == nil || !strings.Contains(err.Error(), "SSH") {
		t.Fatalf("GetWebSSHTarget error = %v, want SSH-only error", err)
	}
}

func TestWebSSHCredentialsAreScopedByUserAndUsername(t *testing.T) {
	cp, r := newTestControlPlane(t)
	defer r.Close()
	_, application := createTestEdgeApplication(t, r)
	application.ApplicationType = model.ApplicationTypeSSH
	application.Port = 22
	if err := r.UpdateApplication(application); err != nil {
		t.Fatalf("update application: %v", err)
	}
	proxy := &model.Proxy{
		Name:          "ssh-proxy",
		ApplicationID: application.ID,
		Port:          39022,
		Status:        model.ProxyStatusRunning,
	}
	if err := r.CreateProxy(proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	grantTestResourceToUsers(t, r, resourceAccess, proxy.ID, 1, 2)

	ctxUser1 := context.WithValue(context.Background(), "user_id", uint(1))
	ctxUser2 := context.WithValue(context.Background(), "user_id", uint(2))
	if err := cp.SaveWebSSHCredential(ctxUser1, proxy.ID, "root", "enc-user1-root", "nonce-1"); err != nil {
		t.Fatalf("save user1 root credential: %v", err)
	}
	if err := cp.SaveWebSSHCredential(ctxUser1, proxy.ID, "admin", "enc-user1-admin", "nonce-2"); err != nil {
		t.Fatalf("save user1 admin credential: %v", err)
	}
	if err := cp.SaveWebSSHCredential(ctxUser2, proxy.ID, "root", "enc-user2-root", "nonce-3"); err != nil {
		t.Fatalf("save user2 root credential: %v", err)
	}

	user1Credentials, err := cp.GetWebSSHCredentials(ctxUser1, proxy.ID)
	if err != nil {
		t.Fatalf("GetWebSSHCredentials user1: %v", err)
	}
	if len(user1Credentials) != 2 {
		t.Fatalf("user1 credential count = %d, want 2", len(user1Credentials))
	}
	user2Credentials, err := cp.GetWebSSHCredentials(ctxUser2, proxy.ID)
	if err != nil {
		t.Fatalf("GetWebSSHCredentials user2: %v", err)
	}
	if len(user2Credentials) != 1 || user2Credentials[0].Username != "root" {
		t.Fatalf("user2 credentials = %+v, want only root", user2Credentials)
	}

	user1Root, err := cp.GetWebSSHCredentialSecret(ctxUser1, proxy.ID, "root")
	if err != nil {
		t.Fatalf("GetWebSSHCredentialSecret user1 root: %v", err)
	}
	if user1Root.EncryptedPassword != "enc-user1-root" {
		t.Fatalf("user1 root encrypted password = %q", user1Root.EncryptedPassword)
	}
	user2Root, err := cp.GetWebSSHCredentialSecret(ctxUser2, proxy.ID, "root")
	if err != nil {
		t.Fatalf("GetWebSSHCredentialSecret user2 root: %v", err)
	}
	if user2Root.EncryptedPassword != "enc-user2-root" {
		t.Fatalf("user2 root encrypted password = %q", user2Root.EncryptedPassword)
	}

	if err := cp.DeleteWebSSHCredential(ctxUser1, proxy.ID, " "); err == nil {
		t.Fatal("DeleteWebSSHCredential with empty username succeeded")
	}
	user1Credentials, err = cp.GetWebSSHCredentials(ctxUser1, proxy.ID)
	if err != nil {
		t.Fatalf("GetWebSSHCredentials user1 after empty delete: %v", err)
	}
	if len(user1Credentials) != 2 {
		t.Fatalf("user1 credential count after empty delete = %d, want 2", len(user1Credentials))
	}

	if err := cp.DeleteWebSSHCredential(ctxUser1, proxy.ID, "root"); err != nil {
		t.Fatalf("DeleteWebSSHCredential user1 root: %v", err)
	}
	if _, err := cp.GetWebSSHCredentialSecret(ctxUser1, proxy.ID, "root"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("user1 root after delete error = %v, want record not found", err)
	}
	if _, err := cp.GetWebSSHCredentialSecret(ctxUser2, proxy.ID, "root"); err != nil {
		t.Fatalf("user2 root should remain after user1 delete: %v", err)
	}
}

func TestWebSSHTargetReportsStatusAndHostKey(t *testing.T) {
	cp, r := newTestControlPlane(t)
	defer r.Close()
	_, application := createTestEdgeApplication(t, r)
	application.ApplicationType = model.ApplicationTypeSSH
	application.Port = 22
	if err := r.UpdateApplication(application); err != nil {
		t.Fatalf("update application: %v", err)
	}
	proxy := &model.Proxy{
		Name:          "ssh-proxy",
		ApplicationID: application.ID,
		Port:          39022,
		Status:        model.ProxyStatusRunning,
	}
	if err := r.CreateProxy(proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	target, err := cp.GetWebSSHTarget(context.Background(), proxy.ID)
	if err != nil {
		t.Fatalf("GetWebSSHTarget: %v", err)
	}
	if target.EffectiveStatus != proxyEffectiveStatusActive {
		t.Fatalf("effective status = %q, want %q", target.EffectiveStatus, proxyEffectiveStatusActive)
	}
	if target.HostKey == nil || target.HostKey.Trusted {
		t.Fatalf("host key = %+v, want untrusted", target.HostKey)
	}

	if err := cp.TrustWebSSHHostKey(context.Background(), proxy.ID, "ssh-ed25519", "SHA256:test", "public-key"); err != nil {
		t.Fatalf("TrustWebSSHHostKey: %v", err)
	}
	target, err = cp.GetWebSSHTarget(context.Background(), proxy.ID)
	if err != nil {
		t.Fatalf("GetWebSSHTarget after trust: %v", err)
	}
	if target.HostKey == nil || !target.HostKey.Trusted || target.HostKey.FingerprintSHA256 != "SHA256:test" {
		t.Fatalf("host key = %+v, want trusted SHA256:test", target.HostKey)
	}

	if err := cp.DeleteWebSSHHostKey(context.Background(), proxy.ID); err != nil {
		t.Fatalf("DeleteWebSSHHostKey: %v", err)
	}
	target, err = cp.GetWebSSHTarget(context.Background(), proxy.ID)
	if err != nil {
		t.Fatalf("GetWebSSHTarget after delete: %v", err)
	}
	if target.HostKey == nil || target.HostKey.Trusted {
		t.Fatalf("host key = %+v, want untrusted after reset", target.HostKey)
	}
}
