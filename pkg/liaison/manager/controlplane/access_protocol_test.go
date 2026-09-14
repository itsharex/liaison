package controlplane

import (
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/repo/model"
)

func TestNormalizeAccessProtocol(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		appType   model.ApplicationType
		port      int
		expose    bool
		want      model.AccessProtocol
		wantError bool
	}{
		{name: "explicit websftp", raw: " WebSFTP ", appType: model.ApplicationTypeSSH, want: model.AccessProtocolWebSFTP},
		{name: "websftp rejects tcp", raw: "websftp", appType: model.ApplicationTypeTCP, wantError: true},
		{name: "websftp rejects database", raw: "websftp", appType: model.ApplicationTypeMySQL, wantError: true},
		{name: "legacy ssh public port remains tcp", appType: model.ApplicationTypeSSH, port: 2222, expose: true, want: model.AccessProtocolTCP},
		{name: "legacy ssh web access becomes webssh", appType: model.ApplicationTypeSSH, port: 0, want: model.AccessProtocolWebSSH},
		{name: "explicit native ssh", raw: " SSH ", appType: model.ApplicationTypeSSH, expose: true, want: model.AccessProtocolSSH},
		{name: "http defaults to http", appType: model.ApplicationTypeHTTP, want: model.AccessProtocolHTTP},
		{name: "http application allows explicit tcp passthrough", raw: "tcp", appType: model.ApplicationTypeHTTP, port: 8080, expose: true, want: model.AccessProtocolTCP},
		{name: "mysql rejects unimplemented native server", raw: "mysql", appType: model.ApplicationTypeMySQL, port: 3306, expose: true, wantError: true},
		{name: "rdp rejects unimplemented native server", raw: "rdp", appType: model.ApplicationTypeRDP, port: 3389, expose: true, wantError: true},
		{name: "vnc rejects unimplemented native server", raw: "vnc", appType: model.ApplicationTypeVNC, wantError: true},
		{name: "postgres rejects native server", raw: "postgresql", appType: model.ApplicationTypePostgreSQL, wantError: true},
		{name: "redis rejects native server", raw: "redis", appType: model.ApplicationTypeRedis, wantError: true},
		{name: "mongo rejects native server", raw: "mongodb", appType: model.ApplicationTypeMongoDB, wantError: true},
		{name: "rdp allows web client", raw: "web", appType: model.ApplicationTypeRDP, want: model.AccessProtocolWeb},
		{name: "vnc allows web client", raw: "web", appType: model.ApplicationTypeVNC, want: model.AccessProtocolWeb},
		{name: "rdp allows opaque tcp", raw: "tcp", appType: model.ApplicationTypeRDP, want: model.AccessProtocolTCP},
		{name: "mysql native protocol rejects postgresql application", raw: "mysql", appType: model.ApplicationTypePostgreSQL, wantError: true},
		{name: "native ssh rejects tcp application", raw: "ssh", appType: model.ApplicationTypeTCP, wantError: true},
		{name: "webssh rejects tcp application", raw: "webssh", appType: model.ApplicationTypeTCP, wantError: true},
		{name: "http rejects ssh application", raw: "http", appType: model.ApplicationTypeSSH, wantError: true},
		{name: "unknown protocol rejected", raw: "telnet", appType: model.ApplicationTypeTCP, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := &model.Application{ApplicationType: test.appType}
			got, err := normalizeAccessProtocol(test.raw, application, test.port, test.expose)
			if test.wantError {
				if err == nil {
					t.Fatalf("normalizeAccessProtocol() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeAccessProtocol() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("normalizeAccessProtocol() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEffectiveAccessProtocolBackfillsLegacyRows(t *testing.T) {
	if accessProtocolRequiresPublicPort(model.AccessProtocolWebSFTP) {
		t.Fatal("WebSFTP must not expose a public port")
	}
	tests := []struct {
		name    string
		proxy   *model.Proxy
		appType model.ApplicationType
		want    model.AccessProtocol
	}{
		{name: "stored websftp wins", proxy: &model.Proxy{AccessProtocol: model.AccessProtocolWebSFTP}, appType: model.ApplicationTypeSSH, want: model.AccessProtocolWebSFTP},
		{name: "stored protocol wins", proxy: &model.Proxy{AccessProtocol: model.AccessProtocolSSH, Port: 2200}, appType: model.ApplicationTypeSSH, want: model.AccessProtocolSSH},
		{name: "stored native database protocol wins", proxy: &model.Proxy{AccessProtocol: model.AccessProtocolMySQL, Port: 3306}, appType: model.ApplicationTypeMySQL, want: model.AccessProtocolMySQL},
		{name: "legacy ssh public port", proxy: &model.Proxy{Port: 2200}, appType: model.ApplicationTypeSSH, want: model.AccessProtocolTCP},
		{name: "legacy ssh browser", proxy: &model.Proxy{Port: 0}, appType: model.ApplicationTypeSSH, want: model.AccessProtocolWebSSH},
		{name: "legacy http", proxy: &model.Proxy{Port: 443}, appType: model.ApplicationTypeHTTP, want: model.AccessProtocolHTTP},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := &model.Application{ApplicationType: test.appType}
			if got := effectiveAccessProtocol(test.proxy, application); got != test.want {
				t.Fatalf("effectiveAccessProtocol() = %q, want %q", got, test.want)
			}
		})
	}
}
