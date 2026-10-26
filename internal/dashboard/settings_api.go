package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hooperits/hitstor/internal/config"
)

type settingsUpdateResponse struct {
	OK              bool `json:"ok"`
	RestartRequired bool `json:"restart_required,omitempty"`
}

func (d *Dashboard) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	view, err := d.buildSettingsView()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, view)
}

func (d *Dashboard) handleAPISettingsGeneral(w http.ResponseWriter, r *http.Request) {
	var req GeneralSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := d.applyGeneralSettings(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, settingsUpdateResponse{OK: true})
}

func (d *Dashboard) handleAPISettingsSecurity(w http.ResponseWriter, r *http.Request) {
	var req SecuritySettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	restartRequired, err := d.applySecuritySettings(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, settingsUpdateResponse{OK: true, RestartRequired: restartRequired})
}

func (d *Dashboard) handleAPISettingsNetwork(w http.ResponseWriter, r *http.Request) {
	var req NetworkSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	restartRequired, err := d.applyNetworkSettings(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, settingsUpdateResponse{OK: true, RestartRequired: restartRequired})
}

func (d *Dashboard) handleAPISettingsNotifications(w http.ResponseWriter, r *http.Request) {
	var req NotificationsSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := d.applyNotificationsSettings(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, settingsUpdateResponse{OK: true})
}

func (d *Dashboard) handleAPISettingsIntegrations(w http.ResponseWriter, r *http.Request) {
	var req IntegrationsSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := d.applyIntegrationsSettings(req); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "encryption key") {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, settingsUpdateResponse{OK: true})
}

func (d *Dashboard) handleAPISettingsImportEnv(w http.ResponseWriter, r *http.Request) {
	cfg, err := d.requireConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	encKey, hasKey := d.encryptionKey()
	if !hasKey {
		http.Error(w, "no encryption key available (open a local repo first)", http.StatusBadRequest)
		return
	}
	if err := config.ImportIntegrationsFromEnv(cfg, encKey); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := d.saveConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := d.refreshIntegrationsRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, settingsUpdateResponse{OK: true})
}
