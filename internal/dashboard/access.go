package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/server"
	"github.com/hooperits/hitstor/internal/users"
)

// routePolicy is what a route needs from the signed-in user: a permission,
// on the agent the request is about (agent), on any agent (anyAgent: the
// handler narrows it), or on every agent (neither: resources that span
// agents). stepUp asks for a recent sign-in or re-authentication.
type routePolicy struct {
	perms    []users.Permission // one of them
	agent    func(d *Dashboard, r *http.Request) []string
	anyAgent bool
	stepUp   bool
}

func perms(p ...users.Permission) []users.Permission { return p }

var (
	view    = perms(users.PermView)
	backup  = perms(users.PermBackup)
	restore = perms(users.PermRestore)
	admin   = perms(users.PermAdmin)
	browse  = perms(users.PermBackup, users.PermRestore)
)

func pathAgent(r *http.Request) []string { return []string{r.PathValue("name")} }

func agentFromPath(d *Dashboard, r *http.Request) []string { return pathAgent(r) }

func agentFromJob(d *Dashboard, r *http.Request) []string {
	if j := d.registry.GetJob(r.PathValue("id")); j != nil {
		return []string{j.AgentName}
	}
	return []string{""} // unknown job: only users on every agent get the 404
}

func agentFromSnapshot(d *Dashboard, r *http.Request) []string {
	return d.snapshotAgents(r.PathValue("id"))
}

func agentFromQuery(key string) func(d *Dashboard, r *http.Request) []string {
	return func(d *Dashboard, r *http.Request) []string { return []string{r.URL.Query().Get(key)} }
}

