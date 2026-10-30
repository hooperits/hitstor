package dashboard

import "net/http"

type settingsPageData struct {
	ActiveTab string
	Accounts  bool // personal accounts replaced the shared dashboard password
}

func (d *Dashboard) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "general"
	}
	data := d.newPageData("Settings", "Server configuration and integrations", "settings", settingsPageData{
		ActiveTab: tab,
		Accounts:  d.authMode() == authUsers,
	})
	d.render(w, r, "settings", data)
}
