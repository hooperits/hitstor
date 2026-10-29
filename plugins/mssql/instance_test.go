package main

import (
	"context"
	"strings"
	"testing"
)

func TestForInstance(t *testing.T) {
	base, _ := parseConfig(map[string]string{"auth": "sql", "username": "job", "password_env": "HITSTOR_TEST_PW"})

	// No saved settings: the job settings apply.
	c, err := base.forInstance("SQL2022", nil)
	if err != nil || c.Instance != "SQL2022" || c.Auth != "sql" || c.Username != "job" {
		t.Fatalf("unsaved = %+v, %v", c, err)
	}
	// Saved settings replace the job's credentials entirely.
	c, err = base.forInstance("MSSQLSERVER", map[string]string{})
	if err != nil || c.Auth != "integrated" || c.Username != "" {
		t.Fatalf("saved integrated = %+v, %v", c, err)
	}
	c, err = base.forInstance("MSSQLSERVER", map[string]string{"auth": "sql", "username": "hitstor", "password": "p@ss"})
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := c.dsn()
	if err != nil || !strings.Contains(dsn, "hitstor:p%40ss@localhost") {
		t.Fatalf("dsn = %s, %v", dsn, err)
	}
	c, _ = base.forInstance("MSSQLSERVER", map[string]string{"auth": "windows", "username": `CORP\svc-hitstor`, "password": "x"})
	if dsn, err := c.dsn(); err != nil || !strings.Contains(dsn, "CORP%5Csvc-hitstor:x@") {
		t.Fatalf("windows dsn = %s, %v", dsn, err)
	}
	if _, err := c.forInstance("I", map[string]string{"auth": "sql", "username": "u"}); err != nil {
		t.Fatal(err) // the password may come from the environment
	}
	c, _ = base.forInstance("I", map[string]string{"auth": "sql", "username": "u"})
	if _, err := c.dsn(); err == nil || !strings.Contains(err.Error(), "configure the instance") {
		t.Fatalf("missing password: %v", err)
	}

	for name, saved := range map[string]map[string]string{
		"windows without domain": {"auth": "windows", "username": "svc"},
		"sql without user":       {"auth": "sql"},
		"unknown key":            {"server": "x"},
		"bad auth":               {"auth": "kerberos"},
	} {
		if _, err := base.forInstance("I", saved); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSplitObject(t *testing.T) {
	cases := map[string][2]string{"MSSQLSERVER": {"MSSQLSERVER", ""}, "SQL2022/Sales": {"SQL2022", "Sales"}}
	for in, want := range cases {
		i, d, err := splitObject(in)
		if err != nil || i != want[0] || d != want[1] {
			t.Errorf("%s = %s %s %v", in, i, d, err)
		}
	}
	for _, bad := range []string{"", "/db", "a/b/c"} {
		if _, _, err := splitObject(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestConfigureObjectsAndInstanceList(t *testing.T) {
	p := &mssqlPlugin{}
	p.cfg, _ = parseConfig(nil)
	if err := p.ConfigureObjects(map[string]map[string]string{"MSSQLSERVER/db": {}}); err == nil {
		t.Fatal("database settings accepted")
	}
	if err := p.ConfigureObjects(map[string]map[string]string{"X": {"auth": "sql"}}); err == nil {
		t.Fatal("invalid settings accepted")
	}
	if err := p.ConfigureObjects(map[string]map[string]string{"Reports": {"auth": "integrated"}}); err != nil {
		t.Fatal(err)
	}
	// Outside Windows nothing is discovered: the saved instance is listed;
	// with none saved, the configured default.
	if got := p.instanceList(); len(got) != 1 || got[0].Name != "Reports" {
		t.Fatalf("instances = %+v", got)
	}
	p.ConfigureObjects(nil)
	got, err := p.Browse(context.Background(), "")
	if err != nil || len(got) != 1 || got[0].Path != defaultInstance || !got[0].Selectable || got[0].Kind != "instance" {
		t.Fatalf("root = %+v, %v", got, err)
	}
}

func TestResolveSelectionDedup(t *testing.T) {
	p := &mssqlPlugin{}
	got, excluded, err := p.resolveSelection(context.Background(), []string{"I/a", "i/A", "I/b"}, "full", true)
	if err != nil || len(got) != 2 || got[1].database != "b" || len(excluded) != 0 {
		t.Fatalf("targets = %+v, %v", got, err)
	}
}

func TestDetail(t *testing.T) {
	if got := joinDetail("FULL", formatSize(1395864371)); got != "FULL · 1.3 GiB" {
		t.Fatal(got)
	}
	if got := joinDetail("SIMPLE", formatSize(0)); got != "SIMPLE" {
		t.Fatal(got)
	}
}
