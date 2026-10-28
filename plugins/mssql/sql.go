package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb" // registers "sqlserver"; Windows integrated auth (SSPI) built in
)

// defaultInstance is SQL Server's name for the unnamed instance.
const defaultInstance = "MSSQLSERVER"

// config holds the plugin settings.
type config struct {
	Server      string // host the agent connects to; default localhost
	Instance    string // instance name; default MSSQLSERVER
	Auth        string // "integrated" (default), "windows", or "sql"
	Username    string // SQL login, or DOMAIN\user with auth=windows
	Password    string // from the instance settings saved on the agent
	PasswordEnv string // environment variable holding the password otherwise
	Encrypt     string // go-mssqldb encrypt value: "true" (default), "false", "disable", "strict"
	TrustCert   bool   // trust the server certificate (default true for localhost)
}

func parseConfig(s map[string]string) (config, error) {
	c := config{
		Server:      "localhost",
		Instance:    defaultInstance,
		Auth:        "integrated",
		PasswordEnv: "HITSTOR_MSSQL_PASSWORD",
		Encrypt:     "true",
		TrustCert:   true,
	}
	for k, v := range s {
		switch k {
		case "server":
			c.Server = v
		case "instance":
			c.Instance = v
		case "auth":
			c.Auth = v
		case "username":
			c.Username = v
		case "password_env":
			c.PasswordEnv = v
		case "encrypt":
			c.Encrypt = v
		case "trust_server_certificate":
			b, err := strconv.ParseBool(v)
			if err != nil {
				return c, fmt.Errorf("trust_server_certificate: %w", err)
			}
			c.TrustCert = b
		case "password":
			return c, fmt.Errorf("never put the password in job settings: save it with the instance's settings on the agent (Configure in the dashboard, or hitstor-agent plugin configure)")
		default:
			return c, fmt.Errorf("unknown setting %q", k)
		}
	}
	if c.Instance == "" {
		c.Instance = defaultInstance
	}
	return c, c.checkAuth()
}

