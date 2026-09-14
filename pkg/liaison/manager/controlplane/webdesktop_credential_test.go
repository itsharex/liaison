package controlplane

import (
	"context"
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/repo/model"
)

func TestDesktopCreationCredentialIsPrivate(t *testing.T) {
	for _, protocol := range []string{"rdp", "vnc"} {
		t.Run(protocol, func(t *testing.T) {
			cp, r := newTestControlPlane(t)
			defer r.Close()
			_, app := createTestEdgeApplication(t, r)
			app.ApplicationType = model.ApplicationType(protocol)
			if err := r.UpdateApplication(app); err != nil {
				t.Fatal(err)
			}
			p := &model.Proxy{Name: "desktop", ApplicationID: app.ID, AccessProtocol: model.AccessProtocol("web" + protocol), Status: model.ProxyStatusRunning}
			if err := r.CreateProxy(p); err != nil {
				t.Fatal(err)
			}
			grantTestResourceToUsers(t, r, resourceAccess, p.ID, 1, 2)
			ctx := context.WithValue(context.Background(), "user_id", uint(1))
			other := context.WithValue(context.Background(), "user_id", uint(2))
			user := "demo"
			if protocol == "vnc" {
				user = ""
			}
			if err := cp.SaveWebDesktopCredential(ctx, p.ID, protocol, user, "", "encrypted-fixture", "nonce-fixture"); err != nil {
				t.Fatal(err)
			}
			target, err := cp.GetWebDesktopTarget(ctx, p.ID)
			if err != nil || len(target.Credentials) != 1 || !target.Credentials[0].Saved {
				t.Fatal("saved credential missing", err)
			}
			target, err = cp.GetWebDesktopTarget(other, p.ID)
			if err != nil || len(target.Credentials) != 0 {
				t.Fatal("credential leaked", err)
			}
			if _, err := cp.GetWebDesktopCredentialSecret(other, p.ID, protocol, user, ""); err == nil {
				t.Fatal("other user read secret")
			}
			if err := cp.SaveWebDesktopCredential(ctx, p.ID, protocol, user, "", "", ""); err != nil {
				t.Fatal(err)
			}
			target, err = cp.GetWebDesktopTarget(ctx, p.ID)
			if err != nil || len(target.Credentials) != 1 || target.Credentials[0].Saved {
				t.Fatal("metadata-only account must not report a saved password", err)
			}
			secret, err := r.GetWebDesktopCredential(p.ID, 1, protocol, user, "")
			if err != nil || secret.EncryptedPassword != "" || secret.Nonce != "" {
				t.Fatal("password was not removed", err)
			}
		})
	}
}
