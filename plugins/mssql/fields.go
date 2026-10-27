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

// fields are the forms the dashboard renders for this plugin.
var fields = []hitstorplugin.FieldSpec{
	field("instance", hitstorplugin.FieldScopeSettings, "string", defaultInstance, "Instance", "Instancia",
		"MSSQLSERVER is the default instance.", "MSSQLSERVER es la instancia predeterminada."),
	field("auth", hitstorplugin.FieldScopeSettings, "choice", "integrated", "Authentication", "Autenticación",
		"integrated uses the agent service account; sql uses a SQL login.",
		"integrated usa la cuenta del servicio del agente; sql usa un inicio de sesión SQL.",
		"integrated", "sql"),
	field("username", hitstorplugin.FieldScopeSettings, "string", "", "SQL login", "Inicio de sesión SQL",
		"Only with authentication sql.", "Solo con autenticación sql."),
	field("password_env", hitstorplugin.FieldScopeSettings, "string", "HITSTOR_MSSQL_PASSWORD", "Password variable", "Variable de la contraseña",
		"Environment variable in the agent's agent.env that holds the password.",
		"Variable de entorno del agent.env del agente que contiene la contraseña."),

	field("copy_only", hitstorplugin.FieldScopeBackup, "bool", "false", "Copy-only backup", "Respaldo de solo copia",
		"Does not affect the database's backup chain (full and log only).",
		"No altera la cadena de respaldos de la base (solo completo y log)."),
	field("compression", hitstorplugin.FieldScopeBackup, "bool", "false", "SQL Server compression", "Compresión de SQL Server",
		"Smaller transfers, but compressed backups do not deduplicate.",
		"Transferencias más chicas, pero los respaldos comprimidos no se deduplican."),

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
