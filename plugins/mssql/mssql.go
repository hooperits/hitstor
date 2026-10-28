package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginpkg"
)

// mssqlPlugin backs up and restores SQL Server databases through VDI.
// Objects are "<instance>" (every database of the instance, resolved at
// backup time) and "<instance>/<database>".
type mssqlPlugin struct {
	manifest *pluginpkg.Manifest

	mu  sync.Mutex
	cfg config // job settings; cfg.Instance is listed when none is discovered
	// instances holds the settings saved on the agent for each instance
	// (credentials), by upper-case instance name.
	instances map[string]instanceSettings
	dbs       map[string]*sql.DB // open connections by upper-case instance
}

type instanceSettings struct {
	name   string // as saved
	values map[string]string
}

// instanceInfo describes an installed instance.
type instanceInfo struct {
	Name    string
	Release string // e.g. "SQL Server 2022"
	Edition string
	Version string
}

func (p *mssqlPlugin) Metadata(context.Context) (hitstorplugin.WorkloadMetadata, error) {
	return hitstorplugin.WorkloadMetadata{
		ID:              p.manifest.ID,
		Name:            p.manifest.Name,
		Version:         p.manifest.Version,
		ProtocolVersion: hitstorplugin.WorkloadProtocolVersion,
		Description:     p.manifest.Description,
		Capabilities: []string{
			"browse", "backup.full", "backup.differential", "backup.log",
			"restore.point_in_time", "restore.rename",
		},
		Fields: fields,
	}, nil
}

func (p *mssqlPlugin) Configure(settings map[string]string) error {
	cfg, err := parseConfig(settings)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeAll()
	p.cfg = cfg
	return nil
}

