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
		{Logical: "App", Type: "ROWS"}, {Logical: "App_log", Type: "LOG"}, {Logical: "App_idx", Type: "ROWS"},
	}
	got := strings.Join(moveClauses(files, "App_copy", `D:\Data`, `L:\Logs\`), "; ")
	want := `MOVE N'App' TO N'D:\Data\App_copy.mdf'; MOVE N'App_log' TO N'L:\Logs\App_copy_log.ldf'; MOVE N'App_idx' TO N'D:\Data\App_copy_App_idx.ndf'`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
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
