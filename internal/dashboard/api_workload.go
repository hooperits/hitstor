package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"google.golang.org/grpc/status"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/server"
	"github.com/hooperits/hitstor/internal/users"
)

// workloadAgent resolves the agent and plugin named in the request path and
// dials the agent. On failure it writes the response and returns nil.
func (d *Dashboard) workloadAgent(w http.ResponseWriter, r *http.Request) (client *server.AgentClient, token string) {
	name, pluginID := r.PathValue("name"), r.PathValue("plugin")
	agentRec := d.registry.GetAgent(name)
	if agentRec == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return nil, ""
	}
	if !agentHasPlugin(agentRec, pluginID) {
		http.Error(w, fmt.Sprintf("agent %q has not reported plugin %q", name, pluginID), http.StatusBadRequest)
		return nil, ""
	}
	token, err := d.registry.DecryptToken(name)
	if err != nil {
		http.Error(w, "agent token unavailable", http.StatusInternalServerError)
		return nil, ""
	}
	client, err = d.dialManagedAgent(name, agentRec.AgentAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("dial agent: %v", err), http.StatusBadGateway)
		return nil, ""
	}
	return client, token
}

// agentErrorMessage drops the gRPC framing from an agent error.
func agentErrorMessage(err error) string {
	if s, ok := status.FromError(err); ok {
		return strings.TrimPrefix(s.Message(), "agent: ")
	}
	return err.Error()
}

// handleAPIWorkloadBrowse handles GET /api/agents/{name}/plugins/{plugin}/browse?path=.
func (d *Dashboard) handleAPIWorkloadBrowse(w http.ResponseWriter, r *http.Request) {
	client, token := d.workloadAgent(w, r)
	if client == nil {
		return
	}
	defer client.Close()
	name, pluginID := r.PathValue("name"), r.PathValue("plugin")

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	resp, err := client.BrowseWorkload(ctx, &pb.BrowseWorkloadRequest{
		Token: token, Plugin: pluginID, Settings: d.workloadSettings(name, pluginID), Path: r.URL.Query().Get("path"),
	})
	if err != nil {
		http.Error(w, agentErrorMessage(err), http.StatusBadGateway)
		return
	}
	type entry struct {
		Path         string `json:"path"`
		Name         string `json:"name"`
		Kind         string `json:"kind,omitempty"`
		Selectable   bool   `json:"selectable"`
		HasChildren  bool   `json:"has_children"`
		Detail       string `json:"detail,omitempty"`
		Status       string `json:"status,omitempty"`
		Configurable bool   `json:"configurable,omitempty"`
		Configured   bool   `json:"configured,omitempty"`
	}
	out := make([]entry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		out = append(out, entry{e.Path, e.Name, e.Kind, e.Selectable, e.HasChildren, e.Detail, e.Status, e.Configurable, e.Configured})
	}
	writeJSON(w, out)
}

type objectConfigJSON struct {
	Configured bool              `json:"configured"`
	Values     map[string]string `json:"values"`
	SecretsSet []string          `json:"secrets_set"`
}

