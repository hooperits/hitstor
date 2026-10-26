//go:build windows

package agent

// defaultExcludes contains patterns that are always excluded on Windows
// to prevent backing up system files, recycling bins, and other OS artifacts
// that cause excessive I/O and are not useful for backup purposes.
var defaultExcludes = []string{
	// System files.
	"$Recycle.Bin/",
	"System Volume Information/",
	"pagefile.sys",
	"hiberfil.sys",
	"swapfile.sys",
	"DumpStack.log.tmp",

	// Registry hives (locked by OS).
	"NTUSER.DAT*",
	"ntuser.dat*",
	"UsrClass.dat*",

	// Windows metadata.
	"Thumbs.db",
	"desktop.ini",
	"IconCache.db",

	// Temp and cache directories.
	"Temp/",
	"INetCache/",
	"Cache/",
	"CacheStorage/",
	"Code Cache/",
	"GPUCache/",
	"ShaderCache/",
	"DawnCache/",
	"GrShaderCache/",
}
