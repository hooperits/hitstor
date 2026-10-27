package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/pluginpkg"
	"github.com/hooperits/hitstor/internal/server"
)

// pluginPackageDir is where uploaded plugin packages are kept.
func (d *Dashboard) pluginPackageDir() string {
	if d.pluginPackages != "" {
		return d.pluginPackages
	}
	return defaultPluginPackageDir()
}

// pluginTrust is the server's view of which packages are acceptable. Agents
// apply their own policy again when installing; checking here only gives
// early feedback on upload.
func (d *Dashboard) pluginTrust() (pluginpkg.TrustPolicy, error) {
	var keys []string
	allowUnsigned := false
	if d.cfg != nil {
		keys = d.cfg.PluginTrustedKeys
		allowUnsigned = d.cfg.PluginAllowUnsigned
	}
	return pluginpkg.NewTrustPolicy(keys, allowUnsigned)
}

type pluginPackageJSON struct {
	File        string   `json:"file"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Platforms   []string `json:"platforms"`
	Description string   `json:"description,omitempty"`
	Signer      string   `json:"signer,omitempty"`
	Official    bool     `json:"official"`
	Size        int64    `json:"size"`
	UploadedAt  string   `json:"uploaded_at"`
}

func describePackage(path string) (pluginPackageJSON, error) {
	pkg, err := pluginpkg.ReadFile(path)
	if err != nil {
		return pluginPackageJSON{}, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return pluginPackageJSON{}, err
	}
	m := pkg.Manifest
	return pluginPackageJSON{
		File: filepath.Base(path), ID: m.ID, Name: m.Name, Version: m.Version, Platforms: m.Platforms,
		Description: m.Description, Signer: pkg.Signer(), Official: slices.Contains(pluginpkg.OfficialKeys, pkg.Signer()),
		Size: fi.Size(), UploadedAt: fi.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// validPackageFile rejects anything but a bare .hpk file name.
func validPackageFile(name string) bool {
	return strings.HasSuffix(name, pluginpkg.Extension) && filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/\`) && name != pluginpkg.Extension
}

// handleAPIPluginPackages handles GET /api/plugin-packages.
func (d *Dashboard) handleAPIPluginPackages(w http.ResponseWriter, r *http.Request) {
	matches, _ := filepath.Glob(filepath.Join(d.pluginPackageDir(), "*"+pluginpkg.Extension))
	out := []pluginPackageJSON{}
	for _, p := range matches {
		info, err := describePackage(p)
		if err != nil {
			slog.Warn("unreadable plugin package", "file", p, "error", err)
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	writeJSON(w, out)
}

// maxPackageUpload bounds an uploaded package.
const maxPackageUpload = pluginpkg.MaxTotalSize + 64<<20

// handleAPIUploadPluginPackage handles POST /api/plugin-packages with the
// package as the request body (application/octet-stream).
func (d *Dashboard) handleAPIUploadPluginPackage(w http.ResponseWriter, r *http.Request) {
	dir := d.pluginPackageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		http.Error(w, fmt.Sprintf("create package folder: %v", err), http.StatusInternalServerError)
		return
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		http.Error(w, fmt.Sprintf("store package: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, http.MaxBytesReader(w, r.Body, maxPackageUpload))
	tmp.Close()
	if err != nil {
		http.Error(w, fmt.Sprintf("upload: %v", err), http.StatusBadRequest)
		return
	}
	if n == 0 {
		http.Error(w, "empty upload", http.StatusBadRequest)
		return
	}

	pkg, err := pluginpkg.ReadFile(tmp.Name())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	trust, err := d.pluginTrust()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := pkg.Verify(trust); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	m := pkg.Manifest
	goos, goarch, _ := strings.Cut(m.Platforms[0], "/")
	name := pluginpkg.PackageName(m.ID, m.Version, goos, goarch)
	final := filepath.Join(dir, name)
	if err := os.Rename(tmp.Name(), final); err != nil {
		http.Error(w, fmt.Sprintf("store package: %v", err), http.StatusInternalServerError)
		return
	}
	slog.Info("plugin package uploaded", "file", name, "signed", pkg.Signed())
	info, err := describePackage(final)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, info)
}

// handleAPIDeletePluginPackage handles DELETE /api/plugin-packages/{file}.
func (d *Dashboard) handleAPIDeletePluginPackage(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if !validPackageFile(file) {
		http.Error(w, "invalid package name", http.StatusBadRequest)
		return
	}
	if err := os.Remove(filepath.Join(d.pluginPackageDir(), file)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "package not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// managedAgentClient dials a managed agent and returns its token.
func (d *Dashboard) managedAgentClient(name string) (*server.AgentClient, string, int, error) {
	a := d.registry.GetAgent(name)
	if a == nil {
		return nil, "", http.StatusNotFound, errors.New("agent not found")
	}
	if !a.Managed || a.AgentAddr == "" {
		return nil, "", http.StatusBadRequest, errors.New("only dashboard-managed agents can be managed remotely")
	}
	token, err := d.registry.DecryptToken(name)
	if err != nil {
		return nil, "", http.StatusInternalServerError, fmt.Errorf("decrypt token: %w", err)
	}
	c, err := d.dialManagedAgent(name, a.AgentAddr)
	if err != nil {
		return nil, "", http.StatusBadGateway, fmt.Errorf("connect to agent: %w", err)
	}
	return c, token, 0, nil
}

// wakeAgent asks the agent to poll now, so its plugin inventory refreshes.
func wakeAgent(c *server.AgentClient, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.WakeUp(ctx, token)
}

// handleAPIInstallAgentPlugin handles POST /api/agents/{name}/plugins with
// {"package": "<file>", "upgrade": bool, "allow_downgrade": bool}.
func (d *Dashboard) handleAPIInstallAgentPlugin(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Package        string `json:"package"`
		Upgrade        bool   `json:"upgrade"`
		AllowDowngrade bool   `json:"allow_downgrade"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !validPackageFile(req.Package) {
		http.Error(w, "package (an uploaded .hpk file name) is required", http.StatusBadRequest)
		return
	}
	path := filepath.Join(d.pluginPackageDir(), req.Package)
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "package not found", http.StatusNotFound)
		return
	}
	c, token, code, err := d.managedAgentClient(name)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	resp, err := c.InstallPlugin(ctx, token, path, req.Upgrade, req.AllowDowngrade)
	if err != nil {
		http.Error(w, fmt.Sprintf("install on %s: %v", name, err), http.StatusBadGateway)
		return
	}
	// Show the plugin right away; the agent's next poll fills in its fields.
	d.upsertAgentPlugin(name, server.AgentPlugin{ID: resp.Id, Name: resp.Name, Version: resp.Version, Status: resp.Status, Signer: resp.Signer})
	wakeAgent(c, token)
	slog.Info("plugin installed on agent", "agent", name, "plugin", resp.Id, "version", resp.Version)
	writeJSON(w, map[string]string{"id": resp.Id, "name": resp.Name, "version": resp.Version, "status": resp.Status})
}

