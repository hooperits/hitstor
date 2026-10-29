package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// A differential is restorable from HitStor only when its base, the last
// full backup that was not copy-only, is one HitStor took; a log backup only
// when the log chain it continues is HitStor's (no other tool took a log
// backup in between, and HitStor's full started it). SQL Server does not
// care who took a backup, so the plugin checks msdb before it backs up, and
// asks for a full backup instead of producing backups no restore could use.

// fullRequiredPrefix starts the error of a run that needs a full backup
// first; the dashboard recognises it and offers to run one.
const fullRequiredPrefix = "full backup required: "

// Reasons a differential or log backup cannot be taken.
const (
	reasonNoFull       = "no-full"       // no full backup since the database was created or switched to FULL
	reasonExternalFull = "external-full" // the base or chain-starting full was taken by another tool
	reasonExternalLog  = "external-log"  // another tool took the last log backup
	reasonUnknownBase  = "unknown-base"  // msdb has no record of the base (history purged)
)

// historyEntry is one backup recorded in msdb.
type historyEntry struct {
	Type   string // D (full), L (log)
	Device string
	Finish time.Time
	User   string
}

func (h historyEntry) hitstor() bool { return isHitStorDevice(h.Device) }

func (h historyEntry) describe() string {
	what := "full"
	if h.Type == "L" {
		what = "log backup"
	}
	return fmt.Sprintf("%s of %s by %s to %s", what, h.Finish.UTC().Format("2006-01-02 15:04 UTC"), h.User, h.Device)
}

// isHitStorDevice reports whether a backup went to a HitStor virtual device
// (see newDeviceName).
func isHitStorDevice(device string) bool { return strings.HasPrefix(device, "HitStor_") }

// chainState is what msdb and the database say about one database's chain.
type chainState struct {
	RecoveryModel string
	// DiffBaseLSN is the checkpoint LSN of the full backup a differential
	// would be based on; empty when there is none.
	DiffBaseLSN string
	Base        *historyEntry // msdb's record of that full
	// LastLogLSN is where the next log backup starts; empty when no full
	// backup has started a log chain.
	LastLogLSN string
	Chain      *historyEntry // msdb's record of the backup that ends the chain so far
}

// chainIssue says why one database needs a full backup first.
type chainIssue struct {
	Object string // instance/database
	Reason string
	Detail string
}

func (c chainIssue) String() string {
	if c.Detail == "" {
		return fmt.Sprintf("%s (%s)", c.Object, c.Reason)
	}
	return fmt.Sprintf("%s (%s: %s)", c.Object, c.Reason, c.Detail)
}

// checkChain returns the issue that keeps a kind backup of obj from being
// restorable from HitStor, or nil. Full backups have none.
func checkChain(kind, obj string, s chainState) *chainIssue {
	issue := func(reason, detail string) *chainIssue {
		return &chainIssue{Object: obj, Reason: reason, Detail: detail}
	}
	switch kind {
	case "differential":
		switch {
		case s.DiffBaseLSN == "":
			return issue(reasonNoFull, "")
		case s.Base == nil:
			return issue(reasonUnknownBase, "no backup history for base LSN "+s.DiffBaseLSN)
		case !s.Base.hitstor():
			return issue(reasonExternalFull, s.Base.describe())
		}
	case "log":
		switch {
		case s.LastLogLSN == "":
			return issue(reasonNoFull, "")
		case s.Chain == nil:
			return issue(reasonUnknownBase, "no backup history for log chain LSN "+s.LastLogLSN)
		case !s.Chain.hitstor() && s.Chain.Type == "L":
			return issue(reasonExternalLog, s.Chain.describe())
		case !s.Chain.hitstor():
			return issue(reasonExternalFull, s.Chain.describe())
		}
	}
	return nil
}

// fullRequiredError lists the databases that need a full backup first.
type fullRequiredError []chainIssue

func (e fullRequiredError) Error() string {
	parts := make([]string, len(e))
	for i, c := range e {
		parts[i] = c.String()
	}
	return fullRequiredPrefix + strings.Join(parts, "; ")
}

// readChainState reads database's chain from the instance and msdb.
func readChainState(ctx context.Context, db *sql.DB, database string) (chainState, error) {
	// Backup dates in msdb are server local time; convert to UTC as
	// readBackupSet does. Matching the database GUID keeps a database
	// recreated or restored under the same name from inheriting another's
	// history. A full starts the log chain at its first or
	// checkpoint LSN; a log backup ends it at its last LSN.
	const q = `
SELECT d.recovery_model_desc,
  ISNULL(CONVERT(varchar(40), f.differential_base_lsn), ''),
  ISNULL(CONVERT(varchar(40), r.last_log_backup_lsn), ''),
  b.physical_device_name, DATEADD(minute, DATEDIFF(minute, GETDATE(), GETUTCDATE()), b.backup_finish_date), b.user_name,
  c.type, c.physical_device_name, DATEADD(minute, DATEDIFF(minute, GETDATE(), GETUTCDATE()), c.backup_finish_date), c.user_name
FROM sys.databases d
JOIN sys.database_recovery_status r ON r.database_id = d.database_id
JOIN sys.master_files f ON f.database_id = d.database_id AND f.file_id = 1
OUTER APPLY (
  SELECT TOP (1) mf.physical_device_name, bs.backup_finish_date, bs.user_name
  FROM msdb.dbo.backupset bs JOIN msdb.dbo.backupmediafamily mf ON mf.media_set_id = bs.media_set_id
  WHERE bs.database_name = d.name AND bs.database_guid = r.database_guid AND bs.type = 'D' AND bs.is_copy_only = 0
    AND bs.checkpoint_lsn = f.differential_base_lsn
  ORDER BY bs.backup_set_id DESC) b
OUTER APPLY (
  SELECT TOP (1) bs.type, mf.physical_device_name, bs.backup_finish_date, bs.user_name
  FROM msdb.dbo.backupset bs JOIN msdb.dbo.backupmediafamily mf ON mf.media_set_id = bs.media_set_id
  WHERE bs.database_name = d.name AND bs.database_guid = r.database_guid AND bs.is_copy_only = 0 AND (
    (bs.type = 'L' AND bs.last_lsn = r.last_log_backup_lsn) OR
    (bs.type = 'D' AND (bs.checkpoint_lsn = r.last_log_backup_lsn OR bs.first_lsn = r.last_log_backup_lsn)))
  ORDER BY bs.backup_set_id DESC) c
WHERE d.name = @p1`
	var s chainState
	var bDev, bUser, cType, cDev, cUser sql.NullString
	var bFinish, cFinish sql.NullTime
	err := db.QueryRowContext(ctx, q, database).Scan(&s.RecoveryModel, &s.DiffBaseLSN, &s.LastLogLSN,
		&bDev, &bFinish, &bUser, &cType, &cDev, &cFinish, &cUser)
	if err != nil {
		return s, fmt.Errorf("read backup chain of %s: %w", database, err)
	}
	if bDev.Valid {
		s.Base = &historyEntry{Type: "D", Device: bDev.String, Finish: bFinish.Time.UTC(), User: bUser.String}
	}
	if cDev.Valid {
		s.Chain = &historyEntry{Type: cType.String, Device: cDev.String, Finish: cFinish.Time.UTC(), User: cUser.String}
	}
	return s, nil
}