// routePolicies lists the routes that do not follow the defaults: reads
// need view on every agent, and changes need the administrator.
var routePolicies = map[string]routePolicy{
	// Pages and lists filtered to the user's agents.
	"GET /{$}":                         {perms: view, anyAgent: true}, // others go to their jobs
	"GET /backups":                     {perms: view, anyAgent: true},
	"GET /backups/{id}":                {perms: view, agent: agentFromJob},
	"GET /restores":                    {perms: restore, anyAgent: true},
	"GET /agents":                      {perms: view, anyAgent: true},
	"GET /agents/{name}":               {perms: view, agent: agentFromPath},
	"GET /snapshots/{id}":              {perms: view, agent: agentFromSnapshot},
	"GET /account":                     {perms: view, anyAgent: true},
	"GET /account/password":            {perms: view, anyAgent: true},
	"GET /api/agents":                  {perms: view, anyAgent: true},
	"GET /api/agents/{name}/snapshots": {perms: view, agent: agentFromPath},
	"GET /api/agents/{name}/info":      {perms: view, agent: agentFromPath},
	"GET /api/active-backups":          {perms: view, anyAgent: true},
	"GET /api/backup-progress/events":  {perms: view, anyAgent: true},
	"GET /api/backups":                 {perms: view, anyAgent: true},
	"GET /api/backups/{id}":            {perms: view, agent: agentFromJob},
	"GET /api/backups/{id}/chains":     {perms: view, agent: agentFromJob},
	"GET /api/backups/validate-cron":   {perms: view, anyAgent: true},
	"GET /api/timezones":               {perms: view, anyAgent: true},
	"GET /api/repos":                   {perms: browse, anyAgent: true}, // names, to choose a job's repository
	"GET /api/restores/sources":        {perms: restore, anyAgent: true},
	"GET /api/restores":                {perms: restore, anyAgent: true},
	"GET /api/restore/{id}/events":     {perms: restore, anyAgent: true},
	"GET /api/workload/restores/{id}":  {perms: restore, anyAgent: true},
	"GET /api/workload/points":         {perms: restore, agent: agentFromQuery("agent")},
	"GET /api/workload/catalog":        {perms: restore, agent: agentFromQuery("agent")},

	// Backup operators: jobs of their agents.
	"POST /api/backups":                                     {perms: backup, anyAgent: true}, // the handler checks agent_name
	"PUT /api/backups/{id}":                                 {perms: backup, agent: agentFromJob},
	"DELETE /api/backups/{id}":                              {perms: backup, agent: agentFromJob, stepUp: true},
	"POST /api/backups/{id}/run":                            {perms: backup, agent: agentFromJob},
	"POST /api/backups/{id}/cancel":                         {perms: backup, agent: agentFromJob},
	"POST /api/agents/{name}/suspend":                       {perms: backup, agent: agentFromPath},
	"POST /api/agents/{name}/unsuspend":                     {perms: backup, agent: agentFromPath},
	"POST /api/agents/{name}/schedule":                      {perms: backup, agent: agentFromPath},
	"POST /api/agents/{name}/browse":                        {perms: browse, agent: agentFromPath},
	"GET /api/agents/{name}/plugins/{plugin}/browse":        {perms: browse, agent: agentFromPath},
	"GET /api/agents/{name}/plugins/{plugin}/object-config": {perms: backup, agent: agentFromPath},
	"PUT /api/agents/{name}/plugins/{plugin}/object-config": {perms: backup, agent: agentFromPath},

	// Restore operators: restores from and to their agents.
	"GET /api/snapshots/{id}/tree":     {perms: restore, agent: agentFromSnapshot},
	"POST /api/snapshots/{id}/restore": {perms: restore, agent: agentFromSnapshot}, // and the target
	"POST /api/workload/restore-plan":  {perms: restore, anyAgent: true},           // the handler checks both agents
	"POST /api/workload/restores":      {perms: restore, anyAgent: true},

	// Every user: their own account.
	"POST /api/account/password":        {perms: view, anyAgent: true},
	"POST /api/account/recovery-codes":  {perms: view, anyAgent: true, stepUp: true},
	"GET /api/account/sessions":         {perms: view, anyAgent: true},
	"DELETE /api/account/sessions/{id}": {perms: view, anyAgent: true},
	"POST /api/reauth":                  {perms: view, anyAgent: true},

	// Administrators: sensitive changes ask for a recent sign-in.
	"GET /users":                             {perms: admin},
	"GET /api/users":                         {perms: admin},
	"GET /api/users/{name}/sessions":         {perms: admin},
	"GET /audit":                             {perms: admin},
	"GET /settings":                          {perms: admin},
	"GET /api/settings":                      {perms: admin},
	"POST /api/users":                        {perms: admin, stepUp: true},
	"PUT /api/users/{name}":                  {perms: admin, stepUp: true},
	"DELETE /api/users/{name}":               {perms: admin, stepUp: true},
	"POST /api/users/{name}/password":        {perms: admin, stepUp: true},
	"DELETE /api/users/{name}/second-factor": {perms: admin, stepUp: true},
	"DELETE /api/users/{name}/sessions/{id}": {perms: admin, stepUp: true},
	"DELETE /api/repos/{name}":               {perms: admin, stepUp: true},
	"POST /api/repos/{name}/prune":           {perms: admin, stepUp: true},
	"POST /api/repos/{name}/forget":          {perms: admin, stepUp: true},
	"POST /api/disaster/recover":             {perms: admin, stepUp: true},
	"POST /api/bmr/prepare/{agent}":          {perms: admin, stepUp: true},
	"PUT /api/settings/security":             {perms: admin, stepUp: true},
	"POST /api/settings/tls/upload":          {perms: admin, stepUp: true},
	"POST /api/plugin-packages":              {perms: admin, stepUp: true},
	"POST /api/agents/{name}/plugins":        {perms: admin, stepUp: true},
	"DELETE /api/agents/{name}":              {perms: admin, stepUp: true},
}

// policyFor returns the policy of a route pattern ("METHOD /path").
func policyFor(pattern string) routePolicy {
	if p, ok := routePolicies[pattern]; ok {
		return p
	}
	if strings.HasPrefix(pattern, "GET ") {
		return routePolicy{perms: view} // reads that span agents
	}
	return routePolicy{perms: admin}
}

// guard is protect plus the route's policy.
func (d *Dashboard) guard(pattern string, h http.Handler) http.Handler {
	p := policyFor(pattern)
	return d.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := authOf(r)
		if a == nil || a.user == nil {
			h.ServeHTTP(w, r) // no accounts: everyone may do everything
			return
		}
		if !allowed(d, r, a.user, p) {
			d.auditEvent(r, "", "denied", r.Method+" "+r.URL.Path, false, "")
			forbid(w, r)
			return
		}
		if p.stepUp && time.Since(a.session.AuthAt) > users.StepUpWindow {
			w.Header().Set("X-HitStor-Reauth", "1")
			writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "reauth_required"})
			return
		}
		h.ServeHTTP(w, r)
	}))
}

