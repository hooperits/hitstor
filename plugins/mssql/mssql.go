package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginpkg"
)

// mssqlPlugin backs up and restores SQL Server databases through VDI.
// Objects are "<instance>/<database>".
type mssqlPlugin struct {
	manifest *pluginpkg.Manifest

	mu  sync.Mutex
	cfg config
	db  *sql.DB
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
	}, nil
}

func (p *mssqlPlugin) Configure(settings map[string]string) error {
	cfg, err := parseConfig(settings)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db != nil {
		p.db.Close()
		p.db = nil
	}
	p.cfg = cfg
	return nil
}

func (p *mssqlPlugin) conn(ctx context.Context) (*sql.DB, config, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db == nil {
		db, err := openDB(p.cfg)
		if err != nil {
			return nil, p.cfg, err
		}
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, p.cfg, fmt.Errorf("connect to SQL Server %s (%s): %w", p.cfg.Server, p.cfg.Instance, err)
		}
		p.db = db
	}
	return p.db, p.cfg, nil
}

// splitObject parses "<instance>/<database>" for the configured instance.
func splitObject(obj string, cfg config) (string, error) {
	inst, dbName, ok := strings.Cut(obj, "/")
	if !ok || dbName == "" {
		return "", fmt.Errorf("object %q must be <instance>/<database>", obj)
	}
	if !strings.EqualFold(inst, cfg.Instance) {
		return "", fmt.Errorf("object %q is on instance %q, but this agent is configured for %q", obj, inst, cfg.Instance)
	}
	return dbName, nil
}

func (p *mssqlPlugin) Browse(ctx context.Context, path string) ([]hitstorplugin.BrowseEntry, error) {
	db, cfg, err := p.conn(ctx)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return []hitstorplugin.BrowseEntry{{Path: cfg.Instance, Name: cfg.Instance, Kind: "instance", HasChildren: true}}, nil
	}
	if !strings.EqualFold(path, cfg.Instance) {
		return nil, fmt.Errorf("unknown instance %q", path)
	}
	dbs, err := listDatabases(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]hitstorplugin.BrowseEntry, 0, len(dbs))
	for _, d := range dbs {
		out = append(out, hitstorplugin.BrowseEntry{
			Path: cfg.Instance + "/" + d.Name, Name: d.Name, Kind: "database",
			Selectable: d.State == "ONLINE",
		})
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
	db, cfg, err := p.conn(ctx)
	if err != nil {
		return err
	}
	kind := req.Kind
	if kind == "" {
		kind = "full"
	}
	ext, ok := backupKinds[kind]
	if !ok {
		return fmt.Errorf("unknown backup kind %q (full, differential, log)", kind)
	}
	var o backupOptions
	if o.CopyOnly, err = boolOption(req.Options, "copy_only", false); err != nil {
		return err
	}
	if o.Compression, err = boolOption(req.Options, "compression", false); err != nil {
		return err
	}

	for _, obj := range req.Selection {
		dbName, err := splitObject(obj, cfg)
		if err != nil {
			return err
		}
		files, err := readFiles(ctx, db, dbName)
		if err != nil {
			return fmt.Errorf("%s: read files: %w", obj, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("database %q not found", dbName)
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
	db, cfg, err := p.conn(ctx)
	if err != nil {
		return "", err
	}
	source, err := splitObject(req.Object, cfg)
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
