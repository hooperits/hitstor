//go:build !linux && !windows

package dashboard

import (
	"net/http"

	"github.com/hooperits/hitstor/internal/logstore"
)

func (d *Dashboard) fetchOSLogsPlatform(r *http.Request, f logstore.Filter) ([]logstore.Entry, string) {
	return nil, "OS log provider not available on this platform"
}
