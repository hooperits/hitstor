// hitstor-plugin-mssql backs up and restores Microsoft SQL Server databases
// through the Virtual Device Interface (VDI): backup data streams straight
// from SQL Server into HitStor's pipeline, with no staging files on disk.
//
// Objects are "<instance>/<database>". Backup kinds: full, differential,
// log. Backup options: copy_only, compression. Restore chooses the full,
// differential, and log backups for a point in time from their LSNs.
//
// Settings (Configure):
//
//	instance                  instance name (default MSSQLSERVER)
//	server                    host to connect to (default localhost)
//	auth                      integrated (default: the agent's Windows account) or sql
//	username                  SQL login, with auth=sql
//	password_env              environment variable with the SQL login password
//	                          (default HITSTOR_MSSQL_PASSWORD; set it in agent.env)
//	encrypt                   connection encryption (default true)
//	trust_server_certificate  default true (the connection is local)
//
// The account must be able to back up and restore (sysadmin, or
// db_backupoperator plus dbcreator for restores) and VDI needs the agent to
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
