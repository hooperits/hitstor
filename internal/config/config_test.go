package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLookupKey(t *testing.T) {
	tests := []struct {
		name     string
		wantOK   bool
		wantType KeyType
	}{
		{"repo", true, String},
		{"parallel", true, Int},
		{"exclude", true, List},
		{"s3_access_key", true, String},
		{"unknown", false, 0},
		{"", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := LookupKey(tt.name)
			if ok != tt.wantOK {
				t.Errorf("LookupKey(%q) ok = %v, want %v", tt.name, ok, tt.wantOK)
			}
			if ok && def.Type != tt.wantType {
				t.Errorf("LookupKey(%q) type = %v, want %v", tt.name, def.Type, tt.wantType)
			}
		})
	}
}

func TestIsListKey(t *testing.T) {
	if !IsListKey("exclude") {
		t.Error("exclude should be a list key")
	}
	if IsListKey("repo") {
		t.Error("repo should not be a list key")
	}
	if IsListKey("unknown") {
		t.Error("unknown should not be a list key")
	}
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		wantErr bool
	}{
		// repo
		{"repo", "/backups/hitstor", false},
		{"repo", "s3:http://localhost:9000/mybucket", false},
		{"repo", "", true},
		{"repo", "s3:", true},         // missing endpoint/bucket
		{"repo", "s3:endpoint", true}, // missing bucket
		// password_file
		{"password_file", "/etc/passwd", false},
		{"password_file", "", true},
		// s3_access_key
		{"s3_access_key", "AKIAIOSFODNN7EXAMPLE", false},
		{"s3_access_key", "", true},
		// parallel
		{"parallel", "0", false},
		{"parallel", "4", false},
		{"parallel", "-1", true},
		{"parallel", "abc", true},
		// unknown key
		{"nonexistent", "value", true},
		// exclude (no validation function)
		{"exclude", "*.tmp", false},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			err := ValidateKey(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKey(%q, %q) error = %v, wantErr %v", tt.key, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"ab", "****"},
		{"abcd", "****"},
		{"abcde", "****bcde"},
		{"AKIAIOSFODNN7EXAMPLE", "****MPLE"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := MaskValue(tt.input)
			if got != tt.want {
				t.Errorf("MaskValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load missing file should not error, got: %v", err)
	}
	if cfg.Exists {
		t.Error("cfg.Exists should be false for missing file")
	}
	if cfg.Path != "/nonexistent/path/config.yaml" {
		t.Errorf("cfg.Path = %q, want /nonexistent/path/config.yaml", cfg.Path)
	}
}

func TestLoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `repo: /backups/test
parallel: 4
exclude:
  - "*.tmp"
  - "*.log"
s3_region: eu-west-1
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !cfg.Exists {
		t.Error("cfg.Exists should be true")
	}
	if cfg.Repo != "/backups/test" {
		t.Errorf("Repo = %q, want /backups/test", cfg.Repo)
	}
	if cfg.Parallel != 4 {
		t.Errorf("Parallel = %d, want 4", cfg.Parallel)
	}
	if len(cfg.Exclude) != 2 || cfg.Exclude[0] != "*.tmp" || cfg.Exclude[1] != "*.log" {
		t.Errorf("Exclude = %v, want [*.tmp *.log]", cfg.Exclude)
	}
	if cfg.S3Region != "eu-west-1" {
		t.Errorf("S3Region = %q, want eu-west-1", cfg.S3Region)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(":::invalid"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load invalid YAML should return error")
	}
}

func TestLoadPartialYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `repo: /backups/only-repo
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Repo != "/backups/only-repo" {
		t.Errorf("Repo = %q, want /backups/only-repo", cfg.Repo)
	}
	if cfg.Parallel != 0 {
		t.Errorf("Parallel = %d, want 0", cfg.Parallel)
	}
	if len(cfg.Exclude) != 0 {
		t.Errorf("Exclude = %v, want empty", cfg.Exclude)
	}
}

func TestSaveCreatesDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "config.yaml")

	cfg := Config{
		Repo:     "/test",
		Parallel: 2,
		Path:     path,
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify file exists and is readable.
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after save error: %v", err)
	}
	if loaded.Repo != "/test" {
		t.Errorf("Repo = %q, want /test", loaded.Repo)
	}
	if loaded.Parallel != 2 {
		t.Errorf("Parallel = %d, want 2", loaded.Parallel)
	}
}

func TestSavePreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Save initial config.
	cfg1 := Config{
		Repo:     "/initial",
		Parallel: 8,
		Path:     path,
	}
	if err := Save(cfg1); err != nil {
		t.Fatal(err)
	}

	// Load, modify one key, save.
	cfg2, _ := Load(path)
	cfg2.Parallel = 4
	if err := Save(cfg2); err != nil {
		t.Fatal(err)
	}

	// Verify both keys.
	cfg3, _ := Load(path)
	if cfg3.Repo != "/initial" {
		t.Errorf("Repo = %q, want /initial", cfg3.Repo)
	}
	if cfg3.Parallel != 4 {
		t.Errorf("Parallel = %d, want 4", cfg3.Parallel)
	}
}

func TestSaveAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := Config{Repo: "/atomic", Path: path}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Verify no .tmp file left behind.
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("temp file %s should not exist after save", tmp)
	}
}

func TestSetAndGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Set on missing file creates it.
	old, err := Set(path, "repo", "/backups/test")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if old != "" {
		t.Errorf("old = %q, want empty", old)
	}

	// Set again replaces.
	old, err = Set(path, "repo", "/backups/new")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if old != "/backups/test" {
		t.Errorf("old = %q, want /backups/test", old)
	}

	// Verify persisted.
	cfg, _ := Load(path)
	if cfg.Repo != "/backups/new" {
		t.Errorf("Repo = %q, want /backups/new", cfg.Repo)
	}
}

func TestSetValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_, err := Set(path, "parallel", "abc")
	if err == nil {
		t.Fatal("Set with invalid value should return error")
	}
}

func TestSetUnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_, err := Set(path, "bogus", "value")
	if err == nil {
		t.Fatal("Set with unknown key should return error")
	}
}

func TestUnset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Set then unset.
	Set(path, "parallel", "4")
	old, wasSet, err := Unset(path, "parallel")
	if err != nil {
		t.Fatalf("Unset error: %v", err)
	}
	if !wasSet {
		t.Error("wasSet should be true")
	}
	if old != "4" {
		t.Errorf("old = %q, want 4", old)
	}

	// Verify parallel is gone.
	cfg, _ := Load(path)
	if cfg.Parallel != 0 {
		t.Errorf("Parallel = %d, want 0 after unset", cfg.Parallel)
	}
}

func TestUnsetAlreadyAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Save empty config.
	Save(Config{Path: path})

	_, wasSet, err := Unset(path, "parallel")
	if err != nil {
		t.Fatalf("Unset error: %v", err)
	}
	if wasSet {
		t.Error("wasSet should be false for absent key")
	}
}

func TestUnsetPreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	Set(path, "repo", "/test")
	Set(path, "parallel", "4")
	Unset(path, "parallel")

	cfg, _ := Load(path)
	if cfg.Repo != "/test" {
		t.Errorf("Repo = %q, want /test (should be preserved)", cfg.Repo)
	}
}

func TestAddToList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Add to empty list.
	list, err := AddToList(path, "exclude", "*.tmp")
	if err != nil {
		t.Fatalf("AddToList error: %v", err)
	}
	if len(list) != 1 || list[0] != "*.tmp" {
		t.Errorf("list = %v, want [*.tmp]", list)
	}

	// Add another.
	list, err = AddToList(path, "exclude", "*.log")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[1] != "*.log" {
		t.Errorf("list = %v, want [*.tmp *.log]", list)
	}
}

func TestAddToListNonListKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_, err := AddToList(path, "parallel", "4")
	if err == nil {
		t.Fatal("AddToList on non-list key should error")
	}
}

func TestRemoveFromList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	AddToList(path, "exclude", "*.tmp")
	AddToList(path, "exclude", "*.log")

	list, found, err := RemoveFromList(path, "exclude", "*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("found should be true")
	}
	if len(list) != 1 || list[0] != "*.log" {
		t.Errorf("list = %v, want [*.log]", list)
	}
}

func TestRemoveFromListNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	AddToList(path, "exclude", "*.tmp")

	_, found, err := RemoveFromList(path, "exclude", "*.xyz")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("found should be false when value not in list")
	}
}

func TestRemoveFromListNonListKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_, _, err := RemoveFromList(path, "repo", "/test")
	if err == nil {
		t.Fatal("RemoveFromList on non-list key should error")
	}
}

// mockFlags implements FlagChecker for testing.
type mockFlags struct {
	changed map[string]bool
}

func (m mockFlags) Changed(name string) bool {
	return m.changed[name]
}

func TestResolveAllDefaults(t *testing.T) {
	cfg := Config{Path: "/test", Exists: false}
	flags := mockFlags{changed: map[string]bool{}}
	envLookup := func(key string) (string, bool) { return "", false }

	resolved := ResolveAll(cfg, flags, envLookup)
	if len(resolved) != len(Registry) {
		t.Fatalf("got %d resolved keys, want %d", len(resolved), len(Registry))
	}

	for _, rk := range resolved {
		if rk.Source != "default" {
			t.Errorf("key %s source = %q, want default", rk.Def.Name, rk.Source)
		}
	}
}

func TestResolveAllConfigOverridesDefault(t *testing.T) {
	cfg := Config{
		Repo:     "/from/config",
		Parallel: 4,
		Path:     "/test",
		Exists:   true,
	}
	flags := mockFlags{changed: map[string]bool{}}
	envLookup := func(key string) (string, bool) { return "", false }

	resolved := ResolveAll(cfg, flags, envLookup)
	for _, rk := range resolved {
		if rk.Def.Name == "repo" {
			if rk.Value != "/from/config" || rk.Source != "config" {
				t.Errorf("repo: value=%q source=%q, want /from/config config", rk.Value, rk.Source)
			}
		}
		if rk.Def.Name == "parallel" {
			if rk.Value != "4" || rk.Source != "config" {
				t.Errorf("parallel: value=%q source=%q, want 4 config", rk.Value, rk.Source)
			}
		}
	}
}

func TestResolveAllEnvOverridesConfig(t *testing.T) {
	cfg := Config{
		Repo:   "/from/config",
		Path:   "/test",
		Exists: true,
	}
	flags := mockFlags{changed: map[string]bool{}}
	envLookup := func(key string) (string, bool) {
		if key == "HITSTOR_REPO" {
			return "/from/env", true
		}
		return "", false
	}

	resolved := ResolveAll(cfg, flags, envLookup)
	for _, rk := range resolved {
		if rk.Def.Name == "repo" {
			if rk.Value != "/from/env" || rk.Source != "env" {
				t.Errorf("repo: value=%q source=%q, want /from/env env", rk.Value, rk.Source)
			}
		}
	}
}

func TestResolveAllFlagOverridesEnv(t *testing.T) {
	cfg := Config{
		Repo:   "/from/flag",
		Path:   "/test",
		Exists: true,
	}
	flags := mockFlags{changed: map[string]bool{"repo": true}}
	envLookup := func(key string) (string, bool) {
		if key == "HITSTOR_REPO" {
			return "/from/env", true
		}
		return "", false
	}

	resolved := ResolveAll(cfg, flags, envLookup)
	for _, rk := range resolved {
		if rk.Def.Name == "repo" {
			if rk.Source != "flag" {
				t.Errorf("repo: source=%q, want flag", rk.Source)
			}
		}
	}
}

func TestGetConfigValue(t *testing.T) {
	cfg := Config{
		Repo:        "/test",
		Parallel:    4,
		Exclude:     []string{"*.tmp", "*.log"},
		S3AccessKey: "AKIA",
	}

	if v := GetConfigValue(cfg, "repo"); v != "/test" {
		t.Errorf("repo = %q", v)
	}
	if v := GetConfigValue(cfg, "parallel"); v != "4" {
		t.Errorf("parallel = %q", v)
	}
	if v := GetConfigValue(cfg, "exclude"); v != "*.tmp, *.log" {
		t.Errorf("exclude = %q", v)
	}
	if v := GetConfigValue(cfg, "s3_access_key"); v != "AKIA" {
		t.Errorf("s3_access_key = %q", v)
	}
}

