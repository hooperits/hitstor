package agent

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginmgr"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

// PluginObjectStore keeps the settings saved for plugin objects, such as
// the credentials of a database instance. The daemon wires it to the
// agent's plugin manager.
type PluginObjectStore interface {
	Get(id string) (*pluginmgr.InstalledPlugin, error)
	ObjectConfigs(id string) (map[string]map[string]string, error)
	SetObjectConfig(id, path string, values map[string]string) error
}

// objectFields returns the plugin's fields of scope "object", by key.
func (s *AgentServer) objectFields(id string) (map[string]plugin.FieldSpec, error) {
	if s.PluginObjects == nil {
		return nil, status.Error(codes.Unavailable, "agent: plugin object settings are not available on this agent")
	}
	p, err := s.PluginObjects.Get(id)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "agent: %v", err)
	}
	out := map[string]plugin.FieldSpec{}
	for _, f := range p.Fields {
		if f.Scope == plugin.FieldScopeObject {
			out[f.Key] = f
		}
	}
	return out, nil
}

// GetWorkloadObjectConfig implements AgentService.GetWorkloadObjectConfig.
func (s *AgentServer) GetWorkloadObjectConfig(_ context.Context, req *pb.GetWorkloadObjectConfigRequest) (*pb.GetWorkloadObjectConfigResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	fields, err := s.objectFields(req.Plugin)
	if err != nil {
		return nil, err
	}
	all, err := s.PluginObjects.ObjectConfigs(req.Plugin)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "agent: %v", err)
	}
	saved, ok := all[req.Path]
	resp := &pb.GetWorkloadObjectConfigResponse{Configured: ok, Values: map[string]string{}}
	for k, v := range saved {
		if fields[k].Type == plugin.FieldTypePassword {
			resp.SecretsSet = append(resp.SecretsSet, k)
			continue
		}
		resp.Values[k] = v
	}
	slices.Sort(resp.SecretsSet)
	return resp, nil
}

// SetWorkloadObjectConfig implements AgentService.SetWorkloadObjectConfig.
// New settings are tested by browsing the object with them; on failure the
// previous settings are put back.
func (s *AgentServer) SetWorkloadObjectConfig(ctx context.Context, req *pb.SetWorkloadObjectConfigRequest) (*pb.SetWorkloadObjectConfigResponse, error) {
	if err := s.verifyToken(req.Token); err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "agent: object path is required")
	}
	fields, err := s.objectFields(req.Plugin)
	if err != nil {
		return nil, err
	}
	all, err := s.PluginObjects.ObjectConfigs(req.Plugin)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "agent: %v", err)
	}
	previous, hadPrevious := all[req.Path]

	if req.Remove {
		if err := s.PluginObjects.SetObjectConfig(req.Plugin, req.Path, nil); err != nil {
			return nil, status.Errorf(codes.Internal, "agent: %v", err)
		}
		slog.Info("plugin object settings removed", "plugin", req.Plugin, "object", req.Path)
		return &pb.SetWorkloadObjectConfigResponse{}, nil
	}

	values := map[string]string{}
	for k, v := range req.Values {
		f, ok := fields[k]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "agent: plugin %s has no object setting %q", req.Plugin, k)
		}
		switch {
		case v != "":
			values[k] = v
		case f.Type == plugin.FieldTypePassword && previous[k] != "":
			values[k] = previous[k] // empty keeps the saved secret
		}
	}
	if err := s.PluginObjects.SetObjectConfig(req.Plugin, req.Path, values); err != nil {
		return nil, status.Errorf(codes.Internal, "agent: %v", err)
	}

	msg, err := s.testObject(ctx, req)
	if err != nil {
		var restore map[string]string
		if hadPrevious {
			restore = previous
		}
		if rerr := s.PluginObjects.SetObjectConfig(req.Plugin, req.Path, restore); rerr != nil {
			slog.Error("could not restore plugin object settings", "plugin", req.Plugin, "object", req.Path, "error", rerr)
		}
		return nil, err
	}
	slog.Info("plugin object settings saved", "plugin", req.Plugin, "object", req.Path)
	return &pb.SetWorkloadObjectConfigResponse{Message: msg}, nil
}

// testObject browses the object with the saved settings.
func (s *AgentServer) testObject(ctx context.Context, req *pb.SetWorkloadObjectConfigRequest) (string, error) {
	p, release, err := s.openWorkload(req.Plugin, req.Settings)
	if err != nil {
		return "", err
	}
	defer release()
	entries, err := p.Browse(ctx, req.Path)
	if err != nil {
		return "", status.Errorf(codes.FailedPrecondition, "agent: test %s: %v", req.Path, err)
	}
	return fmt.Sprintf("%d objects", len(entries)), nil
}

// markConfigurable sets which browse entries take object settings and
// which have them saved.
func (s *AgentServer) markConfigurable(id string, entries []*pb.WorkloadObject) {
	if s.PluginObjects == nil {
		return
	}
	p, err := s.PluginObjects.Get(id)
	if err != nil {
		return
	}
	kinds := map[string]bool{}
	for _, f := range p.Fields {
		if f.Scope == plugin.FieldScopeObject {
			for _, k := range f.ObjectKinds {
				kinds[k] = true
			}
		}
	}
	if len(kinds) == 0 {
		return
	}
	saved, err := s.PluginObjects.ObjectConfigs(id)
	if err != nil {
		slog.Warn("could not read plugin object settings", "plugin", id, "error", err)
	}
	for _, e := range entries {
		e.Configurable = kinds[e.Kind]
		_, e.Configured = saved[e.Path]
	}
}
