package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

// handleAPIWorkloadBrowse handles GET /api/agents/{name}/plugins/{plugin}/browse?path=.
func (d *Dashboard) handleAPIWorkloadBrowse(w http.ResponseWriter, r *http.Request) {
	name, pluginID := r.PathValue("name"), r.PathValue("plugin")
	agentRec := d.registry.GetAgent(name)
	if agentRec == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !agentHasPlugin(agentRec, pluginID) {
		http.Error(w, fmt.Sprintf("agent %q has not reported plugin %q", name, pluginID), http.StatusBadRequest)
		return
	}
	token, err := d.registry.DecryptToken(name)
	if err != nil {
		http.Error(w, "agent token unavailable", http.StatusInternalServerError)
		return
	}
	client, err := d.dialManagedAgent(name, agentRec.AgentAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("dial agent: %v", err), http.StatusBadGateway)
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	resp, err := client.BrowseWorkload(ctx, &pb.BrowseWorkloadRequest{
		Token: token, Plugin: pluginID, Settings: d.workloadSettings(name, pluginID), Path: r.URL.Query().Get("path"),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("browse: %v", err), http.StatusBadGateway)
		return
	}
	type entry struct {
		Path        string `json:"path"`
		Name        string `json:"name"`
		Kind        string `json:"kind,omitempty"`
		Selectable  bool   `json:"selectable"`
		HasChildren bool   `json:"has_children"`
	}
	out := make([]entry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		out = append(out, entry{e.Path, e.Name, e.Kind, e.Selectable, e.HasChildren})
	}
	writeJSON(w, out)
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
		StartTime: time.Now(), status: "planning", settings: settings,
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
