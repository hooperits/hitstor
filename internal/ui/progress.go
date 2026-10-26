package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"
)

// Progress handles backup progress reporting in normal and JSON modes.
type Progress struct {
	w        io.Writer    // output destination (usually os.Stdout)
	jsonMode bool         // if true, output JSON format
	quiet    bool         // if true, suppress non-error output
	color    *ColorWriter // color support for terminal output
}

// NewProgress creates a new Progress reporter.
func NewProgress(w io.Writer, jsonMode bool, quiet bool) *Progress {
	return &Progress{
		w:        w,
		jsonMode: jsonMode,
		quiet:    quiet,
		color:    NewColorWriter(w, jsonMode || quiet),
	}
}

// StartBackup prints the start of a backup operation.
// Suppressed in quiet mode.
func (p *Progress) StartBackup() {
	if p.quiet {
		return
	}
	if p.jsonMode {
		return
	}
	fmt.Fprintf(p.w, "scanning files...\n")
}

// ScanComplete reports that the filesystem scan has finished.
// Suppressed in quiet mode.
func (p *Progress) ScanComplete(files, dirs int) {
	if p.quiet {
		return
	}
	if p.jsonMode {
		return
	}
	fmt.Fprintf(p.w, "scanned %d files in %d directories\n", files, dirs)
}

// FileProcessed reports a processed file during backup.
// path is the file path, size is the file size, isNew indicates if the file
// was new/changed (vs unchanged/skipped).
// Suppressed in quiet mode.
func (p *Progress) FileProcessed(path string, size uint64, isNew bool) {
	if p.quiet {
		return
	}
	if p.jsonMode {
		return
	}
	status := "unchanged"
	if isNew {
		status = "new"
	}
	fmt.Fprintf(p.w, "%s %s (%s)\n", status, path, FormatBytes(size))
}

// BackupComplete prints the backup summary.
// It takes the snapshot ID (8-char hex prefix) and summary stats.
// Output even in quiet mode.
func (p *Progress) BackupComplete(snapshotID string, filesNew, filesUnchanged, dirsNew uint64, bytesAdded, bytesProcessed uint64, durationSecs float64) {
	sid := truncateID(snapshotID)

	if p.jsonMode {
		data := map[string]any{
			"snapshot_id":      sid,
			"files_new":        filesNew,
			"files_unchanged":  filesUnchanged,
			"dirs_new":         dirsNew,
			"bytes_added":      bytesAdded,
			"bytes_processed":  bytesProcessed,
			"duration_seconds": durationSecs,
		}
		enc := json.NewEncoder(p.w)
		enc.SetEscapeHTML(false)
		enc.Encode(data)
		return
	}

	fmt.Fprintf(p.w, "%s\n", p.color.Green("✓ snapshot "+p.color.Cyan(sid)+" saved"))
	fmt.Fprintf(p.w, "  Files:    %8s new, %8s unchanged\n", FormatNumber(filesNew), FormatNumber(filesUnchanged))
	fmt.Fprintf(p.w, "  Dirs:     %8s new\n", FormatNumber(dirsNew))
	fmt.Fprintf(p.w, "  Added:    %8s (%s processed)\n", FormatBytes(bytesAdded), FormatBytes(bytesProcessed))
	fmt.Fprintf(p.w, "  Duration: %8.1fs\n", durationSecs)
}

// AgentBackupComplete prints the remote backup summary.
// Output even in quiet mode.
func (p *Progress) AgentBackupComplete(snapshotID string, filesNew, filesUnchanged, bytesAdded, bytesProcessed uint64, durationSecs float64) {
	sid := truncateID(snapshotID)

	if p.jsonMode {
		data := map[string]any{
			"snapshot_id":      sid,
			"files_new":        filesNew,
			"files_unchanged":  filesUnchanged,
			"bytes_added":      bytesAdded,
			"bytes_processed":  bytesProcessed,
			"duration_seconds": durationSecs,
		}
		enc := json.NewEncoder(p.w)
		enc.SetEscapeHTML(false)
		enc.Encode(data)
		return
	}

	fmt.Fprintf(p.w, "%s\n", p.color.Green("✓ snapshot "+p.color.Cyan(sid)+" saved (remote)"))
	fmt.Fprintf(p.w, "  Files:    %8s new, %8s unchanged\n", FormatNumber(filesNew), FormatNumber(filesUnchanged))
	fmt.Fprintf(p.w, "  Added:    %8s (%s processed)\n", FormatBytes(bytesAdded), FormatBytes(bytesProcessed))
	fmt.Fprintf(p.w, "  Duration: %8.1fs\n", durationSecs)
}

