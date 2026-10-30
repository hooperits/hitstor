package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

// A differential is restorable from HitStor only when HitStor holds its
// base, the last full backup that was not copy-only; a log backup only when
// HitStor holds the backup the log chain continues from (no other tool took
// a log backup in between). SQL Server does not care who took a backup, so
// the plugin checks before it backs up, and asks for a full backup instead
// of producing backups no restore could use. The host lists the items it
// holds (BackupRequest.Known); a host that does not, gets the older check
// of msdb: was the backup written to a HitStor virtual device.

// fullRequiredPrefix starts the error of a run that needs a full backup
// first; the dashboard recognises it and offers to run one.
const fullRequiredPrefix = "full backup required: "

// Reasons a differential or log backup cannot be taken.
const (
	reasonNoFull       = "no-full"       // no full backup since the database was created or switched to FULL
	reasonExternalFull = "external-full" // the base or chain-starting full was taken by another tool
	reasonExternalLog  = "external-log"  // another tool took the last log backup
	reasonUnknownBase  = "unknown-base"  // msdb has no record of the base (history purged)
	reasonNotHeld      = "not-held"      // HitStor took the base but no longer holds it
)

// Reasons a database produced no backup, besides the chain reasons above.
const (
	reasonSimpleRecovery = "simple-recovery" // no log backups in SIMPLE recovery
	reasonNotOnline      = "not-online"      // OFFLINE, RESTORING, ...
	reasonFullOnly       = "full-only"       // master takes only full backups
	reasonError          = "error"           // the backup failed
)

// knownBackups are the backups HitStor holds, by database GUID and object.
// Retention keeps chains by object name, so a backup held under another
// name (a renamed database) does not count.
type knownBackups map[string][]itemMeta

// The object name compares exactly, as retention's object tags do.
func knownKey(guid, object string) string { return strings.ToUpper(guid) + "|" + object }

// newKnownBackups reads the host's list of items, or returns nil when the
// host did not send one.
func newKnownBackups(req hitstorplugin.BackupRequest) knownBackups {
	if !req.KnownProvided {
		return nil
	}
	k := knownBackups{}
	for _, it := range req.Known {
		var m itemMeta
		if json.Unmarshal(it.Metadata, &m) == nil && m.DatabaseGUID != "" {
			key := knownKey(m.DatabaseGUID, path.Dir(it.Item))
			k[key] = append(k[key], m)
		}
	}
	return k
}

// holdsBase reports whether HitStor holds the full a differential with this
// base would build on.
func (k knownBackups) holdsBase(guid, object, baseLSN string) bool {
	for _, m := range k[knownKey(guid, object)] {
		if m.Type == typeFull && !m.CopyOnly && lsn(m.CheckpointLSN).Cmp(lsn(baseLSN)) == 0 {
			return true
		}
	}
	return false
}

