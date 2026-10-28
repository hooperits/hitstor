package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	c, err := parseConfig(nil)
	if err != nil || c.Instance != defaultInstance || c.Auth != "integrated" || c.vdiInstance() != "" {
		t.Fatalf("defaults = %+v, %v", c, err)
	}
	c, err = parseConfig(map[string]string{"instance": "SQL2022", "auth": "sql", "username": "hitstor"})
	if err != nil || c.vdiInstance() != "SQL2022" {
		t.Fatalf("named = %+v, %v", c, err)
	}
	for name, s := range map[string]map[string]string{
		"password in job settings": {"password": "x"},
		"unknown key":              {"pasword_env": "X"},
		"sql without user":         {"auth": "sql"},
		"bad auth":                 {"auth": "kerberos"},
		"bad bool":                 {"trust_server_certificate": "maybe"},
	} {
		if _, err := parseConfig(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestDSN(t *testing.T) {
	c, _ := parseConfig(map[string]string{"instance": "SQL2022"})
	dsn, err := c.dsn()
	if err != nil || !strings.HasPrefix(dsn, "sqlserver://localhost/SQL2022?") || strings.Contains(dsn, "@") {
		t.Fatalf("dsn = %s, %v", dsn, err)
	}
	c, _ = parseConfig(map[string]string{"auth": "sql", "username": "hitstor", "password_env": "HITSTOR_TEST_PW"})
	if _, err := c.dsn(); err == nil {
		t.Fatal("missing password env accepted")
	}
	t.Setenv("HITSTOR_TEST_PW", "p@ss'word")
	dsn, err = c.dsn()
	if err != nil || !strings.Contains(dsn, "hitstor:p%40ss%27word@localhost") {
		t.Fatalf("dsn = %s, %v", dsn, err)
	}
}

func TestQuoting(t *testing.T) {
	if got := quoteName("we]ird"); got != "[we]]ird]" {
		t.Fatal(got)
	}
	if got := quoteString("o'brien"); got != "N'o''brien'" {
		t.Fatal(got)
	}
}

func TestBackupStatement(t *testing.T) {
	cases := map[string]struct {
		kind string
		o    backupOptions
		want string
	}{
		"full":      {"full", backupOptions{}, "BACKUP DATABASE [db] TO VIRTUAL_DEVICE = N'vd' WITH CHECKSUM, NAME = N'HitStor full', NO_COMPRESSION"},
		"copy only": {"full", backupOptions{CopyOnly: true, Compression: true}, "BACKUP DATABASE [db] TO VIRTUAL_DEVICE = N'vd' WITH CHECKSUM, NAME = N'HitStor full', COPY_ONLY, COMPRESSION"},
		"diff":      {"differential", backupOptions{}, "BACKUP DATABASE [db] TO VIRTUAL_DEVICE = N'vd' WITH CHECKSUM, NAME = N'HitStor differential', DIFFERENTIAL, NO_COMPRESSION"},
		"log":       {"log", backupOptions{}, "BACKUP LOG [db] TO VIRTUAL_DEVICE = N'vd' WITH CHECKSUM, NAME = N'HitStor log', NO_COMPRESSION"},
		"copy log":  {"log", backupOptions{CopyOnly: true}, "BACKUP LOG [db] TO VIRTUAL_DEVICE = N'vd' WITH CHECKSUM, NAME = N'HitStor log', COPY_ONLY, NO_COMPRESSION"},
	}
	for name, c := range cases {
		got, err := backupStatement(c.kind, "db", "vd", c.o)
		if err != nil || got != c.want {
			t.Errorf("%s:\n got %s (%v)\nwant %s", name, got, err, c.want)
		}
	}
	if _, err := backupStatement("differential", "db", "vd", backupOptions{CopyOnly: true}); err == nil {
		t.Error("copy-only differential accepted")
	}
	if _, err := backupStatement("snapshot", "db", "vd", backupOptions{}); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestMoveClauses(t *testing.T) {
	files := []backupFile{
		{Logical: "App", Type: "ROWS", Physical: `C:\SQL\App.mdf`},
		{Logical: "App_log", Type: "LOG", Physical: `C:\SQL\App_log.ldf`},
		{Logical: "App_idx", Type: "ROWS", Physical: `C:\SQL\App_idx.ndf`},
	}
	o := restoreOptions{Database: "App_copy", Relocate: true, DataPath: `D:\Data`, LogPath: `L:\Logs\`}
	got := strings.Join(moveClauses(fileTargets(files, o)), "; ")
	want := `MOVE N'App' TO N'D:\Data\App_copy.mdf'; MOVE N'App_log' TO N'L:\Logs\App_copy_log.ldf'; MOVE N'App_idx' TO N'D:\Data\App_copy_App_idx.ndf'`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}

	// A path set for one file wins; files left where they were need no MOVE.
	o = restoreOptions{Database: "App", Files: map[string]string{"App_log": `E:\Logs\App_log.ldf`}}
	if got := strings.Join(moveClauses(fileTargets(files, o)), "; "); got != `MOVE N'App_log' TO N'E:\Logs\App_log.ldf'` {
		t.Fatalf("override: %s", got)
	}
}

func TestParseRestoreOptions(t *testing.T) {
	o, err := parseRestoreOptions(map[string]string{}, "MSSQLSERVER", "Sales")
	if err != nil || o.Database != "Sales" || o.Instance != "MSSQLSERVER" || o.Relocate || !o.Recovery {
		t.Fatalf("defaults = %+v, %v", o, err)
	}
	// Another name or another instance relocates by default.
	if o, _ := parseRestoreOptions(map[string]string{"database": "Sales_copy"}, "MSSQLSERVER", "Sales"); !o.Relocate {
		t.Fatal("rename does not relocate")
	}
	if o, _ := parseRestoreOptions(map[string]string{"instance": "SQL2022"}, "MSSQLSERVER", "Sales"); !o.Relocate || o.Instance != "SQL2022" {
		t.Fatalf("other instance = %+v", o)
	}
	o, err = parseRestoreOptions(map[string]string{"file:Sales": `D:\Data\S.mdf`, "file:Sales_log": `\\nas\sql\S.ldf`}, "I", "Sales")
	if err != nil || o.Files["Sales"] != `D:\Data\S.mdf` || o.Files["Sales_log"] != `\\nas\sql\S.ldf` {
		t.Fatalf("files = %+v, %v", o.Files, err)
	}
	for _, bad := range []string{`relative\S.mdf`, `S.mdf`, `D:`} {
		if _, err := parseRestoreOptions(map[string]string{"file:Sales": bad}, "I", "Sales"); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestRestoreStatement(t *testing.T) {
	o := restoreOptions{Database: "App_copy", Recovery: true}
	full := itemMeta{Type: typeFull}
	logm := itemMeta{Type: typeLog}
	moves := []string{"MOVE N'App' TO N'D:\\App_copy.mdf'"}

	got := restoreStatement(full, "vd", o, moves, true, false, time.Time{})
	if got != `RESTORE DATABASE [App_copy] FROM VIRTUAL_DEVICE = N'vd' WITH MOVE N'App' TO N'D:\App_copy.mdf', NORECOVERY` {
		t.Fatalf("first: %s", got)
	}
	got = restoreStatement(full, "vd", o, moves, true, true, time.Time{})
	if !strings.HasSuffix(got, "MOVE N'App' TO N'D:\\App_copy.mdf', RECOVERY") {
		t.Fatalf("single: %s", got)
	}
	got = restoreStatement(logm, "vd", o, nil, false, true, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if !strings.HasPrefix(got, "DECLARE @stopat datetime2") || !strings.HasSuffix(got, "RESTORE LOG [App_copy] FROM VIRTUAL_DEVICE = N'vd' WITH RECOVERY, STOPAT = @stopat") {
		t.Fatalf("stopat: %s", got)
	}
	o.Replace, o.Recovery = true, false
	got = restoreStatement(full, "vd", o, nil, true, true, time.Time{})
	if got != `RESTORE DATABASE [App_copy] FROM VIRTUAL_DEVICE = N'vd' WITH REPLACE, NORECOVERY` {
		t.Fatalf("replace/norecovery: %s", got)
	}
}