// handleAPIGetObjectConfig handles
// GET /api/agents/{name}/plugins/{plugin}/object-config?path=: the
// settings saved on the agent for one object, without secrets.
func (d *Dashboard) handleAPIGetObjectConfig(w http.ResponseWriter, r *http.Request) {
	client, token := d.workloadAgent(w, r)
	if client == nil {
		return
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := client.GetWorkloadObjectConfig(ctx, &pb.GetWorkloadObjectConfigRequest{
		Token: token, Plugin: r.PathValue("plugin"), Path: r.URL.Query().Get("path"),
	})
	if err != nil {
		http.Error(w, agentErrorMessage(err), http.StatusBadGateway)
		return
	}
	out := objectConfigJSON{Configured: resp.Configured, Values: resp.Values, SecretsSet: resp.SecretsSet}
	if out.Values == nil {
		out.Values = map[string]string{}
	}
	if out.SecretsSet == nil {
		out.SecretsSet = []string{}
	}
	writeJSON(w, out)
}

type setObjectConfigRequest struct {
	Path   string            `json:"path"`
	Values map[string]string `json:"values"`
	Remove bool              `json:"remove"`
}

// handleAPISetObjectConfig handles
// PUT /api/agents/{name}/plugins/{plugin}/object-config: the agent tests
// the settings and saves them, secrets encrypted. The server keeps nothing.
func (d *Dashboard) handleAPISetObjectConfig(w http.ResponseWriter, r *http.Request) {
	var req setObjectConfigRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	client, token := d.workloadAgent(w, r)
	if client == nil {
		return
	}
	defer client.Close()
	name, pluginID := r.PathValue("name"), r.PathValue("plugin")
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	resp, err := client.SetWorkloadObjectConfig(ctx, &pb.SetWorkloadObjectConfigRequest{
		Token: token, Plugin: pluginID, Path: req.Path, Values: req.Values, Remove: req.Remove,
		Settings: d.workloadSettings(name, pluginID),
	})
	if err != nil {
		http.Error(w, agentErrorMessage(err), http.StatusBadRequest)
		return
	}
	action := "plugin object settings saved"
	if req.Remove {
		action = "plugin object settings removed"
	}
	slog.Info(action, "agent", name, "plugin", pluginID, "object", req.Path)
	writeJSON(w, map[string]string{"message": resp.Message})
}

// handleAPIWorkloadPoints handles GET /api/workload/points?agent=&plugin=&object=:
// the items available to restore an object, oldest first.
func (d *Dashboard) handleAPIWorkloadPoints(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	agentName, pluginID, object := q.Get("agent"), q.Get("plugin"), q.Get("object")
	if agentName == "" || pluginID == "" || object == "" {
		http.Error(w, "agent, plugin, and object are required", http.StatusBadRequest)
		return
	}
	points, err := d.workloadPoints(agentName, pluginID, object)
	if err != nil {
		http.Error(w, fmt.Sprintf("list restore points: %v", err), http.StatusInternalServerError)
		return
	}
	if points == nil {
		points = []workloadPoint{}
	}
	writeJSON(w, points)
}

type startWorkloadRestoreRequest struct {
	// AgentName is where the restore runs; SourceAgent (default: the same)
	// is the agent whose backups are restored.
	AgentName      string            `json:"agent_name"`
	SourceAgent    string            `json:"source_agent"`
	Plugin         string            `json:"plugin"`
	Object         string            `json:"object"`
	Target         *time.Time        `json:"target"`
	Options        map[string]string `json:"options"`
	PluginSettings map[string]string `json:"plugin_settings"`
}

// handleAPIStartWorkloadRestore handles POST /api/workload/restores.
func (d *Dashboard) handleAPIStartWorkloadRestore(w http.ResponseWriter, r *http.Request) {
	var req startWorkloadRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.AgentName == "" || req.Plugin == "" || req.Object == "" {
		http.Error(w, "agent_name, plugin, and object are required", http.StatusBadRequest)
		return
	}
	if req.SourceAgent == "" {
		req.SourceAgent = req.AgentName
	}
	if !d.requireAgent(w, r, users.PermRestore, req.SourceAgent) || !d.requireAgent(w, r, users.PermRestore, req.AgentName) {
		return
	}
	agentRec := d.registry.GetAgent(req.AgentName)
	if agentRec == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !agentHasPlugin(agentRec, req.Plugin) {
		http.Error(w, fmt.Sprintf("agent %q has not reported plugin %q", req.AgentName, req.Plugin), http.StatusBadRequest)
		return
	}
	if d.registry.GetAgent(req.SourceAgent) == nil {
		http.Error(w, "source agent not found", http.StatusNotFound)
		return
	}

	points, err := d.workloadPoints(req.SourceAgent, req.Plugin, req.Object)
	if err != nil {
		http.Error(w, fmt.Sprintf("list restore points: %v", err), http.StatusInternalServerError)
		return
	}
	if len(points) == 0 {
		http.Error(w, "no backups of this object were found", http.StatusNotFound)
		return
	}

	settings := req.PluginSettings
	if settings == nil {
		settings = d.workloadSettings(req.AgentName, req.Plugin)
	}
	sess := &WorkloadRestoreSession{
		ID: newWorkloadRestoreID(), AgentName: req.AgentName, SourceAgent: req.SourceAgent,
		Plugin: req.Plugin, Object: req.Object, Target: req.Target, Options: req.Options,
		StartTime: time.Now(), status: "planning", settings: settings, language: d.language(r),
	}
	d.workloadRestores.Store(sess.ID, sess)
	// The restore outlives the HTTP request.
	go d.runWorkloadRestore(context.Background(), sess, points)

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, sess.snapshot())
}