// holdsChainEnd reports whether a log backup starting at lsnEnd will
// restore from what HitStor holds: HitStor holds the log ending there or the
// full that started the chain there, or a full taken after it, which the
// next log covers. The last case lets the full that a gap asks for restore
// the chain, although a full does not move the log chain's start. A
// differential would bridge a gap too, but retention keeps a log's earlier
// logs and full, not differentials, so only a full restarts a chain.
func (k knownBackups) holdsChainEnd(guid, object, lsnEnd string) bool {
	end := lsn(lsnEnd)
	for _, m := range k[knownKey(guid, object)] {
		switch m.Type {
		case typeLog:
			if lsn(m.LastLSN).Cmp(end) == 0 {
				return true
			}
		case typeFull:
			if !m.CopyOnly && (lsn(m.CheckpointLSN).Cmp(end) == 0 || lsn(m.FirstLSN).Cmp(end) == 0 || lsn(m.LastLSN).Cmp(end) > 0) {
				return true
			}
		}
	}
	return false
}

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
	State         string // ONLINE, OFFLINE, ...
	RecoveryModel string
	DatabaseGUID  string
	// DiffBaseLSN is the checkpoint LSN of the full backup a differential
	// would be based on; empty when there is none.
	DiffBaseLSN string
	Base        *historyEntry // msdb's record of that full
	// LastLogLSN is where the next log backup starts; empty when no full
	// backup has started a log chain.
	LastLogLSN string
	Chain      *historyEntry // msdb's record of the backup that ends the chain so far
	// Restart is msdb's record of a full to a HitStor device taken after
	// the chain's end, which the next log covers.
	Restart *historyEntry
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
// restorable from HitStor, or nil. Full backups have none. known is what
// HitStor holds; nil falls back to msdb's record of the backup device.
func checkChain(kind, obj string, s chainState, known knownBackups) *chainIssue {
	issue := func(reason, detail string) *chainIssue {
		return &chainIssue{Object: obj, Reason: reason, Detail: detail}
	}
	// other names why HitStor cannot build on the backup msdb recorded.
	other := func(h *historyEntry, lsnValue string) *chainIssue {
		switch {
		case h == nil:
			return issue(reasonUnknownBase, "no backup history for LSN "+lsnValue)
		case !h.hitstor() && h.Type == "L":
			return issue(reasonExternalLog, h.describe())
		case !h.hitstor():
			return issue(reasonExternalFull, h.describe())
		default:
			return issue(reasonNotHeld, h.describe())
		}
	}
	switch kind {
	case "differential":
		switch {
		case s.DiffBaseLSN == "":
			return issue(reasonNoFull, "")
		case known != nil && known.holdsBase(s.DatabaseGUID, obj, s.DiffBaseLSN):
			return nil
		case known != nil:
			return other(s.Base, s.DiffBaseLSN)
		case s.Base == nil:
			return issue(reasonUnknownBase, "no backup history for base LSN "+s.DiffBaseLSN)
		case !s.Base.hitstor():
			return issue(reasonExternalFull, s.Base.describe())
		}
	case "log":
		switch {
		case s.LastLogLSN == "":
			return issue(reasonNoFull, "")
		case known != nil && known.holdsChainEnd(s.DatabaseGUID, obj, s.LastLogLSN):
			return nil
		case known != nil:
			return other(s.Chain, s.LastLogLSN)
		case s.Restart != nil:
			return nil
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
	// history; matching the name in a binary collation keeps a rename that
	// only changes case from inheriting it either (retention keeps chains by
	// exact name). A full starts the log chain at its first or
	// checkpoint LSN; a log backup ends it at its last LSN.
	const q = `
SELECT d.state_desc, d.recovery_model_desc, ISNULL(CONVERT(varchar(40), r.database_guid), ''),
  ISNULL(CONVERT(varchar(40), f.differential_base_lsn), ''),
  ISNULL(CONVERT(varchar(40), r.last_log_backup_lsn), ''),
  b.physical_device_name, DATEADD(minute, DATEDIFF(minute, GETDATE(), GETUTCDATE()), b.backup_finish_date), b.user_name,
  c.type, c.physical_device_name, DATEADD(minute, DATEDIFF(minute, GETDATE(), GETUTCDATE()), c.backup_finish_date), c.user_name,
  g.type, g.physical_device_name, DATEADD(minute, DATEDIFF(minute, GETDATE(), GETUTCDATE()), g.backup_finish_date), g.user_name
FROM sys.databases d
JOIN sys.database_recovery_status r ON r.database_id = d.database_id
JOIN sys.master_files f ON f.database_id = d.database_id AND f.file_id = 1
OUTER APPLY (
  SELECT TOP (1) mf.physical_device_name, bs.backup_finish_date, bs.user_name
  FROM msdb.dbo.backupset bs JOIN msdb.dbo.backupmediafamily mf ON mf.media_set_id = bs.media_set_id
  WHERE bs.database_name = d.name COLLATE Latin1_General_BIN2 AND bs.database_guid = r.database_guid AND bs.type = 'D' AND bs.is_copy_only = 0
    AND bs.checkpoint_lsn = f.differential_base_lsn
  ORDER BY bs.backup_set_id DESC) b
OUTER APPLY (
  SELECT TOP (1) bs.type, mf.physical_device_name, bs.backup_finish_date, bs.user_name
  FROM msdb.dbo.backupset bs JOIN msdb.dbo.backupmediafamily mf ON mf.media_set_id = bs.media_set_id
  WHERE bs.database_name = d.name COLLATE Latin1_General_BIN2 AND bs.database_guid = r.database_guid AND bs.is_copy_only = 0 AND (
    (bs.type = 'L' AND bs.last_lsn = r.last_log_backup_lsn) OR
    (bs.type = 'D' AND (bs.checkpoint_lsn = r.last_log_backup_lsn OR bs.first_lsn = r.last_log_backup_lsn)))
  ORDER BY bs.backup_set_id DESC) c
OUTER APPLY (
  SELECT TOP (1) bs.type, mf.physical_device_name, bs.backup_finish_date, bs.user_name
  FROM msdb.dbo.backupset bs JOIN msdb.dbo.backupmediafamily mf ON mf.media_set_id = bs.media_set_id
  WHERE bs.database_name = d.name COLLATE Latin1_General_BIN2 AND bs.database_guid = r.database_guid AND bs.is_copy_only = 0
    AND bs.type = 'D' AND bs.last_lsn > r.last_log_backup_lsn AND mf.physical_device_name LIKE 'HitStor[_]%'
  ORDER BY bs.backup_set_id DESC) g
WHERE d.name = @p1`
	var s chainState
	var bDev, bUser, cType, cDev, cUser, gType, gDev, gUser sql.NullString
	var bFinish, cFinish, gFinish sql.NullTime
	err := db.QueryRowContext(ctx, q, database).Scan(&s.State, &s.RecoveryModel, &s.DatabaseGUID, &s.DiffBaseLSN, &s.LastLogLSN,
		&bDev, &bFinish, &bUser, &cType, &cDev, &cFinish, &cUser, &gType, &gDev, &gFinish, &gUser)
	if err != nil {
		return s, fmt.Errorf("read backup chain of %s: %w", database, err)
	}
	if bDev.Valid {
		s.Base = &historyEntry{Type: "D", Device: bDev.String, Finish: bFinish.Time.UTC(), User: bUser.String}
	}
	if cDev.Valid {
		s.Chain = &historyEntry{Type: cType.String, Device: cDev.String, Finish: cFinish.Time.UTC(), User: cUser.String}
	}
	if gDev.Valid {
		s.Restart = &historyEntry{Type: gType.String, Device: gDev.String, Finish: gFinish.Time.UTC(), User: gUser.String}
	}
	return s, nil
}