// RestoreStart prints the start of a restore operation.
// Suppressed in quiet mode (it is a progress line).
func (p *Progress) RestoreStart(snapshotID string, targetPath string) {
	if p.quiet {
		return
	}
	sid := truncateID(snapshotID)

	if p.jsonMode {
		return
	}
	fmt.Fprintf(p.w, "restoring snapshot %s to %s\n", sid, targetPath)
}

// FileRestored reports a restored file during restore.
// Suppressed in quiet mode.
func (p *Progress) FileRestored(path string, size uint64) {
	if p.quiet {
		return
	}
	if p.jsonMode {
		return
	}
	fmt.Fprintf(p.w, "restored %s (%s)\n", path, FormatBytes(size))
}

// RestoreComplete prints the restore summary.
// Output even in quiet mode.
func (p *Progress) RestoreComplete(filesRestored uint64, bytesRestored uint64, durationSecs float64) {
	if p.jsonMode {
		data := map[string]any{
			"files_restored":   filesRestored,
			"bytes_restored":   bytesRestored,
			"duration_seconds": durationSecs,
		}
		enc := json.NewEncoder(p.w)
		enc.SetEscapeHTML(false)
		enc.Encode(data)
		return
	}

	fmt.Fprintf(p.w, "%s\n", p.color.Green(
		fmt.Sprintf("✓ restored %s files (%s) in %.1fs",
			FormatNumber(filesRestored), FormatBytes(bytesRestored), durationSecs)))
}

// InitComplete prints the result of repository initialization.
// Output even in quiet mode.
func (p *Progress) InitComplete(repoID string, location string) {
	rid := truncateID(repoID)

	if p.jsonMode {
		data := map[string]any{
			"repository_id": rid,
			"location":      location,
		}
		enc := json.NewEncoder(p.w)
		enc.SetEscapeHTML(false)
		enc.Encode(data)
		return
	}

	fmt.Fprintf(p.w, "%s\n", p.color.Green("created new repository "+p.color.Cyan(rid)+" at "+location))
}

// ForgetComplete prints the forget summary with retention policy results.
// Output even in quiet mode.
func (p *Progress) ForgetComplete(kept, forgotten int) {
	if p.jsonMode {
		data := map[string]any{
			"snapshots_kept":      kept,
			"snapshots_forgotten": forgotten,
		}
		enc := json.NewEncoder(p.w)
		enc.SetEscapeHTML(false)
		enc.Encode(data)
		return
	}

	fmt.Fprintf(p.w, "%s\n", p.color.Green(fmt.Sprintf("snapshots: %d kept, %d forgotten", kept, forgotten)))
}

// PruneComplete prints the prune summary.
// Output even in quiet mode.
func (p *Progress) PruneComplete(packsDeleted, packsRepacked int, bytesFreed, bytesRepacked uint64, indexesDeleted, indexesWritten int) {
	if p.jsonMode {
		data := map[string]any{
			"packs_deleted":   packsDeleted,
			"packs_repacked":  packsRepacked,
			"bytes_freed":     bytesFreed,
			"bytes_repacked":  bytesRepacked,
			"indexes_deleted": indexesDeleted,
			"indexes_written": indexesWritten,
		}
		enc := json.NewEncoder(p.w)
		enc.SetEscapeHTML(false)
		enc.Encode(data)
		return
	}

	fmt.Fprintf(p.w, "%s\n", p.color.Green("prune complete"))
	fmt.Fprintf(p.w, "Packs:   %4d deleted, %4d repacked\n", packsDeleted, packsRepacked)
	fmt.Fprintf(p.w, "Freed:   %s (repacked %s)\n", FormatBytes(bytesFreed), FormatBytes(bytesRepacked))
	fmt.Fprintf(p.w, "Indexes: %4d deleted, %4d written\n", indexesDeleted, indexesWritten)
}

