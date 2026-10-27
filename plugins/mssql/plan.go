package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"time"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

// Backup set types, as in msdb.dbo.backupset.type.
const (
	typeFull         = "D"
	typeDifferential = "I"
	typeLog          = "L"
)

// backupFile is one database file at backup time, used to relocate files
// when restoring under another name.
type backupFile struct {
	Logical  string `json:"logical"`
	Type     string `json:"type"` // ROWS, LOG, FILESTREAM, FULLTEXT
	Physical string `json:"physical"`
}

// itemMeta is the metadata attached to every backup item. LSNs are kept as
// decimal strings: they are numeric(25,0) in msdb.
type itemMeta struct {
	Instance            string       `json:"instance"`
	Database            string       `json:"database"`
	Type                string       `json:"type"`
	CopyOnly            bool         `json:"copy_only"`
	FirstLSN            string       `json:"first_lsn"`
	LastLSN             string       `json:"last_lsn"`
	CheckpointLSN       string       `json:"checkpoint_lsn"`
	DatabaseBackupLSN   string       `json:"database_backup_lsn"`
	DifferentialBaseLSN string       `json:"differential_base_lsn,omitempty"`
	Start               time.Time    `json:"start"`
	Finish              time.Time    `json:"finish"`
	RecoveryModel       string       `json:"recovery_model"`
	DatabaseGUID        string       `json:"database_guid"`
	FamilyGUID          string       `json:"family_guid,omitempty"`
	Server              string       `json:"server"`
	Files               []backupFile `json:"files,omitempty"`
}

func lsn(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return new(big.Int)
	}
	return n
}

type planItem struct {
	ref  string
	meta itemMeta
}

// planRestore chooses the items that restore a database to target (zero:
// the latest point available): the newest full backup finished by then,
// the newest differential based on it, and the unbroken chain of log
// backups after that, ending with the log that contains target.
//
// When target falls between log backups the restore stops at target
// (STOPAT); when no log reaches it, the database is restored to the latest
// point before it.
func planRestore(target time.Time, cands []hitstorplugin.RestoreCandidate) ([]string, error) {
	var fulls, diffs, logs []planItem
	for _, c := range cands {
		var m itemMeta
		if err := json.Unmarshal(c.Metadata, &m); err != nil || m.Type == "" || m.LastLSN == "" {
			continue // not produced by this plugin, or damaged: never guess
		}
		it := planItem{ref: c.Ref, meta: m}
		switch m.Type {
		case typeFull:
			fulls = append(fulls, it)
		case typeDifferential:
			diffs = append(diffs, it)
		case typeLog:
			logs = append(logs, it)
		}
	}
	upTo := func(m itemMeta) bool { return target.IsZero() || !m.Finish.After(target) }

	// Newest full finished by the target.
	var full *planItem
	for i := range fulls {
		if upTo(fulls[i].meta) && (full == nil || lsn(fulls[i].meta.CheckpointLSN).Cmp(lsn(full.meta.CheckpointLSN)) > 0) {
			full = &fulls[i]
		}
	}
	if full == nil {
		if target.IsZero() {
			return nil, fmt.Errorf("no full backup available")
		}
		return nil, fmt.Errorf("no full backup finished at or before %s", target.Format(time.RFC3339))
	}
	plan := []string{full.ref}
	base := full

	// Newest differential based on that full (copy-only fulls are never a
	// differential base, so their checkpoint matches nothing).
	var diff *planItem
	for i := range diffs {
		d := diffs[i].meta
		if d.DatabaseGUID == full.meta.DatabaseGUID && lsn(d.DifferentialBaseLSN).Cmp(lsn(full.meta.CheckpointLSN)) == 0 && upTo(d) &&
			(diff == nil || lsn(d.LastLSN).Cmp(lsn(diff.meta.LastLSN)) > 0) {
			diff = &diffs[i]
		}
	}
	if diff != nil {
		plan = append(plan, diff.ref)
		base = diff
	}

	// The base already ends exactly at the target: no logs needed.
	if !target.IsZero() && !base.meta.Finish.Before(target) {
		return plan, nil
	}

	// Log chain: the first log covers base's last LSN; each next one starts
	// where the previous ended.
	sort.Slice(logs, func(i, j int) bool { return lsn(logs[i].meta.FirstLSN).Cmp(lsn(logs[j].meta.FirstLSN)) < 0 })
	next := lsn(base.meta.LastLSN)
	first := true
	for _, l := range logs {
		if l.meta.DatabaseGUID != full.meta.DatabaseGUID {
			continue
		}
		f, last := lsn(l.meta.FirstLSN), lsn(l.meta.LastLSN)
		if first {
			if f.Cmp(next) > 0 || last.Cmp(next) <= 0 {
				continue
			}
		} else if f.Cmp(next) != 0 {
			if last.Cmp(next) <= 0 {
				continue // already covered
			}
			break // gap in the chain
		}
		plan = append(plan, l.ref)
		next, first = last, false
		if !target.IsZero() && !l.meta.Finish.Before(target) {
			break // this log contains the target point
		}
	}
	return plan, nil
}
