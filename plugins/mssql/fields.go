package main

import hitstorplugin "github.com/hooperits/hitstor/internal/plugin"

func field(key, scope, typ, def, en, es, helpEN, helpES string, choices ...string) hitstorplugin.FieldSpec {
	f := hitstorplugin.FieldSpec{
		Key: key, Scope: scope, Type: typ, Default: def, Choices: choices,
		Label: map[string]string{"en": en, "es": es},
	}
	if helpEN != "" {
		f.Help = map[string]string{"en": helpEN, "es": helpES}
	}
	return f
}

// browseChoices offers the agent's instances as the field's choices.
func browseChoices(f hitstorplugin.FieldSpec) hitstorplugin.FieldSpec {
	f.ChoicesFrom = "browse"
	return f
}

// instanceField makes f an instance setting, shown when showIf holds.
func instanceField(f hitstorplugin.FieldSpec, showIf string) hitstorplugin.FieldSpec {
	f.ObjectKinds = []string{"instance"}
	f.ShowIf = showIf
	return f
}

// fields are the forms the dashboard renders for this plugin.
var fields = []hitstorplugin.FieldSpec{
	// Each instance's credentials are set from the selection tree and stay
	// on the agent; jobs only name what to back up.
	instanceField(field("auth", hitstorplugin.FieldScopeObject, "choice", "integrated", "Authentication", "Autenticación",
		"integrated: the agent service account (LocalSystem). windows: a Windows account. sql: a SQL Server login. The account must be in the sysadmin role.",
		"integrated: la cuenta del servicio del agente (LocalSystem). windows: una cuenta de Windows. sql: un inicio de sesión de SQL Server. La cuenta debe tener el rol sysadmin.",
		"integrated", "windows", "sql"), ""),
	instanceField(field("username", hitstorplugin.FieldScopeObject, "string", "", "User name", "Usuario",
		`SQL login, or DOMAIN\user for a Windows account (.\user if local).`,
		`Inicio de sesión SQL, o DOMINIO\usuario para una cuenta de Windows (.\usuario si es local).`), "auth=windows|sql"),
	instanceField(field("password", hitstorplugin.FieldScopeObject, hitstorplugin.FieldTypePassword, "", "Password", "Contraseña",
		"Stored encrypted on the agent; never sent back.",
		"Se guarda cifrada en el agente; nunca se vuelve a mostrar."), "auth=windows|sql"),

	field("copy_only", hitstorplugin.FieldScopeBackup, "bool", "false", "Copy-only backup", "Respaldo de solo copia",
		"Does not affect the database's backup chain (full and log only).",
		"No altera la cadena de respaldos de la base (solo completo y log)."),
	field("compression", hitstorplugin.FieldScopeBackup, "bool", "false", "SQL Server compression", "Compresión de SQL Server",
		"Smaller transfers, but compressed backups do not deduplicate.",
		"Transferencias más chicas, pero los respaldos comprimidos no se deduplican."),

	browseChoices(field("instance", hitstorplugin.FieldScopeRestore, "choice", "", "Target instance", "Instancia de destino",
		"Empty restores on the instance the backup came from.", "Vacío restaura en la instancia de la que vino el respaldo.")),
	field("database", hitstorplugin.FieldScopeRestore, "string", "", "Restore as", "Restaurar como",
		"Database name; empty keeps the original name.", "Nombre de la base; vacío conserva el nombre original."),
	field("replace", hitstorplugin.FieldScopeRestore, "bool", "false", "Overwrite existing database", "Sobrescribir la base existente", "", ""),
	field("recovery", hitstorplugin.FieldScopeRestore, "bool", "true", "Bring online", "Dejar en línea",
		"Off leaves the database RESTORING for further manual restores.",
		"Desactivado deja la base en RESTORING para restauraciones manuales posteriores."),
	field("data_path", hitstorplugin.FieldScopeRestore, "string", "", "Data folder", "Carpeta de datos",
		"Where relocated data files go; empty uses the instance default.",
		"Dónde van los archivos de datos reubicados; vacío usa el valor de la instancia."),
	field("log_path", hitstorplugin.FieldScopeRestore, "string", "", "Log folder", "Carpeta de log",
		"Where relocated log files go; empty uses the instance default.",
		"Dónde van los archivos de log reubicados; vacío usa el valor de la instancia."),
}
