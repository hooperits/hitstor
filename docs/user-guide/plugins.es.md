# Plugins

HitStor amplía su funcionalidad mediante binarios de plugins externos que usan el framework [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin). Los plugins se comunican con HitStor mediante **subprocesos gRPC**, no por stdin/stdout.

Para conocer los detalles del desarrollo de plugins, consulte [Desarrollo de plugins](../reference/plugin-development.md).

## Descubrimiento de plugins { #plugin-discovery }

HitStor busca los binarios de plugins en:

1. La ruta de configuración **`plugin_dir`** (se revisa primero)
2. **`$PATH`**

```yaml
plugin_dir: /usr/local/lib/hitstor/plugins
```

## Convención de nombres de los binarios { #binary-naming-convention }

| Tipo de plugin | Patrón | Ejemplo |
|----------------|--------|---------|
| Backend de almacenamiento | `hitstor-backend-NAME` | `hitstor-backend-memory` |
| Notificador | `hitstor-notifier-NAME` | `hitstor-notifier-slack` |
| Hook | `hitstor-hook-NAME` | `hitstor-hook-log` |

## Binarios de plugins integrados { #built-in-plugin-binaries }

El **escáner de sistema de archivos** (`hitstor-scanner-builtin`) es el plugin integrado principal. El paquete del agente lo instala en `/usr/lib/hitstor/plugins/`. Proporciona el respaldo de sistemas de archivos de Linux y Windows; si no se encuentra su binario, `hitstor` y `hitstor-agent` usan el mismo recorrido integrado en ellos.

Los demás plugins (backend de almacenamiento, notificador, hook) son opcionales y amplían la forma en que se almacenan los respaldos, cómo se generan alertas sobre ellos o qué se ejecuta antes y después de ellos.

Compilar todos los plugins:

```bash
make plugins
```

| Binario | Propósito | ¿Se instala de forma predeterminada? |
|---------|-----------|--------------------------------------|
| `hitstor-scanner-builtin` | Escáner principal de sistema de archivos (Linux/Windows) | Sí (paquete principal) |
| `hitstor-backend-memory` | Almacenamiento en memoria (pruebas) | No |
| `hitstor-notifier-log` | Ejemplo de notificador de registro | No |
| `hitstor-hook-log` | Ejemplo de hook | No |

## Plugins de backend de almacenamiento { #storage-backend-plugins }

```console
hitstor init --repo plugin:memory --password "secret"
hitstor backup --repo plugin:memory --password "secret" /data
```

## Plugins notificadores { #notifier-plugins }

```yaml
alert_notifier_plugins:
  - slack
plugin_dir: /usr/local/lib/hitstor/plugins
```

Configure también `alert_webhook_url` para el notificador de webhook integrado. Pueden ejecutarse varios notificadores en paralelo.

## Plugins hook { #hook-plugins }

```yaml
hook_plugins:
  - log
```

Los hooks se ejecutan en `PreBackup` y `PostBackup`. Los errores previos al respaldo cancelan el respaldo; los errores posteriores al respaldo solo se registran.

## Plugins escáneres de sistema de archivos { #filesystem-scanner-plugins }

Solo **un** plugin escáner puede estar activo a la vez:

```yaml
fs_scanner_plugin: builtin   # recorrido predeterminado
```

Use `builtin` para el recorrido predeterminado, o el nombre de un plugin escáner de sistema de archivos personalizado que haya instalado. Si `fs_scanner_plugin` no está definido, HitStor usa `hitstor-scanner-builtin` cuando lo encuentra (en `plugin_dir` o en `$PATH`) y, si no, su recorrido interno; un nombre definido explícitamente, incluido `builtin`, debe encontrarse o el respaldo falla.

## Instalación de plugins { #installing-plugins }

```console
cp bin/hitstor-scanner-myfs /usr/local/lib/hitstor/plugins/
chmod +x /usr/local/lib/hitstor/plugins/hitstor-scanner-myfs
```

Los plugins de backend de almacenamiento y los escáneres se inician cuando un comando los necesita. Los plugins notificadores y hook que usa el servidor se cargan cuando se inicia `hitstor serve`, así que reinicie el servidor después de cambiar `alert_notifier_plugins` o `hook_plugins`.

## Plugins de carga de trabajo { #workload-plugins }

Los plugins de carga de trabajo enseñan a un agente a respaldar una aplicación, como un motor de base de datos, en lugar de archivos. Son distintos de los plugins anteriores: cada uno es un paquete `.hpk` firmado que se instala por agente con [`hitstor-agent plugin`](../reference/agent-commands/plugin.md) o desde el dashboard (**Configuración → Plugins** y luego la página del agente), y el agente verifica la firma antes de activarlo. Los paquetes se publican como `hitstor-plugin-<id>-<versión>-<so>-<arquitectura>.hpk`.

| Plugin | Paquete | Carga de trabajo |
|--------|---------|------------------|
| `mssql` | `hitstor-plugin-mssql-<versión>-windows-amd64.hpk` | Microsoft SQL Server, mediante la Virtual Device Interface. Consulte [SQL Server](sql-server.md). |
| `example` | `hitstor-plugin-example-<versión>-<so>-amd64.hpk` (Linux y Windows) | Plugin de referencia para desarrolladores; no respalda nada útil. |

Las tareas que usan un plugin de carga de trabajo se crean en el dashboard con un **plan de respaldo**: una programación por cada tipo de respaldo que admite el plugin (para SQL Server: completo, diferencial y de log de transacciones). La página de la tarea muestra el resultado de cada base de datos (objeto) por separado.

## Ver también { #see-also }

- [Desarrollo de plugins](../reference/plugin-development.md)
