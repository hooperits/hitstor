//go:build windows

package main

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const instanceNamesKey = `SOFTWARE\Microsoft\Microsoft SQL Server\Instance Names\SQL`

// localInstances lists the SQL Server instances installed on this host,
// from the registry, without connecting to them.
func localInstances() []instanceInfo {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, instanceNamesKey, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil
	}
	var out []instanceInfo
	for _, name := range names {
		id, _, err := k.GetStringValue(name)
		if err != nil {
			continue
		}
		info := instanceInfo{Name: name, Release: releaseName(id)}
		if s, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Microsoft SQL Server\`+id+`\Setup`, registry.QUERY_VALUE); err == nil {
			info.Edition, _, _ = s.GetStringValue("Edition")
			info.Version, _, _ = s.GetStringValue("PatchLevel")
			s.Close()
		}
		out = append(out, info)
	}
	return out
}

// releaseName maps an instance id such as "MSSQL16.MSSQLSERVER" to its
// product release.
func releaseName(id string) string {
	major, _, _ := strings.Cut(strings.TrimPrefix(id, "MSSQL"), ".")
	switch major {
	case "13":
		return "SQL Server 2016"
	case "14":
		return "SQL Server 2017"
	case "15":
		return "SQL Server 2019"
	case "16":
		return "SQL Server 2022"
	case "17":
		return "SQL Server 2025"
	}
	return "SQL Server"
}
