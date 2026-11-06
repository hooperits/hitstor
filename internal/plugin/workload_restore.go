package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
)

// RestoreCandidate is an item available for restore in some snapshot.
type RestoreCandidate struct {
	// Ref is chosen by the host and returned in the plan.
	Ref       string          `json:"ref"`
	Item      string          `json:"item"`
	Kind      string          `json:"kind,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// PlanRestoreRequest asks a plugin which candidates restore Object to
// Target (zero means the latest point available).
type PlanRestoreRequest struct {
	Object     string
	Target     time.Time
	Options    map[string]string
	Candidates []RestoreCandidate
	// Language for the preview, e.g. "es"; English when empty.
	Language string
}

// RestoreFile is one file a restore writes, as the plugin plans it. The
// operator can change Target with the restore option "file:<Key>".
type RestoreFile struct {
	Key    string `json:"key"`
	Kind   string `json:"kind,omitempty"`
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
}

// RestorePreview tells the operator what a planned restore will do.
type RestorePreview struct {
	Files    []RestoreFile
	Warnings []string
	Summary  string
}

// RestorePlan is a plan with its preview.
type RestorePlan struct {
	Refs []string
	RestorePreview
}

// RestorePreviewer is implemented by restorers that can describe a planned
// restore before it runs: where files go, and what to watch out for.
type RestorePreviewer interface {
	PreviewRestore(ctx context.Context, req PlanRestoreRequest, refs []string) (RestorePreview, error)
}

// RestoreRequest describes a restore whose items follow.
type RestoreRequest struct {
	Object    string
	Target    time.Time
	Options   map[string]string
	ItemCount int
}

// RestoreItem is one planned item and its data.
type RestoreItem struct {
	Ref      string
	Item     string
	Metadata json.RawMessage
	Data     io.Reader
}

// RestoreItems yields planned items in order. Next returns io.EOF after the
// last one. Unread data of an item is discarded when Next is called again.
type RestoreItems interface {
	Next() (*RestoreItem, error)
}

// WorkloadRestorer is implemented by plugins that can restore.
type WorkloadRestorer interface {
	PlanRestore(ctx context.Context, req PlanRestoreRequest) ([]string, error)
	// Restore applies the items and returns a one-line summary.
	Restore(ctx context.Context, req RestoreRequest, items RestoreItems) (string, error)
}

func unixNanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func fromUnixNanos(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

// --- Plugin side ---

func (s *workloadGRPCServer) PlanRestore(ctx context.Context, req *pb.PlanRestoreRequest) (*pb.PlanRestoreResponse, error) {
	r, ok := s.impl.(WorkloadRestorer)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "restore not supported")
	}
	cands := make([]RestoreCandidate, 0, len(req.Candidates))
	for _, c := range req.Candidates {
		cands = append(cands, RestoreCandidate{Ref: c.Ref, Item: c.Item, Kind: c.Kind, CreatedAt: fromUnixNanos(c.CreatedUnixNanos), Metadata: c.Metadata})
	}
	pr := PlanRestoreRequest{Object: req.Object, Target: fromUnixNanos(req.TargetUnixNanos), Options: req.Options, Candidates: cands, Language: req.Language}
	refs, err := r.PlanRestore(ctx, pr)
	if err != nil {
		return nil, err
	}
	resp := &pb.PlanRestoreResponse{Refs: refs}
	if p, ok := s.impl.(RestorePreviewer); ok && len(refs) > 0 {
		preview, err := p.PreviewRestore(ctx, pr, refs)
		if err != nil {
			// The plan stands; the restore itself will report the problem.
			preview.Warnings = append(preview.Warnings, err.Error())
		}
		resp.Warnings, resp.Summary = preview.Warnings, preview.Summary
		for _, f := range preview.Files {
			resp.Files = append(resp.Files, &pb.RestoreFile{Key: f.Key, Kind: f.Kind, Source: f.Source, Target: f.Target})
		}
	}
	return resp, nil
}

func (s *workloadGRPCServer) Restore(stream grpc.ClientStreamingServer[pb.RestoreMessage, pb.RestoreResult]) error {
	r, ok := s.impl.(WorkloadRestorer)
	if !ok {
		return status.Error(codes.Unimplemented, "restore not supported")
	}
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	begin := first.GetBegin()
	if begin == nil {
		return fmt.Errorf("plugin: restore: first message must be RestoreBegin")
	}
	items := &recvItems{recv: stream.Recv}
	msg, err := r.Restore(stream.Context(), RestoreRequest{
		Object: begin.Object, Target: fromUnixNanos(begin.TargetUnixNanos), Options: begin.Options, ItemCount: int(begin.ItemCount),
	}, items)
	if err != nil {
		return err
	}
	// A plugin that returns success must have applied every item.
	if _, err := items.Next(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("plugin: restore returned before consuming all items")
		}
		return err
	}
	return stream.SendAndClose(&pb.RestoreResult{Message: msg})
}

// recvItems reads RestoreMessage frames as a RestoreItems iterator.
type recvItems struct {
	recv func() (*pb.RestoreMessage, error)
	cur  *restoreDataReader
	done bool
}

func (it *recvItems) Next() (*RestoreItem, error) {
	if it.done {
		return nil, io.EOF
	}
	if it.cur != nil {
		if _, err := io.Copy(io.Discard, it.cur); err != nil {
			return nil, err
		}
		it.cur = nil
	}
	msg, err := it.recv()
	if err == io.EOF {
		it.done = true
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}
	start := msg.GetStart()
	if start == nil {
		return nil, fmt.Errorf("plugin: restore: expected item start, got %T", msg.Msg)
	}
	it.cur = &restoreDataReader{recv: it.recv}
	return &RestoreItem{Ref: start.Ref, Item: start.Item, Metadata: start.Metadata, Data: it.cur}, nil
}

// restoreDataReader yields an item's data messages up to its RestoreItemEnd.
type restoreDataReader struct {
	recv func() (*pb.RestoreMessage, error)
	buf  []byte
	done bool
}

func (r *restoreDataReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		if r.done {
			return 0, io.EOF
		}
		msg, err := r.recv()
		if err == io.EOF {
			return 0, fmt.Errorf("plugin: restore stream ended inside an item")
		}
		if err != nil {
			return 0, err
		}
		switch m := msg.Msg.(type) {
		case *pb.RestoreMessage_Data:
			r.buf = m.Data
		case *pb.RestoreMessage_End:
			r.done = true
		default:
			return 0, fmt.Errorf("plugin: restore: unexpected %T inside an item", msg.Msg)
		}
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// --- Host side ---

// PlanRestore asks the plugin which candidates to apply. It checks that the
// plan only references known candidates, each at most once.
func (c *WorkloadGRPCClient) PlanRestore(ctx context.Context, req PlanRestoreRequest) ([]string, error) {
	plan, err := c.PlanRestoreDetail(ctx, req)
	return plan.Refs, err
}

// PlanRestoreDetail asks the plugin for a plan and its preview.
func (c *WorkloadGRPCClient) PlanRestoreDetail(ctx context.Context, req PlanRestoreRequest) (RestorePlan, error) {
	pr := &pb.PlanRestoreRequest{Object: req.Object, TargetUnixNanos: unixNanos(req.Target), Options: req.Options, Language: req.Language}
	known := make(map[string]bool, len(req.Candidates))
	for _, cand := range req.Candidates {
		known[cand.Ref] = true
		pr.Candidates = append(pr.Candidates, &pb.RestoreCandidate{
			Ref: cand.Ref, Item: cand.Item, Kind: cand.Kind, CreatedUnixNanos: unixNanos(cand.CreatedAt), Metadata: cand.Metadata,
		})
	}
	resp, err := c.client.PlanRestore(ctx, pr)
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return RestorePlan{}, ErrUnsupported
		}
		return RestorePlan{}, fmt.Errorf("plugin: plan restore: %w", err)
	}
	if len(resp.Refs) == 0 {
		return RestorePlan{}, errors.New("plugin: plan restore: nothing to restore")
	}
	seen := make(map[string]bool, len(resp.Refs))
	for _, ref := range resp.Refs {
		if !known[ref] {
			return RestorePlan{}, fmt.Errorf("plugin: plan restore: unknown candidate %q", ref)
		}
		if seen[ref] {
			return RestorePlan{}, fmt.Errorf("plugin: plan restore: candidate %q planned twice", ref)
		}
		seen[ref] = true
	}
	plan := RestorePlan{Refs: resp.Refs}
	plan.Warnings, plan.Summary = resp.Warnings, resp.Summary
	for _, f := range resp.Files {
		plan.Files = append(plan.Files, RestoreFile{Key: f.Key, Kind: f.Kind, Source: f.Source, Target: f.Target})
	}
	return plan, nil
}

// Restore streams the items to the plugin and returns its summary.
func (c *WorkloadGRPCClient) Restore(ctx context.Context, req RestoreRequest, items RestoreItems) (string, error) {
	stream, err := c.client.Restore(ctx)
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return "", ErrUnsupported
		}
		return "", fmt.Errorf("plugin: restore: %w", err)
	}
	if err := c.sendRestore(stream, req, items); err != nil {
		// io.EOF from Send means the plugin ended the call; its own error
		// comes from CloseAndRecv.
		if !errors.Is(err, io.EOF) {
			stream.CloseSend()
			return "", err
		}
	}
	res, err := stream.CloseAndRecv()
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return "", ErrUnsupported
		}
		return "", fmt.Errorf("plugin: restore: %w", err)
	}
	return res.Message, nil
}

func (c *WorkloadGRPCClient) sendRestore(stream grpc.ClientStreamingClient[pb.RestoreMessage, pb.RestoreResult], req RestoreRequest, items RestoreItems) error {
	if err := stream.Send(&pb.RestoreMessage{Msg: &pb.RestoreMessage_Begin{Begin: &pb.RestoreBegin{
		Object: req.Object, TargetUnixNanos: unixNanos(req.Target), Options: req.Options, ItemCount: int32(req.ItemCount),
	}}}); err != nil {
		return err
	}
	buf := make([]byte, itemChunkSize)
	for {
		it, err := items.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&pb.RestoreMessage{Msg: &pb.RestoreMessage_Start{Start: &pb.RestoreItemStart{
			Ref: it.Ref, Item: it.Item, Metadata: it.Metadata,
		}}}); err != nil {
			return err
		}
		for {
			n, rerr := it.Data.Read(buf)
			if n > 0 {
				data := append([]byte(nil), buf[:n]...)
				if err := stream.Send(&pb.RestoreMessage{Msg: &pb.RestoreMessage_Data{Data: data}}); err != nil {
					return err
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				return fmt.Errorf("plugin: restore: read item %s: %w", it.Item, rerr)
			}
		}
		if err := stream.Send(&pb.RestoreMessage{Msg: &pb.RestoreMessage_End{End: &pb.RestoreItemEnd{}}}); err != nil {
			return err
		}
	}
}

// SliceItems adapts a fixed list of items to RestoreItems.
type SliceItems []*RestoreItem

// Next implements RestoreItems.
func (s *SliceItems) Next() (*RestoreItem, error) {
	if len(*s) == 0 {
		return nil, io.EOF
	}
	it := (*s)[0]
	*s = (*s)[1:]
	return it, nil
}
