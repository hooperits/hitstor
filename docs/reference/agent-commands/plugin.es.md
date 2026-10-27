# hitstor-agent plugin

Instala y administra plugins de carga de trabajo en este agente.

## Sinopsis { #synopsis }

```
hitstor-agent plugin install <paquete.hpk>
hitstor-agent plugin upgrade <paquete.hpk> [--allow-downgrade]
hitstor-agent plugin list
hitstor-agent plugin info <id>
hitstor-agent plugin remove <id>
```

## Descripción { #description }

Los plugins de carga de trabajo son paquetes `.hpk` firmados que le enseñan a
un agente a respaldar una aplicación, como un motor de base de datos. Un plugin
se instala solo en los agentes que lo necesitan y se puede actualizar o
desinstalar sin reinstalar el agente. Consulte
[Arquitectura de plugins de carga de trabajo](../../dev-guide/plugin-architecture.md)
para conocer el diseño.

Antes de activar un paquete, el agente:

1. Verifica su firma contra las claves de confianza: la clave oficial de
   HitStor más las que figuren en `plugin_trusted_keys`.
2. Comprueba que sea compatible con el sistema operativo y la arquitectura del
   agente, con el protocolo de carga de trabajo y con su versión
   (`min_agent_version`).
3. Lo descomprime en un directorio de staging y lanza el plugin una vez para
   confirmar que reporta el id y la versión que declara su manifiesto.

Solo entonces la nueva versión pasa a estar activa. Si algún paso falla, la
versión instalada anteriormente sigue activa y sin cambios.

El agente informa al servidor sus plugins instalados cada vez que consulta su
programación, así el servidor sabe qué cargas de trabajo puede atender cada
agente.

## Subcomandos { #subcommands }

| Subcomando | Descripción |
|------------|-------------|
| `install <paquete.hpk>` | Instala un plugin que todavía no está instalado |
| `upgrade <paquete.hpk>` | Reemplaza un plugin instalado por otra versión. Las versiones anteriores se rechazan salvo que se indique `--allow-downgrade` |
| `list` | Lista los plugins instalados con su versión, estado y firma |
| `info <id>` | Muestra el detalle de un plugin, incluidas sus capacidades y la ruta de su binario |
| `remove <id>` | Desinstala un plugin. Las instantáneas (snapshots) que creó permanecen en el repositorio |

## Flags { #flags }

| Flag | Predeterminado | Descripción |
|------|----------------|-------------|
| `--store` | `plugin_store_dir`, o el valor por defecto de la plataforma | Directorio del almacén de plugins |
| `--allow-downgrade` | `false` | Solo `upgrade`: permite instalar una versión anterior |
| `--json` | `false` | Imprime los resultados en JSON |

Sin `--config`, el comando lee la configuración del agente instalado
(`/etc/hitstor/agent-config.yaml` o `C:\ProgramData\HitStor\agent-config.yaml`),
de modo que aplica las mismas reglas de confianza que el servicio del agente.

## Almacén de plugins { #plugin-store }

| Plataforma | Directorio predeterminado |
|------------|---------------------------|
| Linux | `/var/lib/hitstor/plugins` |
| Windows | `C:\ProgramData\HitStor\plugins` |

El almacén queda restringido a su propietario en Linux (modo `0700`) y a
SYSTEM y Administradores en Windows, porque los plugins se ejecutan con los
privilegios del agente. Por eso instalar y desinstalar plugins requiere root o
una consola con privilegios elevados.

## Ejemplos { #examples }

```bash
sudo hitstor-agent plugin install hitstor-plugin-example-0.1.0-linux-amd64.hpk
sudo hitstor-agent plugin list
```

Salida:

```
ID       VERSION  STATUS  SIGNED    NAME
example  0.1.0    active  official  Example workload plugin
```

La columna `SIGNED` muestra `official` para los paquetes firmados con la clave
de publicación de HitStor, `third-party` para los firmados con una clave de
`plugin_trusted_keys` y `unsigned` cuando `plugin_allow_unsigned` está
activado.

Para confiar en su propia clave de firma, por ejemplo para plugins internos:

```yaml
# agent-config.yaml
plugin_trusted_keys:
  - "<clave pública ed25519 en base64>"
```

## Consulte también { #see-also }

- [Guía de desarrollo de plugins](../plugin-development.md#workload-plugins)
- [Referencia de configuración](../config-reference.md#plugin-settings)
