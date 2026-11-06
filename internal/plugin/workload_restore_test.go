package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// fakeRestorer records what it was asked to plan and restore.
type fakeRestorer struct {
	metaOnly
	plan      []string
	mode      string // "", "fail", "partial"
	gotPlan   PlanRestoreRequest
	gotReq    RestoreRequest
	restored  map[string][]byte
	metaByRef map[string]string
}

func (f *fakeRestorer) PlanRestore(_ context.Context, req PlanRestoreRequest) ([]string, error) {
	f.gotPlan = req
	return f.plan, nil
}

func (f *fakeRestorer) Restore(_ context.Context, req RestoreRequest, items RestoreItems) (string, error) {
	f.gotReq = req
	f.restored = map[string][]byte{}
	f.metaByRef = map[string]string{}
	for {
		it, err := items.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if f.mode == "partial" {
			return "stopped early", nil
		}
		data, err := io.ReadAll(it.Data)
		if err != nil {
			return "", err
		}
		f.restored[it.Ref] = data
		f.metaByRef[it.Ref] = string(it.Metadata)
		if f.mode == "fail" {
			return "", errors.New("RESTORE DATABASE failed")
		}
	}
	return fmt.Sprintf("restored %d items", len(f.restored)), nil
}

func TestWorkloadPlanRestore(t *testing.T) {
	target := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	impl := &fakeRestorer{plan: []string{"b", "a"}}
	c := setupWorkloadBufconn(t, impl)
	refs, err := c.PlanRestore(context.Background(), PlanRestoreRequest{
		Object: "inst/db", Target: target, Options: map[string]string{"new_name": "db2"},
		Candidates: []RestoreCandidate{
			{Ref: "a", Item: "inst/db/full.bak", Kind: "full", CreatedAt: target.Add(-time.Hour), Metadata: []byte(`{"x":1}`)},
			{Ref: "b", Item: "inst/db/log.trn", Kind: "log", CreatedAt: target},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(refs, ",") != "b,a" {
		t.Fatalf("refs = %v", refs)
	}
	g := impl.gotPlan
	if g.Object != "inst/db" || !g.Target.Equal(target) || g.Options["new_name"] != "db2" || len(g.Candidates) != 2 ||
		g.Candidates[0].Kind != "full" || !g.Candidates[0].CreatedAt.Equal(target.Add(-time.Hour)) || string(g.Candidates[0].Metadata) != `{"x":1}` {
		t.Fatalf("plugin got %+v", g)
	}

	// Latest point: zero target round-trips as zero.
	if _, err := c.PlanRestore(context.Background(), PlanRestoreRequest{Candidates: []RestoreCandidate{{Ref: "a"}, {Ref: "b"}}}); err != nil {
		t.Fatal(err)
	}
	if !impl.gotPlan.Target.IsZero() {
		t.Fatalf("target = %v, want zero", impl.gotPlan.Target)
	}
}

func TestWorkloadPlanRestoreRejectsBadPlans(t *testing.T) {
	cands := []RestoreCandidate{{Ref: "a"}, {Ref: "b"}}
	for name, plan := range map[string][]string{"unknown": {"a", "zzz"}, "duplicate": {"a", "a"}, "empty": nil} {
		t.Run(name, func(t *testing.T) {
			c := setupWorkloadBufconn(t, &fakeRestorer{plan: plan})
			if _, err := c.PlanRestore(context.Background(), PlanRestoreRequest{Candidates: cands}); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestWorkloadRestoreStreamsItems(t *testing.T) {
	big := bytes.Repeat([]byte("x"), 3<<20)
	impl := &fakeRestorer{}
	c := setupWorkloadBufconn(t, impl)
	items := SliceItems{
		{Ref: "a", Item: "inst/db/full.bak", Metadata: []byte(`{"k":"full"}`), Data: bytes.NewReader(big)},
		{Ref: "b", Item: "inst/db/log.trn", Data: strings.NewReader("log")},
	}
	msg, err := c.Restore(context.Background(), RestoreRequest{Object: "inst/db", Options: map[string]string{"o": "1"}, ItemCount: 2}, &items)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if msg != "restored 2 items" {
		t.Fatalf("msg = %q", msg)
	}
	if !bytes.Equal(impl.restored["a"], big) || string(impl.restored["b"]) != "log" || impl.metaByRef["a"] != `{"k":"full"}` {
		t.Fatal("restored data mismatch")
	}
	if impl.gotReq.Object != "inst/db" || impl.gotReq.ItemCount != 2 || impl.gotReq.Options["o"] != "1" {
		t.Fatalf("req = %+v", impl.gotReq)
	}
}

func TestWorkloadRestoreErrors(t *testing.T) {
	for mode, want := range map[string]string{"fail": "RESTORE DATABASE failed", "partial": "before consuming"} {
		t.Run(mode, func(t *testing.T) {
			c := setupWorkloadBufconn(t, &fakeRestorer{mode: mode})
			items := SliceItems{
				{Ref: "a", Data: bytes.NewReader(bytes.Repeat([]byte("y"), 2<<20))},
				{Ref: "b", Data: strings.NewReader("z")},
			}
			_, err := c.Restore(context.Background(), RestoreRequest{ItemCount: 2}, &items)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestWorkloadRestoreUnsupported(t *testing.T) {
	c := setupWorkloadBufconn(t, metaOnly{})
	if _, err := c.PlanRestore(context.Background(), PlanRestoreRequest{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("plan err = %v", err)
	}
	items := SliceItems{}
	if _, err := c.Restore(context.Background(), RestoreRequest{}, &items); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("restore err = %v", err)
	}
}

// previewRestorer also describes its plans.
type previewRestorer struct {
	fakeRestorer
	fail bool
}

func (p *previewRestorer) PreviewRestore(_ context.Context, req PlanRestoreRequest, refs []string) (RestorePreview, error) {
	if p.fail {
		return RestorePreview{}, errors.New("cannot reach the instance")
	}
	return RestorePreview{
		Files:    []RestoreFile{{Key: "db", Kind: "ROWS", Source: `C:\db.mdf`, Target: `D:\db2.mdf`}},
		Warnings: []string{"db2 exists"},
		Summary:  req.Language + ": " + strings.Join(refs, "+"),
	}, nil
}

func TestWorkloadPlanRestorePreview(t *testing.T) {
	impl := &previewRestorer{fakeRestorer: fakeRestorer{plan: []string{"a"}}}
	c := setupWorkloadBufconn(t, impl)
	req := PlanRestoreRequest{Object: "inst/db", Language: "es", Candidates: []RestoreCandidate{{Ref: "a"}}}
	plan, err := c.PlanRestoreDetail(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary != "es: a" || len(plan.Warnings) != 1 || len(plan.Files) != 1 || plan.Files[0].Target != `D:\db2.mdf` || plan.Files[0].Kind != "ROWS" {
		t.Fatalf("plan = %+v", plan)
	}
	// A preview that fails keeps the plan and reports why.
	impl.fail = true
	plan, err = c.PlanRestoreDetail(context.Background(), req)
	if err != nil || len(plan.Refs) != 1 || len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "cannot reach") {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
}