// handleAPIGetWorkloadRestore handles GET /api/workload/restores/{id}.
func (d *Dashboard) handleAPIGetWorkloadRestore(w http.ResponseWriter, r *http.Request) {
	v, ok := d.workloadRestores.Load(r.PathValue("id"))
	if !ok {
		http.Error(w, "restore not found", http.StatusNotFound)
		return
	}
	writeJSON(w, v.(*WorkloadRestoreSession).snapshot())
}

// catalogPoint is a restore point with the object it belongs to.
type catalogPoint struct {
	Object string `json:"object"`
	workloadPoint
}

// handleAPIWorkloadCatalog handles GET /api/workload/catalog?agent=&plugin=&snapshot=:
// every backup item a plugin produced on an agent, newest first, optionally
// only those in one snapshot (an ID or prefix).
func (d *Dashboard) handleAPIWorkloadCatalog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	agentName, pluginID := q.Get("agent"), q.Get("plugin")
	if agentName == "" || pluginID == "" {
		http.Error(w, "agent and plugin are required", http.StatusBadRequest)
		return
	}
	points, err := d.workloadItems(agentName, pluginID, "", q.Get("snapshot"))
	if err != nil {
		http.Error(w, fmt.Sprintf("list backups: %v", err), http.StatusInternalServerError)
		return
	}
	out := make([]catalogPoint, 0, len(points))
	for i := len(points) - 1; i >= 0; i-- {
		out = append(out, catalogPoint{Object: path.Dir(points[i].Item), workloadPoint: points[i]})
	}
	writeJSON(w, out)
}

// handleAPIWorkloadRestorePlan handles POST /api/workload/restore-plan: what
// a restore with these parameters would apply and do, without running it.
func (d *Dashboard) handleAPIWorkloadRestorePlan(w http.ResponseWriter, r *http.Request) {
	var req startWorkloadRestoreRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.AgentName == "" || req.Plugin == "" || req.Object == "" {
		http.Error(w, "agent_name, plugin, and object are required", http.StatusBadRequest)
		return
	}
	if req.SourceAgent == "" {
		req.SourceAgent = req.AgentName
	}
	if !d.requireAgent(w, r, users.PermRestore, req.SourceAgent) || !d.requireAgent(w, r, users.PermRestore, req.AgentName) {
		return
	}
	agentRec := d.registry.GetAgent(req.AgentName)
	if agentRec == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !agentHasPlugin(agentRec, req.Plugin) {
		http.Error(w, fmt.Sprintf("agent %q has not reported plugin %q", req.AgentName, req.Plugin), http.StatusBadRequest)
		return
	}
	points, err := d.workloadPoints(req.SourceAgent, req.Plugin, req.Object)
	if err != nil {
		http.Error(w, fmt.Sprintf("list restore points: %v", err), http.StatusInternalServerError)
		return
	}
	if len(points) == 0 {
		http.Error(w, "no backups of this object were found", http.StatusNotFound)
		return
	}
	token, err := d.registry.DecryptToken(req.AgentName)
	if err != nil {
		http.Error(w, "agent token unavailable", http.StatusInternalServerError)
		return
	}
	client, err := d.dialManagedAgent(req.AgentName, agentRec.AgentAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("dial agent: %v", err), http.StatusBadGateway)
		return
	}
	defer client.Close()
	settings := req.PluginSettings
	if settings == nil {
		settings = d.workloadSettings(req.AgentName, req.Plugin)
	}
	sess := &WorkloadRestoreSession{Plugin: req.Plugin, Object: req.Object, Target: req.Target, Options: req.Options,
		settings: settings, language: d.language(r)}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	plan, planned, err := d.planWorkloadRestore(ctx, client, token, sess, points)
	if err != nil {
		http.Error(w, agentErrorMessage(err), http.StatusBadRequest)
		return
	}
	type file struct {
		Key    string `json:"key"`
		Kind   string `json:"kind,omitempty"`
		Source string `json:"source,omitempty"`
		Target string `json:"target"`
	}
	out := struct {
		Summary  string          `json:"summary"`
		Warnings []string        `json:"warnings"`
		Files    []file          `json:"files"`
		Items    []workloadPoint `json:"items"`
	}{Summary: plan.Summary, Warnings: plan.Warnings, Items: planned, Files: []file{}}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	for _, f := range plan.Files {
		out.Files = append(out.Files, file{f.Key, f.Kind, f.Source, f.Target})
	}
	writeJSON(w, out)
}