// ProgressBar renders a live-updating progress bar to the terminal.
// It shows percentage, file count, throughput, and ETA.
// The bar is suppressed in quiet mode, JSON mode, or non-interactive terminals.
type ProgressBar struct {
	totalFiles uint64
	totalBytes uint64
	doneFiles  uint64
	doneBytes  uint64
	startTime  time.Time
	w          io.Writer
	active     bool
	done       chan struct{}
	barWidth   int
}

// NewProgressBar creates a new progress bar. If suppress is true or stdout is
// not a terminal, the bar is inactive and all methods are no-ops.
func NewProgressBar(w io.Writer, suppress bool) *ProgressBar {
	bar := &ProgressBar{
		w:        w,
		barWidth: 20,
	}
	if suppress || !isTerminal(w) {
		return bar
	}
	bar.active = true
	return bar
}

// Start begins the progress bar with the given totals and starts the
// background rendering ticker.
func (b *ProgressBar) Start(totalFiles, totalBytes uint64) {
	if !b.active {
		return
	}
	b.totalFiles = totalFiles
	b.totalBytes = totalBytes
	b.startTime = time.Now()
	b.done = make(chan struct{})

	go func() {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				b.render()
			case <-b.done:
				return
			}
		}
	}()
}

// Add records a completed file with the given size.
func (b *ProgressBar) Add(fileSize uint64) {
	if !b.active {
		return
	}
	atomic.AddUint64(&b.doneFiles, 1)
	atomic.AddUint64(&b.doneBytes, fileSize)
}

// Active returns true if the progress bar is rendering.
func (b *ProgressBar) Active() bool {
	return b.active
}

// Finish stops the ticker and clears the progress line.
func (b *ProgressBar) Finish() {
	if !b.active {
		return
	}
	if b.done != nil {
		close(b.done)
	}
	// Clear the line.
	fmt.Fprintf(b.w, "\r%*s\r", 80, "")
}

// render writes the current progress to the terminal using \r.
func (b *ProgressBar) render() {
	files := atomic.LoadUint64(&b.doneFiles)
	bytes := atomic.LoadUint64(&b.doneBytes)
	total := b.totalFiles

	var pct float64
	if total > 0 {
		pct = float64(files) / float64(total) * 100
	}

	// Build the bar: [####----]
	filled := 0
	if total > 0 {
		filled = int(float64(b.barWidth) * float64(files) / float64(total))
	}
	if filled > b.barWidth {
		filled = b.barWidth
	}
	bar := make([]byte, b.barWidth)
	for i := range bar {
		if i < filled {
			bar[i] = '#'
		} else {
			bar[i] = '-'
		}
	}

	elapsed := time.Since(b.startTime).Seconds()
	var throughput string
	if elapsed > 0 {
		bps := float64(bytes) / elapsed
		throughput = FormatBytes(uint64(bps)) + "/s"
	} else {
		throughput = "-- B/s"
	}

	var eta string
	if files > 0 && total > files {
		secsPerFile := elapsed / float64(files)
		remaining := secsPerFile * float64(total-files)
		eta = formatDuration(remaining)
	} else if files >= total {
		eta = "0s"
	} else {
		eta = "--"
	}

	line := fmt.Sprintf("\r[%s] %3.0f%% | %s/%s files | %s | ETA %s",
		string(bar), pct,
		FormatNumber(files), FormatNumber(total),
		throughput, eta)

	fmt.Fprint(b.w, line)
}

// isTerminal checks if the writer is connected to a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// formatDuration formats seconds into a human-readable duration like "1m23s" or "45s".
func formatDuration(secs float64) string {
	if secs < 60 {
		return fmt.Sprintf("%.0fs", secs)
	}
	m := int(secs) / 60
	s := int(secs) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}

// truncateID returns the first 8 characters of an ID string.
// If the string is shorter than 8 characters, it is returned as-is.
func truncateID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// FormatBytes formats a byte count into a human-readable string using
// binary prefixes (KiB, MiB, GiB, TiB).
func FormatBytes(n uint64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
		tib = 1024 * gib
	)

	switch {
	case n >= tib:
		return fmt.Sprintf("%.1f TiB", float64(n)/float64(tib))
	case n >= gib:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// FormatNumber formats a uint64 with comma separators for thousands.
func FormatNumber(n uint64) string {
	if n == 0 {
		return "0"
	}

	// Convert to string without commas first.
	s := fmt.Sprintf("%d", n)

	// Insert commas from the right.
	result := make([]byte, 0, len(s)+(len(s)-1)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
