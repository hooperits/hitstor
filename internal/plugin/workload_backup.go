package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
)

// itemChunkSize caps the data carried by one BackupMessage.
const itemChunkSize = 1 << 20

// ErrUnsupported reports that a plugin does not implement an optional RPC.
var ErrUnsupported = errors.New("plugin: operation not supported by this plugin")

// BrowseEntry is one object in a plugin's selection tree.
type BrowseEntry struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Selectable  bool   `json:"selectable"`
	HasChildren bool   `json:"has_children"`
	// Detail is a short line of facts shown next to the name.
	Detail string `json:"detail,omitempty"`
	// Status is set when the object needs attention, e.g. "OFFLINE".
	Status string `json:"status,omitempty"`
}

// BackupRequest asks a plugin to back up the selected objects.
type BackupRequest struct {
	Selection []string
	Kind      string
	Options   map[string]string
	// Known lists items the host holds for the selection, with their
	// metadata, so a plugin can check that a backup builds on restorable
	// ones. KnownProvided says the host sent the list (possibly empty);
	// without it the host does not know.
	Known         []KnownItem
	KnownProvided bool
	// ObjectResults is set by hosts that accept ItemSink.Result and
	// ItemWriter.Abort: the plugin reports objects it skipped or failed
	// and goes on with the others.
	ObjectResults bool
}

