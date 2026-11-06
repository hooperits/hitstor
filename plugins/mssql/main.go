// hitstor-plugin-mssql backs up and restores Microsoft SQL Server databases
// through the Virtual Device Interface (VDI): backup data streams straight
// from SQL Server into HitStor's pipeline, with no staging files on disk.
//
// Objects are "<instance>" (every database that supports the backup kind,
// resolved at each run) and "<instance>/<database>". Backup kinds: full,
// differential, log. Backup options: copy_only, compression. Restore
// chooses the full, differential, and log backups for a point in time from
// their LSNs.
//
// Each instance's credentials are object settings saved on the agent
// (Configure on the instance in the dashboard, or hitstor-agent plugin
// configure), never in jobs:
//
//	auth      integrated (default: the agent's Windows account), windows, or sql
//	username  SQL login, or DOMAIN\user with auth=windows
//	password  stored encrypted on the agent
//
// Job settings (Configure), for the command line and older jobs:
//
//	instance                  instance listed when none is discovered (default MSSQLSERVER)
//	server                    host to connect to (default localhost)
//	auth, username            defaults for instances without saved settings
//	password_env              environment variable with the password for those
//	                          (default HITSTOR_MSSQL_PASSWORD)
//	encrypt                   connection encryption (default true)
//	trust_server_certificate  default true (the connection is local)
//
// The login must be in the sysadmin role: SQL Server requires it for BACKUP
// and RESTORE to a virtual device (error 3265). VDI also needs the agent to
// run on the SQL Server host.
package main

import (
	_ "embed"
	"fmt"
	"os"

	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/pluginpkg"
)

//go:embed plugin.yaml
var sourceManifest []byte

func main() {
	m, err := pluginpkg.ParseSource(sourceManifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg, _ := parseConfig(nil)
	hitstorplugin.ServeWorkload(&mssqlPlugin{manifest: m, cfg: cfg})
}
