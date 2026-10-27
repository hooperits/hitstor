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

El **escáner de sistema de archivos** (`hitstor-scanner-builtin`) es el plugin integrado principal y se instala con los paquetes del agente y del servidor en el directorio estándar de plugins. Proporciona el respaldo de sistemas de archivos de Linux y Windows.

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

Use `builtin` para el recorrido predeterminado, o el nombre de un plugin escáner de sistema de archivos personalizado que haya instalado.

## Instalación de plugins { #installing-plugins }

```console
cp bin/hitstor-scanner-myfs /usr/local/lib/hitstor/plugins/
chmod +x /usr/local/lib/hitstor/plugins/hitstor-scanner-myfs
```

No es necesario reiniciar: los plugins se inician cuando se necesitan.

## Ver también { #see-also }

- [Desarrollo de plugins](../reference/plugin-development.md)