func allowed(d *Dashboard, r *http.Request, u *users.User, p routePolicy) bool {
	for _, perm := range p.perms {
		switch {
		case p.agent != nil:
			agents := p.agent(d, r)
			for _, a := range agents {
				if a != "" && u.Can(perm, a) || a == "" && u.CanAll(perm) {
					return true
				}
			}
		case p.anyAgent:
			if u.Can(perm, "") {
				return true
			}
		default:
			if u.CanAll(perm) {
				return true
			}
		}
	}
	return false
}

func forbid(w http.ResponseWriter, r *http.Request) {
	if isAPI(r) {
		http.Error(w, "forbidden: your role does not allow this", http.StatusForbidden)
		return
	}
	http.Error(w, "Forbidden: your role does not allow this page", http.StatusForbidden)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// can reports whether the request's user has perm on agent ("" for every
// agent). Without accounts, everyone can.
func can(r *http.Request, perm users.Permission, agent string) bool {
	u := currentUser(r)
	if u == nil {
		return true
	}
	if agent == "" {
		return u.CanAll(perm)
	}
	return u.Can(perm, agent)
}

// requireAgent answers 403 and returns false unless the request's user has
// perm on agent; handlers use it for agents named in request bodies.
func (d *Dashboard) requireAgent(w http.ResponseWriter, r *http.Request, perm users.Permission, agent string) bool {
	if can(r, perm, agent) {
		return true
	}
	d.auditEvent(r, "", "denied", r.Method+" "+r.URL.Path, false, "agent "+agent)
	forbid(w, r)
	return false
}

// agentFilter returns whether the request's user may see an agent with
// perm; nil means every agent.
func agentFilter(r *http.Request, perm users.Permission) func(agent string) bool {
	u := currentUser(r)
	if u == nil {
		return nil
	}
	all, agents := u.Agents(perm)
	if all {
		return nil
	}
	return func(agent string) bool { return agents[agent] }
}

// snapshotAgents returns the agents a snapshot (by ID or prefix) belongs to:
// through its job, or by host name.
func (d *Dashboard) snapshotAgents(idPrefix string) []string {
	if d.repos == nil || idPrefix == "" {
		return []string{""}
	}
	for _, mr := range d.repos.List() {
		ids, err := mr.Repo.ListSnapshots()
		if err != nil {
			continue
		}
		for _, id := range ids {
			if !strings.HasPrefix(id.String(), idPrefix) {
				continue
			}
			snap, err := mr.Repo.LoadSnapshot(id)
			if err != nil {
				return []string{""}
			}
			var out []string
			cs := cachedSnapshotSummary{Hostname: snap.Hostname, Tags: snap.Tags}
			for _, a := range d.registry.ListAgents() {
				if d.snapshotMatchesAgent(cs, a.Name) {
					out = append(out, a.Name)
				}
			}
			for _, t := range snap.Tags {
				if id, ok := strings.CutPrefix(t, "job:"); ok {
					if j := d.registry.GetJob(id); j != nil {
						out = append(out, j.AgentName)
					}
				}
			}
			if len(out) == 0 {
				return []string{""}
			}
			return out
		}
	}
	return []string{""}
}

// visibleAgents returns the agents the request's user may see.
func (d *Dashboard) visibleAgents(r *http.Request) []server.AgentRecord {
	all := d.registry.ListAgents()
	visible := agentFilter(r, users.PermView)
	if visible == nil {
		return all
	}
	out := all[:0:0]
	for _, a := range all {
		if visible(a.Name) {
			out = append(out, a)
		}
	}
	return out
}

// visibleJobs returns the jobs the request's user may see.
func (d *Dashboard) visibleJobs(r *http.Request) []server.BackupJob {
	all := d.registry.ListJobs()
	visible := agentFilter(r, users.PermView)
	if visible == nil {
		return all
	}
	out := all[:0:0]
	for _, j := range all {
		if visible(j.AgentName) {
			out = append(out, j)
		}
	}
	return out
}

// snapshotVisible reports whether the request's user has perm on an agent
// the snapshot belongs to.
func (d *Dashboard) snapshotVisible(r *http.Request, perm users.Permission, cs cachedSnapshotSummary) bool {
	visible := agentFilter(r, perm)
	if visible == nil {
		return true
	}
	for _, a := range d.registry.ListAgents() {
		if visible(a.Name) && d.snapshotMatchesAgent(cs, a.Name) {
			return true
		}
	}
	for _, t := range cs.Tags {
		if id, ok := strings.CutPrefix(t, "job:"); ok {
			if j := d.registry.GetJob(id); j != nil && visible(j.AgentName) {
				return true
			}
		}
	}
	return false
}