// KnownItem is an item the host holds.
type KnownItem struct {
	Item     string          `json:"item"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Object result statuses.
const (
	ObjectExcluded = "excluded" // by design, e.g. no log backups in SIMPLE recovery
	ObjectFailed   = "failed"   // should have been backed up
)

// ObjectResult reports a selected object that produced no item.
type ObjectResult struct {
	Object  string `json:"object"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ItemWriter receives one item's bytes. Finish ends the item and attaches
// its metadata, which must be a JSON value (or nil). Abort discards an item
// that failed part way; only when the request set ObjectResults.
type ItemWriter interface {
	io.Writer
	Finish(metadata []byte) error
	Abort() error
}

// ItemSink is handed to a plugin's Backup to emit items, one at a time.
type ItemSink interface {
	// Item starts a new item; the previous one must have been finished.
	Item(path string) (ItemWriter, error)
	// Result reports an object that produced no item; only when the
	// request set ObjectResults.
	Result(ObjectResult) error
}

// WorkloadBrowser is implemented by plugins that expose a selection tree.
type WorkloadBrowser interface {
	Browse(ctx context.Context, path string) ([]BrowseEntry, error)
}

// WorkloadBackuper is implemented by plugins that can back up.
type WorkloadBackuper interface {
	Backup(ctx context.Context, req BackupRequest, sink ItemSink) error
}

// ValidItemPath reports whether p is an acceptable item path: relative,
// slash-separated, clean, and not escaping the plugin's directory.
func ValidItemPath(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("plugin: empty item path")
	case strings.Contains(p, `\`), strings.HasPrefix(p, "/"), len(p) > 1 && p[1] == ':':
		return fmt.Errorf("plugin: item path %q must be relative and slash-separated", p)
	case path.Clean(p) != p, p == "..", strings.HasPrefix(p, "../"):
		return fmt.Errorf("plugin: item path %q is not clean", p)
	case strings.HasSuffix(p, MetadataSuffix):
		return fmt.Errorf("plugin: item path %q uses the reserved %s suffix", p, MetadataSuffix)
	}
	return nil
}

// MetadataSuffix names the snapshot file that carries an item's metadata.
const MetadataSuffix = ".meta.json"

// --- Plugin side ---

func (s *workloadGRPCServer) Browse(ctx context.Context, req *pb.BrowseRequest) (*pb.BrowseResponse, error) {
	b, ok := s.impl.(WorkloadBrowser)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "browse not supported")
	}
	entries, err := b.Browse(ctx, req.Path)
	if err != nil {
		return nil, err
	}
	resp := &pb.BrowseResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, &pb.BrowseEntry{
			Path: e.Path, Name: e.Name, Kind: e.Kind, Selectable: e.Selectable, HasChildren: e.HasChildren,
			Detail: e.Detail, Status: e.Status,
		})
	}
	return resp, nil
}

func (s *workloadGRPCServer) Backup(req *pb.BackupRequest, stream grpc.ServerStreamingServer[pb.BackupMessage]) error {
	b, ok := s.impl.(WorkloadBackuper)
	if !ok {
		return status.Error(codes.Unimplemented, "backup not supported")
	}
	sink := &streamSink{stream: stream, results: req.ObjectResults}
	known := make([]KnownItem, 0, len(req.Known))
	for _, k := range req.Known {
		known = append(known, KnownItem{Item: k.Item, Metadata: k.Metadata})
	}
	err := b.Backup(stream.Context(), BackupRequest{Selection: req.Selection, Kind: req.Kind, Options: req.Options,
		Known: known, KnownProvided: req.KnownProvided, ObjectResults: req.ObjectResults}, sink)
	if err != nil {
		return err
	}
	if sink.open != nil {
		return fmt.Errorf("plugin: item %q was not finished", sink.open.path)
	}
	return nil
}

// streamSink sends items over the Backup stream.
type streamSink struct {
	stream  grpc.ServerStreamingServer[pb.BackupMessage]
	open    *streamItem
	results bool // the host accepts results and aborted items
}

func (s *streamSink) Result(r ObjectResult) error {
	if !s.results {
		return fmt.Errorf("plugin: the host does not accept object results")
	}
	if s.open != nil {
		return fmt.Errorf("plugin: result for %q while item %q is open", r.Object, s.open.path)
	}
	return s.stream.Send(&pb.BackupMessage{Msg: &pb.BackupMessage_Result{Result: &pb.ObjectResult{
		Object: r.Object, Status: r.Status, Reason: r.Reason, Message: r.Message,
	}}})
}

func (s *streamSink) Item(p string) (ItemWriter, error) {
	if s.open != nil {
		return nil, fmt.Errorf("plugin: item %q started before %q was finished", p, s.open.path)
	}
	if err := ValidItemPath(p); err != nil {
		return nil, err
	}
	if err := s.stream.Send(&pb.BackupMessage{Msg: &pb.BackupMessage_Start{Start: &pb.ItemStart{Path: p}}}); err != nil {
		return nil, err
	}
	s.open = &streamItem{sink: s, path: p}
	return s.open, nil
}

type streamItem struct {
	sink *streamSink
	path string
	done bool
}

func (w *streamItem) Write(p []byte) (int, error) {
	if w.done {
		return 0, fmt.Errorf("plugin: write to finished item %q", w.path)
	}
	written := 0
	for len(p) > 0 {
		n := min(len(p), itemChunkSize)
		// gRPC may hold the message after Send returns; copy so callers may reuse p.
		buf := append([]byte(nil), p[:n]...)
		if err := w.sink.stream.Send(&pb.BackupMessage{Msg: &pb.BackupMessage_Data{Data: buf}}); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}

func (w *streamItem) Finish(metadata []byte) error {
	if w.done {
		return fmt.Errorf("plugin: item %q finished twice", w.path)
	}
	if len(metadata) > 0 && !json.Valid(metadata) {
		return fmt.Errorf("plugin: metadata of item %q is not valid JSON", w.path)
	}
	w.done = true
	w.sink.open = nil
	return w.sink.stream.Send(&pb.BackupMessage{Msg: &pb.BackupMessage_End{End: &pb.ItemEnd{Metadata: metadata}}})
}

func (w *streamItem) Abort() error {
	if w.done {
		return fmt.Errorf("plugin: item %q aborted after it was finished", w.path)
	}
	if !w.sink.results {
		return fmt.Errorf("plugin: the host does not accept aborted items")
	}
	w.done = true
	w.sink.open = nil
	return w.sink.stream.Send(&pb.BackupMessage{Msg: &pb.BackupMessage_End{End: &pb.ItemEnd{Aborted: true}}})
}

// --- Host side ---

// Browse lists objects under path. Returns ErrUnsupported if the plugin has
// no selection tree.
func (c *WorkloadGRPCClient) Browse(ctx context.Context, p string) ([]BrowseEntry, error) {
	resp, err := c.client.Browse(ctx, &pb.BrowseRequest{Path: p})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, ErrUnsupported
		}
		return nil, fmt.Errorf("plugin: browse: %w", err)
	}
	out := make([]BrowseEntry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		out = append(out, BrowseEntry{Path: e.Path, Name: e.Name, Kind: e.Kind, Selectable: e.Selectable, HasChildren: e.HasChildren,
			Detail: e.Detail, Status: e.Status})
	}
	return out, nil
}

// BackupItem is one item received from a plugin backup.
type BackupItem struct {
	Path     string
	Size     int64
	Metadata json.RawMessage
}

// ItemHandler consumes one item's data. It should read data to EOF; any
// unread remainder is discarded. Reading an item the plugin aborted fails
// with ErrItemAborted, and the handler must not keep what it read.
type ItemHandler func(path string, data io.Reader) error

// ErrItemAborted is returned while reading an item the plugin aborted.
var ErrItemAborted = errors.New("plugin: item aborted by the plugin")

// BackupOutcome is what a plugin backup produced: its finished items and
// its reports on objects that produced none.
type BackupOutcome struct {
	Items   []BackupItem
	Results []ObjectResult
}

// Backup runs a plugin backup, calling handle for each item in order. It
// returns the items finished, each with its size and metadata, and the
// plugin's object results. On error the outcome still holds what finished
// before it, so the caller can keep backups the database already counts.
func (c *WorkloadGRPCClient) Backup(ctx context.Context, req BackupRequest, handle ItemHandler) (BackupOutcome, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	known := make([]*pb.KnownItem, 0, len(req.Known))
	for _, k := range req.Known {
		known = append(known, &pb.KnownItem{Item: k.Item, Metadata: k.Metadata})
	}
	var out BackupOutcome
	stream, err := c.client.Backup(ctx, &pb.BackupRequest{Selection: req.Selection, Kind: req.Kind, Options: req.Options,
		Known: known, KnownProvided: req.KnownProvided, ObjectResults: req.ObjectResults})
	if err != nil {
		return out, fmt.Errorf("plugin: backup: %w", err)
	}
	r := &itemReader{stream: stream}

	seen := make(map[string]bool)
	for {
		msg, err := r.next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if res := msg.GetResult(); res != nil {
			if !req.ObjectResults {
				return out, fmt.Errorf("plugin: backup: unexpected object result")
			}
			out.Results = append(out.Results, ObjectResult{Object: res.Object, Status: res.Status, Reason: res.Reason, Message: res.Message})
			continue
		}
		start := msg.GetStart()
		if start == nil {
			return out, fmt.Errorf("plugin: backup: expected item start, got %T", msg.Msg)
		}
		if err := ValidItemPath(start.Path); err != nil {
			return out, err
		}
		if seen[start.Path] {
			return out, fmt.Errorf("plugin: backup: duplicate item %q", start.Path)
		}
		seen[start.Path] = true

		r.begin()
		err = handle(start.Path, r)
		if err == nil {
			_, err = io.Copy(io.Discard, r)
		}
		if r.aborted {
			// The plugin gave up on this item and reports why; go on.
			if !req.ObjectResults {
				return out, fmt.Errorf("plugin: backup: unexpected aborted item %q", start.Path)
			}
			continue
		}
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, BackupItem{Path: start.Path, Size: r.size, Metadata: r.metadata})
	}
}

// itemReader exposes the data messages of the current item as an io.Reader
// and stops at its ItemEnd.
type itemReader struct {
	stream   grpc.ServerStreamingClient[pb.BackupMessage]
	buf      []byte
	inItem   bool
	aborted  bool
	size     int64
	metadata json.RawMessage
}

func (r *itemReader) next() (*pb.BackupMessage, error) {
	msg, err := r.stream.Recv()
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, fmt.Errorf("plugin: backup stream: %w", err)
	}
	return msg, nil
}

func (r *itemReader) begin() {
	r.inItem, r.aborted, r.buf, r.size, r.metadata = true, false, nil, 0, nil
}

func (r *itemReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		if !r.inItem {
			return 0, io.EOF
		}
		msg, err := r.next()
		if err == io.EOF {
			return 0, fmt.Errorf("plugin: backup stream ended inside an item")
		}
		if err != nil {
			return 0, err
		}
		switch m := msg.Msg.(type) {
		case *pb.BackupMessage_Data:
			r.buf = m.Data
		case *pb.BackupMessage_End:
			if m.End.Aborted {
				r.inItem, r.aborted = false, true
				return 0, ErrItemAborted
			}
			if len(m.End.Metadata) > 0 && !json.Valid(m.End.Metadata) {
				return 0, fmt.Errorf("plugin: item metadata is not valid JSON")
			}
			r.metadata = m.End.Metadata
			r.inItem = false
		default:
			return 0, fmt.Errorf("plugin: backup: unexpected %T inside an item", msg.Msg)
		}
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	r.size += int64(n)
	return n, nil
}

// ItemManifest is the content of an item's MetadataSuffix file in a
// snapshot. The agent writes it; the plugin's own metadata travels inside,
// uninterpreted, and is handed back to the plugin when planning a restore.
type ItemManifest struct {
	Plugin        string          `json:"plugin"`
	PluginVersion string          `json:"plugin_version,omitempty"`
	Item          string          `json:"item"`
	Kind          string          `json:"kind,omitempty"`
	Size          int64           `json:"size"`
	SHA256        string          `json:"sha256"`
	CreatedAt     time.Time       `json:"created_at"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}
