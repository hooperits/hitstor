package dashboard

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/server"
)

// A database backup chain, as the mssql plugin records it: a full backup,
// the differentials based on it (their differential base LSN is the full's
// checkpoint LSN) and the log backups after it, each starting where the
// previous one ended. Plugins whose metadata lacks these fields get no
// chains, only their backups in time order.

// chainMeta is the part of an item's metadata chains are built from.
type chainMeta struct {
	Type                string    `json:"type"` // D, I, L
	CopyOnly            bool      `json:"copy_only"`
	FirstLSN            string    `json:"first_lsn"`
	LastLSN             string    `json:"last_lsn"`
	CheckpointLSN       string    `json:"checkpoint_lsn"`
	DifferentialBaseLSN string    `json:"differential_base_lsn"`
	DatabaseGUID        string    `json:"database_guid"`
	Finish              time.Time `json:"finish"`
}

func lsnOf(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return new(big.Int)
	}
	return n
}

// chainPoint is one backup in a chain.
type chainPoint struct {
	Kind     string    `json:"kind"` // full, differential, log
	Label    string    `json:"label"`
	At       time.Time `json:"at"`
	When     string    `json:"when"`
	Size     string    `json:"size"`
	Snapshot string    `json:"snapshot"`
	CopyOnly bool      `json:"copy_only,omitempty"`
	// Gap marks a log backup that does not start where the chain left
	// off: changes between them are missing.
	Gap bool `json:"gap,omitempty"`

	meta chainMeta
}

// backupChain is a full backup and what builds on it.
type backupChain struct {
	Full  chainPoint   `json:"full"`
	Diffs []chainPoint `json:"differentials"`
	Logs  []chainPoint `json:"logs"`
	// RestorableTo is the latest moment the chain restores to without a
	// gap.
	RestorableTo string `json:"restorable_to"`
	Broken       bool   `json:"broken"` // a log gap cuts the chain short
	Current      bool   `json:"current"`
}

// databaseChains are one database's chains, newest first, and its backups
// that build on nothing HitStor holds.
type databaseChains struct {
	Object  string        `json:"object"`
	Chains  []backupChain `json:"chains"`
	Orphans []chainPoint  `json:"orphans,omitempty"`
}

