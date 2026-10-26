//go:build !linux

package dashboard

import (
	"net/http"

	"github.com/hooperits/hitstor/internal/logstore"
)

func (d *Dashboard) fetchOSLogs(r *http.Request, f logstore.Filter) ([]logstore.Entry, string) {
	return d.fetchOSLogsPlatform(r, f)
}
