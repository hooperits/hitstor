package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

// absWinPath matches C:\... and \\server\share\... paths.
var absWinPath = regexp.MustCompile(`^(?:[A-Za-z]:\\|\\\\[^\\]+\\[^\\]+\\).+`)

// parseRestoreOptions reads the restore options for a backup of source on
// instance.
func parseRestoreOptions(opts map[string]string, instance, source string) (restoreOptions, error) {
	o := restoreOptions{Instance: opts["instance"], Database: opts["database"], DataPath: opts["data_path"], LogPath: opts["log_path"]}
	if o.Instance == "" {
		o.Instance = instance
	}
	if o.Database == "" {
		o.Database = source
	}
	var err error
	if o.Replace, err = boolOption(opts, "replace", false); err != nil {
		return o, err
	}
	if o.Recovery, err = boolOption(opts, "recovery", true); err != nil {
		return o, err
	}
	// A copy under another name or on another instance of the same host
	// must not reuse the original's files.
	moved := !strings.EqualFold(o.Database, source) || !strings.EqualFold(o.Instance, instance)
	if o.Relocate, err = boolOption(opts, "relocate", moved); err != nil {
		return o, err
	}
	for k, v := range opts {
		name, ok := strings.CutPrefix(k, "file:")
		if !ok || v == "" {
			continue
		}
		if !absWinPath.MatchString(v) {
			return o, fmt.Errorf("file %s: %q is not a full path (C:\\... or \\\\server\\share\\...)", name, v)
		}
		if o.Files == nil {
			o.Files = map[string]string{}
		}
		o.Files[name] = v
	}
	return o, nil
}

// restoreSetup connects to the instance a restore of object goes to and
// works out its options, including the default file paths when relocating.
func (p *mssqlPlugin) restoreSetup(ctx context.Context, object string, opts map[string]string) (*sql.DB, config, string, restoreOptions, error) {
	instance, source, err := splitObject(object)
	if err != nil {
		return nil, config{}, "", restoreOptions{}, err
	}
	if source == "" {
		return nil, config{}, "", restoreOptions{}, fmt.Errorf("object %q must be <instance>/<database>", object)
	}
	o, err := parseRestoreOptions(opts, instance, source)
	if err != nil {
		return nil, config{}, source, o, err
	}
	db, cfg, err := p.conn(ctx, o.Instance)
	if err != nil {
		return nil, cfg, source, o, err
	}
	if o.Relocate && (o.DataPath == "" || o.LogPath == "") {
		data, log, err := defaultPaths(ctx, db)
		if err != nil {
			return nil, cfg, source, o, fmt.Errorf("read default file paths of %s: %w", o.Instance, err)
		}
		if o.DataPath == "" {
			o.DataPath = data
		}
		if o.LogPath == "" {
			o.LogPath = log
		}
	}
	return db, cfg, source, o, nil
}

// PreviewRestore tells the operator where a planned restore writes its
// files, and whether it would collide with what is on the instance.
func (p *mssqlPlugin) PreviewRestore(ctx context.Context, req hitstorplugin.PlanRestoreRequest, refs []string) (hitstorplugin.RestorePreview, error) {
	msg := messagesFor(req.Language)
	var first, last itemMeta
	for _, c := range req.Candidates {
		var m itemMeta
		if c.Ref == refs[0] && json.Unmarshal(c.Metadata, &m) == nil {
			first = m
		}
		if c.Ref == refs[len(refs)-1] && json.Unmarshal(c.Metadata, &m) == nil {
			last = m
		}
	}
	db, cfg, source, o, err := p.restoreSetup(ctx, req.Object, req.Options)
	if err != nil {
		return hitstorplugin.RestorePreview{}, err
	}
	var pv hitstorplugin.RestorePreview
	pv.Files = fileTargets(first.Files, o)

	point := last.Finish
	if !req.Target.IsZero() && last.Type == typeLog && last.Finish.After(req.Target) {
		point = req.Target
	}
	state := msg.online
	if !o.Recovery {
		state = msg.restoring
	}
	pv.Summary = fmt.Sprintf(msg.summary, source, o.Database, cfg.Instance, point.Local().Format("2006-01-02 15:04:05"), len(refs), state)

	var dbState string
	switch err := db.QueryRowContext(ctx, `SELECT state_desc FROM sys.databases WHERE name = @p1`, o.Database).Scan(&dbState); {
	case err == sql.ErrNoRows:
	case err != nil:
		pv.Warnings = append(pv.Warnings, fmt.Sprintf(msg.checkFailed, err))
	case o.Replace:
		pv.Warnings = append(pv.Warnings, fmt.Sprintf(msg.willReplace, o.Database, cfg.Instance))
	default:
		pv.Warnings = append(pv.Warnings, fmt.Sprintf(msg.exists, o.Database, cfg.Instance))
	}

	// Files another database already uses make RESTORE fail.
	rows, err := db.QueryContext(ctx, `SELECT DB_NAME(database_id), physical_name FROM sys.master_files`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var owner, phys string
			if rows.Scan(&owner, &phys) != nil {
				continue
			}
			if strings.EqualFold(owner, o.Database) && o.Replace {
				continue
			}
			for _, f := range pv.Files {
				if strings.EqualFold(f.Target, phys) {
					pv.Warnings = append(pv.Warnings, fmt.Sprintf(msg.fileInUse, f.Target, owner))
				}
			}
		}
	}
	return pv, nil
}

type previewMessages struct {
	summary, online, restoring, checkFailed, willReplace, exists, fileInUse string
}

func messagesFor(lang string) previewMessages {
	if strings.HasPrefix(lang, "es") {
		return previewMessages{
			summary:     "Restaura %s como %s en %s, al %s, aplicando %d respaldo(s); la base quedará %s.",
			online:      "en línea",
			restoring:   "en estado RESTORING",
			checkFailed: "No se pudo comprobar si la base existe: %v",
			willReplace: "La base %s ya existe en %s y se sobrescribirá.",
			exists:      "La base %s ya existe en %s: restaure con otro nombre o active Reemplazar para sobrescribirla.",
			fileInUse:   "El archivo %s lo usa la base %s: elija otra ruta.",
		}
	}
	return previewMessages{
		summary:     "Restores %s as %s on %s, as of %s, applying %d backup(s); the database will be %s.",
		online:      "online",
		restoring:   "left RESTORING",
		checkFailed: "Could not check whether the database exists: %v",
		willReplace: "Database %s exists on %s and will be overwritten.",
		exists:      "Database %s already exists on %s: restore under another name, or turn on Replace to overwrite it.",
		fileInUse:   "File %s is used by database %s: choose another path.",
	}
}