func TestServeConfigKeys(t *testing.T) {
	// LookupKey
	def, ok := LookupKey("serve_port")
	if !ok {
		t.Fatal("serve_port not in registry")
	}
	if def.Type != Int {
		t.Errorf("serve_port type = %v, want Int", def.Type)
	}

	def, ok = LookupKey("serve_tls_cert")
	if !ok {
		t.Fatal("serve_tls_cert not in registry")
	}
	if def.Type != String {
		t.Errorf("serve_tls_cert type = %v, want String", def.Type)
	}

	def, ok = LookupKey("serve_tls_key")
	if !ok {
		t.Fatal("serve_tls_key not in registry")
	}
	if def.Type != String {
		t.Errorf("serve_tls_key type = %v, want String", def.Type)
	}
}

func TestServeConfigValidation(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		wantErr bool
	}{
		{"serve_port", "8420", false},
		{"serve_port", "1", false},
		{"serve_port", "65535", false},
		{"serve_port", "0", true},
		{"serve_port", "65536", true},
		{"serve_port", "abc", true},
		{"serve_tls_cert", "/path/to/cert.pem", false},
		{"serve_tls_cert", "", true},
		{"serve_tls_key", "/path/to/key.pem", false},
		{"serve_tls_key", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			err := ValidateKey(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKey(%q, %q) error = %v, wantErr %v", tt.key, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestServeConfigSetGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Set serve_port.
	if _, err := Set(path, "serve_port", "9000"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServePort != 9000 {
		t.Errorf("ServePort = %d, want 9000", cfg.ServePort)
	}

	// Set TLS paths.
	Set(path, "serve_tls_cert", "/etc/ssl/server.crt")
	Set(path, "serve_tls_key", "/etc/ssl/server.key")

	cfg, _ = Load(path)
	if cfg.ServeTLSCert != "/etc/ssl/server.crt" {
		t.Errorf("ServeTLSCert = %q, want /etc/ssl/server.crt", cfg.ServeTLSCert)
	}
	if cfg.ServeTLSKey != "/etc/ssl/server.key" {
		t.Errorf("ServeTLSKey = %q, want /etc/ssl/server.key", cfg.ServeTLSKey)
	}

	// GetConfigValue
	if v := GetConfigValue(cfg, "serve_port"); v != "9000" {
		t.Errorf("GetConfigValue serve_port = %q, want 9000", v)
	}
	if v := GetConfigValue(cfg, "serve_tls_cert"); v != "/etc/ssl/server.crt" {
		t.Errorf("GetConfigValue serve_tls_cert = %q", v)
	}

	// IsZeroValue
	if IsZeroValue(cfg, "serve_port") {
		t.Error("serve_port should not be zero")
	}
	if IsZeroValue(cfg, "serve_tls_cert") {
		t.Error("serve_tls_cert should not be zero")
	}

	// Unset
	Unset(path, "serve_port")
	cfg, _ = Load(path)
	if cfg.ServePort != 0 {
		t.Errorf("ServePort after unset = %d, want 0", cfg.ServePort)
	}
	if !IsZeroValue(cfg, "serve_port") {
		t.Error("serve_port should be zero after unset")
	}
}

func TestAgentConfigKeys(t *testing.T) {
	tests := []struct {
		name     string
		wantType KeyType
	}{
		{"agent_server", String},
		{"agent_api_key", String},
		{"agent_ca_cert", String},
		{"agent_hostname", String},
		{"agent_paths", List},
		{"agent_excludes", List},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := LookupKey(tt.name)
			if !ok {
				t.Fatalf("%s not in registry", tt.name)
			}
			if def.Type != tt.wantType {
				t.Errorf("type = %v, want %v", def.Type, tt.wantType)
			}
		})
	}
}

func TestAgentConfigValidation(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		wantErr bool
	}{
		{"agent_server", "localhost:8420", false},
		{"agent_server", "", true},
		{"agent_api_key", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", false},
		{"agent_api_key", "too-short", true},
		{"agent_api_key", "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789", false},
		{"agent_api_key", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", true}, // non-hex
		{"agent_ca_cert", "/path/to/ca.pem", false},
		{"agent_ca_cert", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			err := ValidateKey(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKey(%q, %q) error = %v, wantErr %v", tt.key, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestAgentConfigSetGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Set agent string keys.
	Set(path, "agent_server", "backup.internal:8420")
	Set(path, "agent_api_key", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	Set(path, "agent_ca_cert", "/etc/ssl/ca.pem")
	Set(path, "agent_hostname", "my-server")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentServer != "backup.internal:8420" {
		t.Errorf("AgentServer = %q", cfg.AgentServer)
	}
	if cfg.AgentAPIKey != "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" {
		t.Errorf("AgentAPIKey = %q", cfg.AgentAPIKey)
	}
	if cfg.AgentCACert != "/etc/ssl/ca.pem" {
		t.Errorf("AgentCACert = %q", cfg.AgentCACert)
	}
	if cfg.AgentHostname != "my-server" {
		t.Errorf("AgentHostname = %q", cfg.AgentHostname)
	}

	// GetConfigValue
	if v := GetConfigValue(cfg, "agent_server"); v != "backup.internal:8420" {
		t.Errorf("GetConfigValue agent_server = %q", v)
	}

	// IsZeroValue
	if IsZeroValue(cfg, "agent_server") {
		t.Error("agent_server should not be zero")
	}

	// Unset
	Unset(path, "agent_server")
	cfg, _ = Load(path)
	if cfg.AgentServer != "" {
		t.Errorf("AgentServer after unset = %q", cfg.AgentServer)
	}
	if !IsZeroValue(cfg, "agent_server") {
		t.Error("agent_server should be zero after unset")
	}
}

func TestAgentListConfigKeys(t *testing.T) {
	if !IsListKey("agent_paths") {
		t.Error("agent_paths should be a list key")
	}
	if !IsListKey("agent_excludes") {
		t.Error("agent_excludes should be a list key")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Add to agent_paths list.
	list, err := AddToList(path, "agent_paths", "/data")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0] != "/data" {
		t.Errorf("agent_paths = %v", list)
	}

	list, err = AddToList(path, "agent_paths", "/home")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("agent_paths = %v, want 2 entries", list)
	}

	// Remove from agent_paths.
	list, found, err := RemoveFromList(path, "agent_paths", "/data")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("found should be true")
	}
	if len(list) != 1 || list[0] != "/home" {
		t.Errorf("agent_paths after remove = %v", list)
	}

	// Add to agent_excludes.
	list, err = AddToList(path, "agent_excludes", "*.log")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0] != "*.log" {
		t.Errorf("agent_excludes = %v", list)
	}
}

func TestDefaultPath(t *testing.T) {
	p := DefaultPath()
	if p == "" {
		t.Error("DefaultPath returned empty string")
	}
	if !filepath.IsAbs(p) {
		// May not be absolute if both UserConfigDir and UserHomeDir fail,
		// but on most systems it should be.
	}
	if filepath.Base(p) != "config.yaml" {
		t.Errorf("DefaultPath base = %q, want config.yaml", filepath.Base(p))
	}
}

func TestValidateStaleThreshold(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"1", false},
		{"25", false},
		{"100", false},
		{"0", true},
		{"-1", true},
		{"abc", true},
		{"", true},
	}
	for _, tt := range tests {
		err := validateStaleThreshold(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateStaleThreshold(%q) err = %v, wantErr = %v", tt.input, err, tt.wantErr)
		}
	}
}

func TestSetConfigValueAllKeys(t *testing.T) {
	var cfg Config

	// String keys.
	stringKeys := map[string]string{
		"repo":                     "/tmp/repo",
		"password_file":            "/tmp/pass",
		"s3_access_key":            "AKID",
		"s3_secret_key":            "secret",
		"s3_region":                "eu-west-1",
		"serve_tls_cert":           "/cert.pem",
		"serve_tls_key":            "/key.pem",
		"serve_dashboard_password": "pass123",
		"alert_webhook_url":        "https://hook.example.com",
		"agent_server":             "localhost:8420",
		"agent_api_key":            "abc123",
		"agent_ca_cert":            "/ca.pem",
		"agent_hostname":           "myhost",
		"fs_scanner_plugin":        "custom-scan",
		"plugin_dir":               "/plugins",
	}
	for key, val := range stringKeys {
		SetConfigValue(&cfg, key, val)
		got := GetConfigValue(cfg, key)
		if got != val {
			t.Errorf("SetConfigValue(%q, %q): GetConfigValue = %q", key, val, got)
		}
	}

	// Int keys.
	intKeys := map[string]string{
		"parallel":              "4",
		"serve_port":            "9000",
		"serve_dashboard_port":  "9001",
		"alert_stale_threshold": "48",
	}
	for key, val := range intKeys {
		SetConfigValue(&cfg, key, val)
		got := GetConfigValue(cfg, key)
		if got != val {
			t.Errorf("SetConfigValue(%q, %q): GetConfigValue = %q", key, val, got)
		}
	}

	// List keys.
	listKeys := map[string]string{
		"exclude":                "*.log, *.tmp",
		"alert_notifier_plugins": "slack, pagerduty",
		"agent_paths":            "/data, /home",
		"agent_excludes":         "*.cache, *.bak",
		"hook_plugins":           "pre-check, post-notify",
	}
	for key, val := range listKeys {
		SetConfigValue(&cfg, key, val)
		got := GetConfigValue(cfg, key)
		if got != val {
			t.Errorf("SetConfigValue(%q, %q): GetConfigValue = %q", key, val, got)
		}
	}

	// Empty list value clears to nil.
	SetConfigValue(&cfg, "exclude", "")
	if len(cfg.Exclude) != 0 {
		t.Errorf("SetConfigValue(exclude, '') should clear list, got %v", cfg.Exclude)
	}
}

func TestZeroConfigValueAllKeys(t *testing.T) {
	// Populate all fields first.
	cfg := Config{
		Repo:                   "/repo",
		PasswordFile:           "/pass",
		S3AccessKey:            "key",
		S3SecretKey:            "secret",
		S3Region:               "us-east-1",
		Exclude:                []string{"*.log"},
		Parallel:               4,
		ServePort:              9000,
		ServeTLSCert:           "/cert",
		ServeTLSKey:            "/key",
		ServeDashboardPort:     9001,
		ServeDashboardPassword: "pass",
		AlertWebhookURL:        "https://hook",
		AlertStaleThreshold:    48,
		AlertNotifierPlugins:   []string{"slack"},
		AgentServer:            "localhost",
		AgentAPIKey:            "apikey",
		AgentCACert:            "/ca",
		AgentHostname:          "host",
		AgentPaths:             []string{"/data"},
		AgentExcludes:          []string{"*.tmp"},
		HookPlugins:            []string{"pre"},
		FsScannerPlugin:        "scan",
		PluginDir:              "/plugins",
	}

	// Zero every key and verify.
	for _, def := range Registry {
		ZeroConfigValue(&cfg, def.Name)
		if !IsZeroValue(cfg, def.Name) {
			t.Errorf("after ZeroConfigValue(%q), IsZeroValue = false", def.Name)
		}
	}
}

// --- Multi-repo config tests (Feature 012) ---

func TestRepoConfigYAMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := Config{
		Path:      path,
		ServePort: 8420,
		Repositories: []RepoConfig{
			{
				Name:         "main-local",
				Type:         "local",
				Path:         "/var/lib/hitstor/repo",
				PasswordFile: "/etc/hitstor/password.txt",
			},
			{
				Name:               "offsite-s3",
				Type:               "s3",
				Endpoint:           "https://s3.example.com",
				Bucket:             "hitstor-backup",
				Region:             "us-east-1",
				PasswordFile:       "/etc/hitstor/s3-pass.txt",
				AccessKeyEncrypted: "base64encryptedkey==",
				SecretKeyEncrypted: "base64encryptedsecret==",
			},
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Repositories) != 2 {
		t.Fatalf("Repositories count = %d, want 2", len(loaded.Repositories))
	}

	r0 := loaded.Repositories[0]
	if r0.Name != "main-local" || r0.Type != "local" || r0.Path != "/var/lib/hitstor/repo" {
		t.Errorf("repo 0: got %+v", r0)
	}
	if r0.PasswordFile != "/etc/hitstor/password.txt" {
		t.Errorf("repo 0 password_file = %q", r0.PasswordFile)
	}

	r1 := loaded.Repositories[1]
	if r1.Name != "offsite-s3" || r1.Type != "s3" {
		t.Errorf("repo 1: got %+v", r1)
	}
	if r1.Endpoint != "https://s3.example.com" || r1.Bucket != "hitstor-backup" {
		t.Errorf("repo 1 S3: endpoint=%q bucket=%q", r1.Endpoint, r1.Bucket)
	}
	if r1.AccessKeyEncrypted != "base64encryptedkey==" {
		t.Errorf("repo 1 access_key_encrypted = %q", r1.AccessKeyEncrypted)
	}
	if r1.SecretKeyEncrypted != "base64encryptedsecret==" {
		t.Errorf("repo 1 secret_key_encrypted = %q", r1.SecretKeyEncrypted)
	}
}

func TestRepoConfigLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write a legacy single-repo config (no repositories list).
	data := []byte("repo: /var/lib/hitstor/repo\npassword_file: /etc/hitstor/pass.txt\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Repositories) != 1 {
		t.Fatalf("Repositories count = %d, want 1 (legacy fallback)", len(cfg.Repositories))
	}

	r := cfg.Repositories[0]
	if r.Name != "default" {
		t.Errorf("legacy repo name = %q, want \"default\"", r.Name)
	}
	if r.Type != "local" {
		t.Errorf("legacy repo type = %q, want \"local\"", r.Type)
	}
	if r.Path != "/var/lib/hitstor/repo" {
		t.Errorf("legacy repo path = %q", r.Path)
	}
	if r.PasswordFile != "/etc/hitstor/pass.txt" {
		t.Errorf("legacy repo password_file = %q", r.PasswordFile)
	}
}

