package features

import "testing"

// A feature's Settings reach its own table alone: another feature's values
// and secrets are not found, and each call reads the file as it is now.
func TestSettingsReachOnlyTheirOwnFeaturesTable(t *testing.T) {
	tables := map[string]Table{
		"a": {Values: map[string]string{"host": "a.example"}, Secrets: map[string]string{"token": "a-secret"}},
		"b": {Values: map[string]string{"port": "8080"}, Secrets: map[string]string{"hook": "b-secret"}},
	}
	a := SettingsIn(func() map[string]Table { return tables }, "a")
	if value, found := a.Setting("host"); !found || value != "a.example" {
		t.Errorf("a's host is %q, %v", value, found)
	}
	if value, found := a.Secret("token"); !found || value != "a-secret" {
		t.Errorf("a's token is %q, %v", value, found)
	}
	if value, found := a.Secret("hook"); found {
		t.Errorf("a read b's secret %q", value)
	}
	if value, found := a.Setting("port"); found {
		t.Errorf("a read b's setting %q", value)
	}
	if _, found := a.Setting("token"); found {
		t.Error("a secret is read as a setting")
	}
	tables = map[string]Table{"a": {Values: map[string]string{"host": "new.example"}}}
	if value, _ := a.Setting("host"); value != "new.example" {
		t.Errorf("after the file changed, a's host is %q", value)
	}
	var none Settings
	if _, found := none.Secret("token"); found {
		t.Error("the zero Settings holds a secret")
	}
}
