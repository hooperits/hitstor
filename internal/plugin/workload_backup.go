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
}

// ItemWriter receives one item's bytes. Finish ends the item and attaches
// its metadata, which must be a JSON value (or nil).
type ItemWriter interface {
	io.Writer
	Finish(metadata []byte) error
}

// ItemSink is handed to a plugin's Backup to emit items, one at a time.
type ItemSink interface {
	// Item starts a new item; the previous one must have been finished.
	Item(path string) (ItemWriter, error)
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
	sink := &streamSink{stream: stream}
	err := b.Backup(stream.Context(), BackupRequest{Selection: req.Selection, Kind: req.Kind, Options: req.Options}, sink)
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
	stream grpc.ServerStreamingServer[pb.BackupMessage]
	open   *streamItem
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
// unread remainder is discarded.
type ItemHandler func(path string, data io.Reader) error

// Backup runs a plugin backup, calling handle for each item in order. It
// returns the items received, each with its size and metadata.
func (c *WorkloadGRPCClient) Backup(ctx context.Context, req BackupRequest, handle ItemHandler) ([]BackupItem, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := c.client.Backup(ctx, &pb.BackupRequest{Selection: req.Selection, Kind: req.Kind, Options: req.Options})
	if err != nil {
		return nil, fmt.Errorf("plugin: backup: %w", err)
	}
	r := &itemReader{stream: stream}

	var items []BackupItem
	seen := make(map[string]bool)
	for {
		msg, err := r.next()
		if err == io.EOF {
			return items, nil
		}
		if err != nil {
			return nil, err
		}
		start := msg.GetStart()
		if start == nil {
			return nil, fmt.Errorf("plugin: backup: expected item start, got %T", msg.Msg)
		}
		if err := ValidItemPath(start.Path); err != nil {
			return nil, err
		}
		if seen[start.Path] {
			return nil, fmt.Errorf("plugin: backup: duplicate item %q", start.Path)
		}
		seen[start.Path] = true

		r.begin()
		if err := handle(start.Path, r); err != nil {
			return nil, err
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			return nil, err
		}
		items = append(items, BackupItem{Path: start.Path, Size: r.size, Metadata: r.metadata})
	}
}

// itemReader exposes the data messages of the current item as an io.Reader
// and stops at its ItemEnd.
type itemReader struct {
	stream   grpc.ServerStreamingClient[pb.BackupMessage]
	buf      []byte
	inItem   bool
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
	r.inItem, r.buf, r.size, r.metadata = true, nil, 0, nil
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
