package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateEnvFileKeepsOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	old := "# written by the installer\r\nHITSTOR_AGENT_LISTEN_PORT=9422\r\nHITSTOR_AGENT_SERVER=old:8420\r\nHITSTOR_AGENT_CA_CERT=/old/ca.crt\r\nHITSTOR_MSSQL_PASSWORD=x\r\n"
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	set := map[string]string{"HITSTOR_AGENT_SERVER": "new:8420", "HITSTOR_AGENT_API_KEY": "abc"}
	if err := UpdateEnvFile(path, set, registrationEnvKeys); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "# written by the installer\nHITSTOR_AGENT_LISTEN_PORT=9422\nHITSTOR_MSSQL_PASSWORD=x\nHITSTOR_AGENT_SERVER=new:8420\nHITSTOR_AGENT_API_KEY=abc\n"
	if string(got) != want {
		t.Fatalf("agent.env =\n%s\nwant\n%s", got, want)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0600 && os.PathSeparator == '/' {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}

func TestUpdateEnvFileCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	set := map[string]string{"HITSTOR_AGENT_CA_CERT": "/c.crt", "HITSTOR_AGENT_API_KEY": "k", "HITSTOR_AGENT_SERVER": "s:1"}
	if err := UpdateEnvFile(path, set, registrationEnvKeys); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "HITSTOR_AGENT_SERVER=s:1\nHITSTOR_AGENT_API_KEY=k\nHITSTOR_AGENT_CA_CERT=/c.crt\n" {
		t.Fatalf("agent.env = %q", got)
	}
}