// handleAPIRemoveAgentPlugin handles DELETE /api/agents/{name}/plugins/{id}.
func (d *Dashboard) handleAPIRemoveAgentPlugin(w http.ResponseWriter, r *http.Request) {
	name, id := r.PathValue("name"), r.PathValue("id")
	for _, j := range d.registry.ListJobsByAgent(name) {
		if server.IsWorkloadJobType(j.JobType) && j.Plugin == id {
			http.Error(w, fmt.Sprintf("backup job %q uses this plugin; delete it first", j.Name), http.StatusConflict)
			return
		}
	}
	c, token, code, err := d.managedAgentClient(name)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	if err := c.RemovePlugin(ctx, token, id); err != nil {
		http.Error(w, fmt.Sprintf("remove from %s: %v", name, err), http.StatusBadGateway)
		return
	}
	d.removeAgentPlugin(name, id)
	wakeAgent(c, token)
	w.WriteHeader(http.StatusNoContent)
}

func (d *Dashboard) upsertAgentPlugin(agentName string, p server.AgentPlugin) {
	a := d.registry.GetAgent(agentName)
	if a == nil {
		return
	}
	list := slices.Clone(a.Plugins)
	replaced := false
	for i := range list {
		if list[i].ID == p.ID {
			p.Fields, p.Capabilities, p.ProtocolVersion = list[i].Fields, list[i].Capabilities, list[i].ProtocolVersion
			list[i], replaced = p, true
		}
	}
	if !replaced {
		list = append(list, p)
	}
	if err := d.registry.SetAgentPlugins(agentName, list); err != nil {
		slog.Warn("record agent plugin", "agent", agentName, "error", err)
	}
}

func (d *Dashboard) removeAgentPlugin(agentName, id string) {
	a := d.registry.GetAgent(agentName)
	if a == nil {
		return
	}
	list := slices.DeleteFunc(slices.Clone(a.Plugins), func(p server.AgentPlugin) bool { return p.ID == id })
	if err := d.registry.SetAgentPlugins(agentName, list); err != nil {
		slog.Warn("record agent plugin", "agent", agentName, "error", err)
	}
}