// ConfigureObjects receives the instance settings saved on the agent.
func (p *mssqlPlugin) ConfigureObjects(objects map[string]map[string]string) error {
	instances := make(map[string]instanceSettings, len(objects))
	for path, values := range objects {
		if strings.Contains(path, "/") {
			return fmt.Errorf("object settings for %q: only instances take settings", path)
		}
		if _, err := p.cfg.forInstance(path, values); err != nil {
			return err
		}
		instances[strings.ToUpper(path)] = instanceSettings{name: path, values: values}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeAll()
	p.instances = instances
	return nil
}

func (p *mssqlPlugin) closeAll() {
	for _, db := range p.dbs {
		db.Close()
	}
	p.dbs = nil
}

// conn returns the connection to an instance, opened with its settings.
func (p *mssqlPlugin) conn(ctx context.Context, instance string) (*sql.DB, config, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := strings.ToUpper(instance)
	cfg, err := p.cfg.forInstance(instance, p.instances[key].values)
	if err != nil {
		return nil, cfg, err
	}
	if db := p.dbs[key]; db != nil {
		return db, cfg, nil
	}
	db, err := openDB(cfg)
	if err != nil {
		return nil, cfg, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, cfg, fmt.Errorf("connect to SQL Server instance %s as %s: %w", instance, cfg.loginName(), err)
	}
	if p.dbs == nil {
		p.dbs = map[string]*sql.DB{}
	}
	p.dbs[key] = db
	return db, cfg, nil
}

// splitObject parses "<instance>" or "<instance>/<database>".
func splitObject(obj string) (instance, database string, err error) {
	instance, database, _ = strings.Cut(obj, "/")
	if instance == "" || strings.Contains(database, "/") {
		return "", "", fmt.Errorf("object %q must be <instance> or <instance>/<database>", obj)
	}
	return instance, database, nil
}

// instanceList returns the instances to show: those installed on this host,
// plus any with saved settings, or the configured one when none is found.
func (p *mssqlPlugin) instanceList() []instanceInfo {
	found := localInstances()
	seen := map[string]bool{}
	for _, i := range found {
		seen[strings.ToUpper(i.Name)] = true
	}
	p.mu.Lock()
	for key, s := range p.instances {
		if !seen[key] {
			found = append(found, instanceInfo{Name: s.name})
			seen[key] = true
		}
	}
	if len(found) == 0 {
		found = append(found, instanceInfo{Name: p.cfg.Instance})
	}
	p.mu.Unlock()
	sort.Slice(found, func(i, j int) bool {
		// The default instance first, then by name.
		di, dj := strings.EqualFold(found[i].Name, defaultInstance), strings.EqualFold(found[j].Name, defaultInstance)
		if di != dj {
			return di
		}
		return strings.ToUpper(found[i].Name) < strings.ToUpper(found[j].Name)
	})
	return found
}

func (p *mssqlPlugin) Browse(ctx context.Context, path string) ([]hitstorplugin.BrowseEntry, error) {
	if path == "" {
		var out []hitstorplugin.BrowseEntry
		for _, i := range p.instanceList() {
			out = append(out, hitstorplugin.BrowseEntry{
				Path: i.Name, Name: i.Name, Kind: "instance", Selectable: true, HasChildren: true,
				Detail: joinDetail(i.Release, strings.TrimSuffix(i.Edition, " Edition"), i.Version),
			})
		}
		return out, nil
	}
	instance, database, err := splitObject(path)
	if err != nil {
		return nil, err
	}
	if database != "" {
		return nil, nil
	}
	db, _, err := p.conn(ctx, instance)
	if err != nil {
		return nil, err
	}
	dbs, err := listDatabases(ctx, db)
	if err != nil {
		return nil, err
	}
	// User databases first, then the system ones.
	sort.SliceStable(dbs, func(i, j int) bool { return !systemDatabases[dbs[i].Name] && systemDatabases[dbs[j].Name] })
	out := make([]hitstorplugin.BrowseEntry, 0, len(dbs))
	for _, d := range dbs {
		kind := "database"
		if systemDatabases[d.Name] {
			kind = "system_database"
		}
		e := hitstorplugin.BrowseEntry{
			Path: instance + "/" + d.Name, Name: d.Name, Kind: kind,
			Selectable: d.State == "ONLINE",
			Detail:     joinDetail(d.RecoveryModel, formatSize(d.Size)),
		}
		if d.State != "ONLINE" {
			e.Status = d.State
		}
		out = append(out, e)
	}
	return out, nil
}

func joinDetail(parts ...string) string {
	var kept []string
	for _, s := range parts {
		if s != "" {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, " · ")
}

func formatSize(b int64) string {
	switch {
	case b <= 0:
		return ""
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MiB", float64(b)/(1<<20))
	}
	return fmt.Sprintf("%.0f KiB", float64(b)/(1<<10))
}

// backupTarget is one database to back up.
type backupTarget struct {
	instance, database string
}

// resolveSelection expands instances into their databases and drops
// duplicates. An instance stands for every online database that supports
// the backup kind: differentials skip master, and log backups skip
// databases in the SIMPLE recovery model. Databases selected one by one are
// kept as they are, so an unsupported kind fails loudly.
func (p *mssqlPlugin) resolveSelection(ctx context.Context, selection []string, kind string) ([]backupTarget, error) {
	var out []backupTarget
	seen := map[string]bool{}
	add := func(t backupTarget) {
		key := strings.ToUpper(t.instance + "/" + t.database)
		if !seen[key] {
			seen[key] = true
			out = append(out, t)
		}
	}
	for _, obj := range selection {
		instance, database, err := splitObject(obj)
		if err != nil {
			return nil, err
		}
		if database != "" {
			add(backupTarget{instance, database})
			continue
		}
		db, _, err := p.conn(ctx, instance)
		if err != nil {
			return nil, err
		}
		dbs, err := listDatabases(ctx, db)
		if err != nil {
			return nil, fmt.Errorf("%s: list databases: %w", instance, err)
		}
		n := 0
		for _, d := range dbs {
			skip := ""
			switch {
			case d.State != "ONLINE":
				skip = "database is " + d.State
			case kind == "differential" && d.Name == "master":
				skip = "master takes only full backups"
			case kind == "log" && d.RecoveryModel == "SIMPLE":
				skip = "SIMPLE recovery model has no log backups"
			}
			if skip != "" {
				slog.Info("database skipped", "instance", instance, "database", d.Name, "kind", kind, "reason", skip)
				continue
			}
			add(backupTarget{instance, d.Name})
			n++
		}
		if n == 0 {
			return nil, fmt.Errorf("instance %s has no databases that take %s backups", instance, kind)
		}
	}
	return out, nil
}

func boolOption(opts map[string]string, key string, def bool) (bool, error) {
	v, ok := opts[key]
	if !ok || v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("option %s: %w", key, err)
	}
	return b, nil
}

func (p *mssqlPlugin) Backup(ctx context.Context, req hitstorplugin.BackupRequest, sink hitstorplugin.ItemSink) error {
	kind := req.Kind
	if kind == "" {
		kind = "full"
	}
	ext, ok := backupKinds[kind]
	if !ok {
		return fmt.Errorf("unknown backup kind %q (full, differential, log)", kind)
	}
	var o backupOptions
	var err error
	if o.CopyOnly, err = boolOption(req.Options, "copy_only", false); err != nil {
		return err
	}
	if o.Compression, err = boolOption(req.Options, "compression", false); err != nil {
		return err
	}

	targets, err := p.resolveSelection(ctx, req.Selection, kind)
	if err != nil {
		return err
	}
	for _, t := range targets {
		obj, dbName := t.instance+"/"+t.database, t.database
		db, cfg, err := p.conn(ctx, t.instance)
		if err != nil {
			return err
		}
		files, err := readFiles(ctx, db, dbName)
		if err != nil {
			return fmt.Errorf("%s: read files: %w", obj, err)
		}
		if len(files) == 0 {
			// Without VIEW ANY DEFINITION or access to the database, the
			// login cannot see it either; name the login to make that clear.
			var login string
			if err := db.QueryRowContext(ctx, `SELECT SUSER_SNAME()`).Scan(&login); err != nil {
				login = "?"
			}
			return fmt.Errorf("database %q not found or not visible to login %q (check auth/username and the login's permissions)", dbName, login)
		}

		// Milliseconds: log backups can be seconds apart.
		stamp := time.Now().UTC().Format("20060102T150405.000Z")
		w, err := sink.Item(fmt.Sprintf("%s/%s/%s-%s.%s", cfg.Instance, dbName, kind, stamp, ext))
		if err != nil {
			return err
		}
		var device string
		start := time.Now()
		var written int64
		err = withVirtualDevice(ctx, cfg.vdiInstance(),
			func(ctx context.Context, dev string) error {
				device = dev
				stmt, err := backupStatement(kind, dbName, dev, o)
				if err != nil {
					return err
				}
				_, err = db.ExecContext(ctx, stmt)
				return err
			},
			func(dev vdDevice) error {
				n, err := backupTransfer(dev, w)
				written = n
				return err
			})
		if err != nil {
			return fmt.Errorf("%s: %s backup: %w", obj, kind, err)
		}
		meta, err := readBackupSet(ctx, db, device)
		if err != nil {
			return fmt.Errorf("%s: %w", obj, err)
		}
		meta.Instance = cfg.Instance
		meta.Files = files
		raw, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		if err := w.Finish(raw); err != nil {
			return err
		}
		slog.Info("database backed up", "object", obj, "kind", kind, "bytes", written,
			"duration", time.Since(start).Round(time.Millisecond), "last_lsn", meta.LastLSN)
	}
	return nil
}

func (p *mssqlPlugin) PlanRestore(_ context.Context, req hitstorplugin.PlanRestoreRequest) ([]string, error) {
	return planRestore(req.Target, req.Candidates)
}

// Restore options:
//
//	database   name to restore as (default: the original name)
//	replace    overwrite an existing database (default false)
//	recovery   bring the database online at the end (default true)
//	data_path, log_path   where to put files when relocating (default: instance defaults)
//	relocate   move files even when keeping the name (default: only when renaming)
func (p *mssqlPlugin) Restore(ctx context.Context, req hitstorplugin.RestoreRequest, items hitstorplugin.RestoreItems) (string, error) {
	instance, source, err := splitObject(req.Object)
	if err != nil {
		return "", err
	}
	if source == "" {
		return "", fmt.Errorf("object %q must be <instance>/<database>", req.Object)
	}
	db, cfg, err := p.conn(ctx, instance)
	if err != nil {
		return "", err
	}
	o := restoreOptions{Database: req.Options["database"], DataPath: req.Options["data_path"], LogPath: req.Options["log_path"]}
	if o.Database == "" {
		o.Database = source
	}
	if o.Replace, err = boolOption(req.Options, "replace", false); err != nil {
		return "", err
	}
	if o.Recovery, err = boolOption(req.Options, "recovery", true); err != nil {
		return "", err
	}
	if o.Relocate, err = boolOption(req.Options, "relocate", !strings.EqualFold(o.Database, source)); err != nil {
		return "", err
	}
	if o.Relocate && (o.DataPath == "" || o.LogPath == "") {
		data, log, err := defaultPaths(ctx, db)
		if err != nil {
			return "", fmt.Errorf("read default file paths: %w", err)
		}
		if o.DataPath == "" {
			o.DataPath = data
		}
		if o.LogPath == "" {
			o.LogPath = log
		}
	}

	var applied []string
	var lastMeta itemMeta
	stopped := false
	for i := 0; ; i++ {
		it, err := items.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		var meta itemMeta
		if err := json.Unmarshal(it.Metadata, &meta); err != nil || meta.Type == "" {
			return "", fmt.Errorf("item %s has no SQL Server backup metadata", it.Item)
		}
		first, last := i == 0, i == req.ItemCount-1
		var moves []string
		if first && o.Relocate {
			moves = moveClauses(meta.Files, o.Database, o.DataPath, o.LogPath)
		}
		var stopAt time.Time
		if last && meta.Type == typeLog && !req.Target.IsZero() && meta.Finish.After(req.Target) {
			stopAt = req.Target
			stopped = true
		}
		err = withVirtualDevice(ctx, cfg.vdiInstance(),
			func(ctx context.Context, dev string) error {
				stmt := restoreStatement(meta, dev, o, moves, first, last, stopAt)
				if stopAt.IsZero() {
					_, err := db.ExecContext(ctx, stmt)
					return err
				}
				_, err := db.ExecContext(ctx, stmt, stopAt)
				return err
			},
			func(dev vdDevice) error {
				_, err := restoreTransfer(dev, it.Data)
				return err
			})
		if err != nil {
			return "", fmt.Errorf("restore %s: %w", it.Item, err)
		}
		applied = append(applied, it.Item)
		lastMeta = meta
		slog.Info("backup applied", "item", it.Item, "database", o.Database)
	}
	if len(applied) == 0 {
		return "", fmt.Errorf("nothing to restore")
	}

	point := lastMeta.Finish.Format(time.RFC3339)
	if stopped {
		point = req.Target.UTC().Format(time.RFC3339)
	}
	state := "online"
	if !o.Recovery {
		state = "left in RESTORING state"
	}
	return fmt.Sprintf("restored %s as %s to %s from %d backup(s); database %s", source, o.Database, point, len(applied), state), nil
}