func TestRepoConfigLegacyS3Fallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	data := []byte("repo: s3:s3.example.com/hitstor-backup\npassword_file: /etc/hitstor/pass.txt\ns3_region: eu-west-1\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Repositories) != 1 {
		t.Fatalf("Repositories count = %d, want 1", len(cfg.Repositories))
	}

	r := cfg.Repositories[0]
	if r.Type != "s3" {
		t.Errorf("legacy S3 repo type = %q, want \"s3\"", r.Type)
	}
	if r.Endpoint != "https://s3.example.com" {
		t.Errorf("legacy S3 endpoint = %q", r.Endpoint)
	}
	if r.Bucket != "hitstor-backup" {
		t.Errorf("legacy S3 bucket = %q", r.Bucket)
	}
	if r.Region != "eu-west-1" {
		t.Errorf("legacy S3 region = %q", r.Region)
	}
}

func TestRepoConfigNoFallbackWhenReposExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write config with both repo field and repositories list.
	// The repositories list should take precedence (no fallback).
	data := []byte(`repo: /old/repo
repositories:
  - name: new-repo
    type: local
    path: /new/repo
    password_file: /pass.txt
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Repositories) != 1 {
		t.Fatalf("Repositories count = %d, want 1", len(cfg.Repositories))
	}
	if cfg.Repositories[0].Name != "new-repo" {
		t.Errorf("repo name = %q, want \"new-repo\"", cfg.Repositories[0].Name)
	}
}

func TestValidateRepoConfig(t *testing.T) {
	tests := []struct {
		name    string
		rc      RepoConfig
		wantErr bool
	}{
		{
			name:    "valid local",
			rc:      RepoConfig{Name: "main", Type: "local", Path: "/var/lib/repo", PasswordFile: "/pass.txt"},
			wantErr: false,
		},
		{
			name:    "valid s3",
			rc:      RepoConfig{Name: "offsite", Type: "s3", Endpoint: "https://s3.example.com", Bucket: "backup", PasswordFile: "/pass.txt"},
			wantErr: false,
		},
		{
			name:    "empty name",
			rc:      RepoConfig{Name: "", Type: "local", Path: "/repo", PasswordFile: "/pass.txt"},
			wantErr: true,
		},
		{
			name:    "invalid name chars",
			rc:      RepoConfig{Name: "bad name!", Type: "local", Path: "/repo", PasswordFile: "/pass.txt"},
			wantErr: true,
		},
		{
			name:    "name starting with hyphen",
			rc:      RepoConfig{Name: "-bad", Type: "local", Path: "/repo", PasswordFile: "/pass.txt"},
			wantErr: true,
		},
		{
			name:    "invalid type",
			rc:      RepoConfig{Name: "repo", Type: "ftp", Path: "/repo", PasswordFile: "/pass.txt"},
			wantErr: true,
		},
		{
			name:    "local missing path",
			rc:      RepoConfig{Name: "repo", Type: "local", PasswordFile: "/pass.txt"},
			wantErr: true,
		},
		{
			name:    "local relative path",
			rc:      RepoConfig{Name: "repo", Type: "local", Path: "relative/path", PasswordFile: "/pass.txt"},
			wantErr: true,
		},
		{
			name:    "s3 missing endpoint",
			rc:      RepoConfig{Name: "repo", Type: "s3", Bucket: "bucket", PasswordFile: "/pass.txt"},
			wantErr: true,
		},
		{
			name:    "s3 missing bucket",
			rc:      RepoConfig{Name: "repo", Type: "s3", Endpoint: "https://s3", PasswordFile: "/pass.txt"},
			wantErr: true,
		},
		{
			name:    "missing password_file",
			rc:      RepoConfig{Name: "repo", Type: "local", Path: "/repo"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepoConfig(tt.rc)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRepoConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRepositoriesDuplicateNames(t *testing.T) {
	repos := []RepoConfig{
		{Name: "repo-a", Type: "local", Path: "/a", PasswordFile: "/pass.txt"},
		{Name: "repo-a", Type: "local", Path: "/b", PasswordFile: "/pass.txt"},
	}
	if err := ValidateRepositories(repos); err == nil {
		t.Error("ValidateRepositories should fail on duplicate names")
	}
}

func TestValidateRepositoriesValid(t *testing.T) {
	repos := []RepoConfig{
		{Name: "repo-a", Type: "local", Path: "/a", PasswordFile: "/pass.txt"},
		{Name: "repo-b", Type: "s3", Endpoint: "https://s3", Bucket: "bkt", PasswordFile: "/pass.txt"},
	}
	if err := ValidateRepositories(repos); err != nil {
		t.Errorf("ValidateRepositories unexpected error: %v", err)
	}
}

func TestEncryptDecryptCredentialRoundTrip(t *testing.T) {
	var key [32]byte
	// Use a deterministic key for testing.
	for i := range key {
		key[i] = byte(i)
	}

	original := "my-secret-access-key-12345"
	encoded, err := EncryptCredential(key, original)
	if err != nil {
		t.Fatalf("EncryptCredential: %v", err)
	}
	if encoded == "" {
		t.Fatal("encoded should not be empty")
	}
	if encoded == original {
		t.Error("encoded should differ from plaintext")
	}

	decrypted, err := DecryptCredential(key, encoded)
	if err != nil {
		t.Fatalf("DecryptCredential: %v", err)
	}
	if decrypted != original {
		t.Errorf("decrypted = %q, want %q", decrypted, original)
	}
}

func TestEncryptCredentialPersistReload(t *testing.T) {
	var key [32]byte
	for i := range key {
		key[i] = byte(i + 10)
	}

	accessKey := "AKIAIOSFODNN7EXAMPLE"
	secretKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

	encAccess, err := EncryptCredential(key, accessKey)
	if err != nil {
		t.Fatal(err)
	}
	encSecret, err := EncryptCredential(key, secretKey)
	if err != nil {
		t.Fatal(err)
	}

	// Build a config with encrypted creds and save to YAML.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := Config{
		Path:   cfgPath,
		Exists: true,
		Repositories: []RepoConfig{
			{
				Name:               "s3-repo",
				Type:               "s3",
				Endpoint:           "https://s3.example.com",
				Bucket:             "test-bucket",
				PasswordFile:       "/tmp/pw",
				AccessKeyEncrypted: encAccess,
				SecretKeyEncrypted: encSecret,
			},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and decrypt.
	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Repositories) != 1 {
		t.Fatalf("repos count = %d, want 1", len(loaded.Repositories))
	}

	rc := loaded.Repositories[0]
	gotAccess, err := DecryptCredential(key, rc.AccessKeyEncrypted)
	if err != nil {
		t.Fatalf("decrypt access key: %v", err)
	}
	gotSecret, err := DecryptCredential(key, rc.SecretKeyEncrypted)
	if err != nil {
		t.Fatalf("decrypt secret key: %v", err)
	}

	if gotAccess != accessKey {
		t.Errorf("access key = %q, want %q", gotAccess, accessKey)
	}
	if gotSecret != secretKey {
		t.Errorf("secret key = %q, want %q", gotSecret, secretKey)
	}
}

func TestLoadStorageOptions(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "sync_writes: true\nmin_free_reserve_bytes: 5368709120\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SyncWrites {
		t.Error("SyncWrites = false, want true")
	}
	if cfg.MinFreeReserveBytes != 5*1024*1024*1024 {
		t.Errorf("MinFreeReserveBytes = %d, want 5GiB", cfg.MinFreeReserveBytes)
	}
}