func (c config) checkAuth() error {
	switch c.Auth {
	case "integrated":
	case "sql":
		if c.Username == "" {
			return fmt.Errorf("auth=sql needs username")
		}
	case "windows":
		if !strings.Contains(c.Username, `\`) {
			return fmt.Errorf(`auth=windows needs username as DOMAIN\user (.\user for a local account)`)
		}
	default:
		return fmt.Errorf("auth must be integrated, windows, or sql, got %q", c.Auth)
	}
	return nil
}

// forInstance returns the settings for one instance: the job settings with
// the instance's own settings (object settings saved on the agent) on top.
func (c config) forInstance(instance string, saved map[string]string) (config, error) {
	c.Instance = instance
	if saved == nil {
		return c, nil
	}
	c.Auth, c.Username, c.Password = "integrated", "", ""
	for k, v := range saved {
		switch k {
		case "auth":
			c.Auth = v
		case "username":
			c.Username = v
		case "password":
			c.Password = v
		default:
			return c, fmt.Errorf("instance %s: unknown setting %q", instance, k)
		}
	}
	if c.Auth == "" {
		c.Auth = "integrated"
	}
	if err := c.checkAuth(); err != nil {
		return c, fmt.Errorf("instance %s: %w", instance, err)
	}
	return c, nil
}

// loginName describes who the plugin connects as, for error messages.
func (c config) loginName() string {
	if c.Auth == "integrated" {
		return "the agent's Windows account"
	}
	return c.Username
}

// vdiInstance is the instance name VDI expects: empty for the default one.
func (c config) vdiInstance() string {
	if strings.EqualFold(c.Instance, defaultInstance) {
		return ""
	}
	return c.Instance
}

func (c config) dsn() (string, error) {
	u := &url.URL{Scheme: "sqlserver", Host: c.Server}
	if c.vdiInstance() != "" {
		u.Path = c.Instance
	}
	q := url.Values{}
	q.Set("database", "master")
	q.Set("app name", "HitStor")
	q.Set("encrypt", c.Encrypt)
	q.Set("TrustServerCertificate", strconv.FormatBool(c.TrustCert))
	if c.Auth == "sql" || c.Auth == "windows" {
		pw := c.Password
		if pw == "" {
			var ok bool
			if pw, ok = os.LookupEnv(c.PasswordEnv); !ok {
				return "", fmt.Errorf("instance %s: auth=%s needs a password: configure the instance", c.Instance, c.Auth)
			}
		}
		// A DOMAIN\user name makes the driver use Windows authentication
		// (SSPI) with these credentials instead of a SQL login.
		u.User = url.UserPassword(c.Username, pw)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func openDB(c config) (*sql.DB, error) {
	dsn, err := c.dsn()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, err
	}
	// One connection runs BACKUP/RESTORE while another reads msdb.
	db.SetMaxOpenConns(4)
	db.SetConnMaxIdleTime(time.Minute)
	return db, nil
}

// quoteName quotes an identifier like T-SQL QUOTENAME.
func quoteName(s string) string { return "[" + strings.ReplaceAll(s, "]", "]]") + "]" }

// quoteString quotes a Unicode string literal.
func quoteString(s string) string { return "N'" + strings.ReplaceAll(s, "'", "''") + "'" }

type dbInfo struct {
	Name          string
	RecoveryModel string
	State         string
	Size          int64 // bytes of all files; 0 when unknown
}

// systemDatabases are backed up like the others but listed last.
var systemDatabases = map[string]bool{"master": true, "model": true, "msdb": true}

func listDatabases(ctx context.Context, db *sql.DB) ([]dbInfo, error) {
	rows, err := db.QueryContext(ctx, `
SELECT d.name, d.recovery_model_desc, d.state_desc,
       (SELECT SUM(CAST(f.size AS bigint)) * 8192 FROM sys.master_files f WHERE f.database_id = d.database_id)
FROM sys.databases d
WHERE d.name <> 'tempdb'
ORDER BY d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dbInfo
	for rows.Next() {
		var d dbInfo
		var size sql.NullInt64
		if err := rows.Scan(&d.Name, &d.RecoveryModel, &d.State, &size); err != nil {
			return nil, err
		}
		d.Size = size.Int64
		out = append(out, d)
	}
	return out, rows.Err()
}

// backupKinds maps the plugin's backup kinds to their item extension.
var backupKinds = map[string]string{"full": "bak", "differential": "bak", "log": "trn"}

type backupOptions struct {
	CopyOnly    bool
	Compression bool
}

// backupStatement builds the BACKUP statement for one database.
func backupStatement(kind, database, device string, o backupOptions) (string, error) {
	var b strings.Builder
	switch kind {
	case "full", "differential":
		fmt.Fprintf(&b, "BACKUP DATABASE %s TO VIRTUAL_DEVICE = %s WITH ", quoteName(database), quoteString(device))
	case "log":
		fmt.Fprintf(&b, "BACKUP LOG %s TO VIRTUAL_DEVICE = %s WITH ", quoteName(database), quoteString(device))
	default:
		return "", fmt.Errorf("unknown backup kind %q (full, differential, log)", kind)
	}
	opts := []string{"CHECKSUM", "NAME = " + quoteString("HitStor "+kind)}
	if kind == "differential" {
		opts = append(opts, "DIFFERENTIAL")
	}
	if o.CopyOnly {
		if kind == "differential" {
			return "", fmt.Errorf("copy_only does not apply to differential backups")
		}
		opts = append(opts, "COPY_ONLY")
	}
	// Uncompressed backups deduplicate across runs; compression defeats that.
	if o.Compression {
		opts = append(opts, "COMPRESSION")
	} else {
		opts = append(opts, "NO_COMPRESSION")
	}
	b.WriteString(strings.Join(opts, ", "))
	return b.String(), nil
}

// readBackupSet reads msdb's record of the backup written to device.
func readBackupSet(ctx context.Context, db *sql.DB, device string) (itemMeta, error) {
	const q = `
SELECT TOP (1)
  bs.type, bs.is_copy_only,
  CONVERT(varchar(40), bs.first_lsn), CONVERT(varchar(40), bs.last_lsn),
  CONVERT(varchar(40), bs.checkpoint_lsn), CONVERT(varchar(40), bs.database_backup_lsn),
  ISNULL(CONVERT(varchar(40), bs.differential_base_lsn), ''),
  DATEADD(minute, DATEDIFF(minute, GETDATE(), GETUTCDATE()), bs.backup_start_date),
  DATEADD(minute, DATEDIFF(minute, GETDATE(), GETUTCDATE()), bs.backup_finish_date),
  bs.recovery_model, CONVERT(varchar(40), bs.database_guid), ISNULL(CONVERT(varchar(40), bs.family_guid), ''),
  bs.server_name, bs.database_name
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily mf ON mf.media_set_id = bs.media_set_id
WHERE mf.physical_device_name = @p1
ORDER BY bs.backup_set_id DESC`
	var m itemMeta
	err := db.QueryRowContext(ctx, q, device).Scan(&m.Type, &m.CopyOnly, &m.FirstLSN, &m.LastLSN, &m.CheckpointLSN, &m.DatabaseBackupLSN,
		&m.DifferentialBaseLSN, &m.Start, &m.Finish, &m.RecoveryModel, &m.DatabaseGUID, &m.FamilyGUID, &m.Server, &m.Database)
	if err != nil {
		return m, fmt.Errorf("read backup history for %s: %w", device, err)
	}
	m.Start, m.Finish = m.Start.UTC(), m.Finish.UTC()
	return m, nil
}

func readFiles(ctx context.Context, db *sql.DB, database string) ([]backupFile, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, type_desc, physical_name FROM sys.master_files WHERE database_id = DB_ID(@p1) ORDER BY file_id`, database)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []backupFile
	for rows.Next() {
		var f backupFile
		if err := rows.Scan(&f.Logical, &f.Type, &f.Physical); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func defaultPaths(ctx context.Context, db *sql.DB) (data, log string, err error) {
	err = db.QueryRowContext(ctx, `SELECT CONVERT(nvarchar(4000), SERVERPROPERTY('InstanceDefaultDataPath')), CONVERT(nvarchar(4000), SERVERPROPERTY('InstanceDefaultLogPath'))`).Scan(&data, &log)
	return data, log, err
}

// restoreOptions controls how items are restored.
type restoreOptions struct {
	Database string // target database name
	Replace  bool   // overwrite an existing database
	Recovery bool   // bring the database online after the last item
	DataPath string // where to put data files when relocating
	LogPath  string // where to put log files when relocating
	Relocate bool   // MOVE files (always when restoring under another name)
}

// moveClauses relocates every file of the backup next to the default paths,
// named after the target database so a copy never collides with the
// original's files.
func moveClauses(files []backupFile, target, dataPath, logPath string) []string {
	var out []string
	rows, logs := 0, 0
	for _, f := range files {
		var dst string
		switch f.Type {
		case "LOG":
			name := target + "_log"
			if logs > 0 {
				name += "_" + f.Logical
			}
			dst = joinWin(logPath, name+".ldf")
			logs++
		default:
			name, ext := target, ".mdf"
			if rows > 0 {
				name, ext = target+"_"+f.Logical, ".ndf"
			}
			dst = joinWin(dataPath, name+ext)
			rows++
		}
		out = append(out, fmt.Sprintf("MOVE %s TO %s", quoteString(f.Logical), quoteString(dst)))
	}
	return out
}

func joinWin(dir, name string) string {
	if dir == "" || strings.HasSuffix(dir, `\`) {
		return dir + name
	}
	return dir + `\` + name
}

// restoreStatement builds the RESTORE for one item of a plan. first adds
// the MOVE clauses; last applies recovery; stopAt (non-zero, UTC) ends a log
// restore at that point, given as parameter @p1 converted to server time.
func restoreStatement(meta itemMeta, device string, o restoreOptions, moves []string, first, last bool, stopAt time.Time) string {
	what := "DATABASE"
	if meta.Type == typeLog {
		what = "LOG"
	}
	opts := []string{}
	if first {
		opts = append(opts, moves...)
		if o.Replace {
			opts = append(opts, "REPLACE")
		}
	}
	if last && o.Recovery {
		opts = append(opts, "RECOVERY")
	} else {
		opts = append(opts, "NORECOVERY")
	}
	prefix := ""
	if !stopAt.IsZero() && meta.Type == typeLog {
		prefix = "DECLARE @stopat datetime2 = DATEADD(minute, DATEDIFF(minute, GETUTCDATE(), GETDATE()), @p1);\n"
		opts = append(opts, "STOPAT = @stopat")
	}
	return fmt.Sprintf("%sRESTORE %s %s FROM VIRTUAL_DEVICE = %s WITH %s", prefix, what, quoteName(o.Database), quoteString(device), strings.Join(opts, ", "))
}