// buildChains groups one database's backup items into chains.
func buildChains(c *i18n.Catalog, object string, items []workloadPoint) databaseChains {
	out := databaseChains{Object: object}
	var fulls []*backupChain
	var diffs, logs []chainPoint
	for _, p := range items {
		var m chainMeta
		if json.Unmarshal(p.Metadata, &m) != nil || m.Type == "" {
			continue
		}
		at := m.Finish
		if at.IsZero() {
			at = p.CreatedAt
		}
		cp := chainPoint{At: at, When: dateTimeIn(c, at.Local()), Size: formatBytes(uint64(p.Size)),
			Snapshot: shortSnapshot(p.SnapshotID), CopyOnly: m.CopyOnly, meta: m}
		switch m.Type {
		case "D":
			cp.Kind = "full"
			fulls = append(fulls, &backupChain{Full: cp})
		case "I":
			cp.Kind = "differential"
			diffs = append(diffs, cp)
		case "L":
			cp.Kind = "log"
			logs = append(logs, cp)
		}
	}
	for _, list := range [][]chainPoint{diffs, logs} {
		for i := range list {
			list[i].Label = kindLabel(c, list[i].Kind)
		}
	}
	sort.Slice(fulls, func(i, j int) bool { return fulls[i].Full.At.Before(fulls[j].Full.At) })
	for _, f := range fulls {
		f.Full.Label = kindLabel(c, "full")
	}

	// A differential belongs to the full whose checkpoint is its base;
	// copy-only fulls are never a base.
	for _, d := range diffs {
		var base *backupChain
		for _, f := range fulls {
			if !f.Full.CopyOnly && f.Full.meta.DatabaseGUID == d.meta.DatabaseGUID &&
				lsnOf(f.Full.meta.CheckpointLSN).Cmp(lsnOf(d.meta.DifferentialBaseLSN)) == 0 {
				base = f
			}
		}
		if base == nil {
			out.Orphans = append(out.Orphans, d)
			continue
		}
		base.Diffs = append(base.Diffs, d)
	}

	// A log backup continues the chain of the latest full taken before
	// it; logs before any full belong to none.
	sort.Slice(logs, func(i, j int) bool { return lsnOf(logs[i].meta.FirstLSN).Cmp(lsnOf(logs[j].meta.FirstLSN)) < 0 })
	for _, l := range logs {
		var owner *backupChain
		for _, f := range fulls {
			if !f.Full.CopyOnly && !f.Full.At.After(l.At) {
				owner = f
			}
		}
		if owner == nil {
			out.Orphans = append(out.Orphans, l)
			continue
		}
		if n := len(owner.Logs); n > 0 {
			l.Gap = lsnOf(l.meta.FirstLSN).Cmp(lsnOf(owner.Logs[n-1].meta.LastLSN)) != 0
		} else {
			// The first log must reach back to the full's last LSN.
			l.Gap = lsnOf(l.meta.FirstLSN).Cmp(lsnOf(owner.Full.meta.LastLSN)) > 0
		}
		owner.Logs = append(owner.Logs, l)
	}

	current := -1
	for i, f := range fulls {
		if !f.Full.CopyOnly {
			current = i
		}
	}
	for i := len(fulls) - 1; i >= 0; i-- {
		f := fulls[i]
		latest := f.Full.At
		for _, d := range f.Diffs {
			if d.At.After(latest) {
				latest = d.At
			}
		}
		for _, l := range f.Logs {
			if l.Gap {
				f.Broken = true
				break
			}
			if l.At.After(latest) {
				latest = l.At
			}
		}
		f.RestorableTo = dateTimeIn(c, latest.Local())
		f.Current = i == current
		sort.Slice(f.Diffs, func(a, b int) bool { return f.Diffs[a].At.After(f.Diffs[b].At) })
		sort.Slice(f.Logs, func(a, b int) bool { return f.Logs[a].At.After(f.Logs[b].At) })
		out.Chains = append(out.Chains, *f)
	}
	sort.Slice(out.Orphans, func(i, j int) bool { return out.Orphans[i].At.After(out.Orphans[j].At) })
	return out
}

func shortSnapshot(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// selectionCovers reports whether a job's selection includes object: the
// object itself or a container of it (an instance holds its databases).
func selectionCovers(selection []string, object string) bool {
	for _, s := range selection {
		if object == s || strings.HasPrefix(object, s+"/") {
			return true
		}
	}
	return false
}

// handleAPIBackupJobChains handles GET /api/backups/{id}/chains: the backup
// chains of every database a workload job backs up.
func (d *Dashboard) handleAPIBackupJobChains(w http.ResponseWriter, r *http.Request) {
	job := d.registry.GetJob(r.PathValue("id"))
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if job.JobType != server.JobTypeWorkload {
		writeJSON(w, []databaseChains{})
		return
	}
	points, err := d.workloadItems(job.AgentName, job.Plugin, "", "")
	if err != nil {
		http.Error(w, fmt.Sprintf("list backups: %v", err), http.StatusInternalServerError)
		return
	}
	byObject := map[string][]workloadPoint{}
	var objects []string
	for _, p := range points {
		obj := path.Dir(p.Item)
		if !selectionCovers(job.Selection, obj) {
			continue
		}
		if _, ok := byObject[obj]; !ok {
			objects = append(objects, obj)
		}
		byObject[obj] = append(byObject[obj], p)
	}
	sort.Strings(objects)
	c := d.catalog(r)
	out := make([]databaseChains, 0, len(objects))
	for _, obj := range objects {
		out = append(out, buildChains(c, obj, byObject[obj]))
	}
	writeJSON(w, out)
}
