package features

// Settings are a feature's own table in the settings file, features.toml
// beside policy.toml, such as a git host's token: not the log's settings,
// which Reader has. Each call reads the version in force, so an edit to the
// file reaches a running job. The zero Settings has none.
type Settings struct {
	feature string
	file    func() map[string]Table
}

// Table is one feature's table in the settings file: its values, and its
// secrets, from the [<feature>.secrets] table under it.
type Table struct{ Values, Secrets map[string]string }

// SettingsIn are feature's settings in file, which returns the tables in
// force, by feature. They reach that feature's table alone.
func SettingsIn(file func() map[string]Table, feature string) Settings {
	return Settings{feature: feature, file: file}
}

// Setting is the value of key, and whether the file sets it.
func (s Settings) Setting(key string) (string, bool) {
	value, found := s.table().Values[key]
	return value, found
}

// Secret is the secret named key, and whether the file holds it. A secret
// is never written to the log, a snapshot, the page or the API.
func (s Settings) Secret(key string) (string, bool) {
	value, found := s.table().Secrets[key]
	return value, found
}

func (s Settings) table() Table {
	if s.file == nil {
		return Table{}
	}
	return s.file()[s.feature]
}
