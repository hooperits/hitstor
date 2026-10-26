package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (d *Dashboard) handleDisasterPage(w http.ResponseWriter, r *http.Request) {
	data := d.newPageData(
		"Disaster Recovery",
		"Guided full-system restore for a protected host",
		"disaster",
		nil,
	)
	d.render(w, "disaster", data)
}

func (d *Dashboard) handleAPIDisasterPlans(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, d.listDisasterPlanSummaries())
}

func (d *Dashboard) handleAPIDisasterPlan(w http.ResponseWriter, r *http.Request) {
	agent := r.PathValue("agent")
	if agent == "" {
		http.Error(w, "agent is required", http.StatusBadRequest)
		return
	}
	plan, err := d.buildDisasterPlan(agent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, plan)
}

func (d *Dashboard) handleAPIDisasterRecover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceAgent string   `json:"source_agent"`
		TargetAgent string   `json:"target_agent"`
		Mode        string   `json:"mode"`
		StagingRoot string   `json:"staging_root"`
		JobIDs      []string `json:"job_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.SourceAgent = strings.TrimSpace(req.SourceAgent)
	req.TargetAgent = strings.TrimSpace(req.TargetAgent)
	if req.SourceAgent == "" {
		http.Error(w, "source_agent is required", http.StatusBadRequest)
		return
	}
	rec, err := d.startDisasterRecovery(req.SourceAgent, req.TargetAgent, req.Mode, req.StagingRoot, req.JobIDs)
	if err != nil {
		// Client errors vs server
		msg := err.Error()
		status := http.StatusBadRequest
		if strings.Contains(msg, "not initialized") {
			status = http.StatusInternalServerError
		}
		http.Error(w, msg, status)
		return
	}
	writeJSON(w, rec.snapshot())
}

func (d *Dashboard) handleAPIDisasterRecoveries(w http.ResponseWriter, r *http.Request) {
	if d.disaster == nil {
		writeJSON(w, []any{})
		return
	}
	list := d.disaster.List()
	out := make([]DisasterRecoveryView, 0, len(list))
	for _, rec := range list {
		out = append(out, rec.snapshot())
	}
	writeJSON(w, out)
}

func (d *Dashboard) handleAPIDisasterRecovery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if d.disaster == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rec := d.disaster.Get(id)
	if rec == nil {
		http.Error(w, "recovery not found", http.StatusNotFound)
		return
	}
	writeJSON(w, rec.snapshot())
}
