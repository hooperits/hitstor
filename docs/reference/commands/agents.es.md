# hitstor agents

Gestiona los agentes de respaldo registrados (compatible con varios repositorios).

## Sinopsis { #synopsis }

```
hitstor agents [command]
```

## Comandos disponibles { #available-commands }

| Comando     | Descripción |
|-------------|-------------|
| add         | Registra un nuevo agente de respaldo (admite --addr para agentes gestionados) |
| list        | Lista los agentes de respaldo registrados (admite --repo-name, --all-repos, --json) |
| remove      | Elimina un agente de respaldo registrado (`--keep-jobs` conserva sus tareas) |
| re-register | Vuelve a registrar un agente existente, conservando sus tareas, con `--token` de `hitstor-agent show-id` (`--addr`, `--skip-rpc`) |
| assign-repo | Asigna el repositorio de destino de un agente (para tareas futuras) |

## Descripción { #description }

Los agentes son los que realizan los respaldos. `hitstor agents` es el mismo grupo de comandos que [`hitstor serve agent`](serve.md#subcommands). El servidor guarda su registro de agentes en el primer repositorio configurado (por nombre), y estos comandos usan ese repositorio de forma predeterminada; `--repo-name` con otro repositorio muestra una advertencia, porque el servidor en ejecución no lo lee. `list --all-repos` lista los agentes registrados en todos los repositorios configurados.

Admite tanto agentes simples como agentes gestionados desde el dashboard.

La gestión del servicio está disponible en el binario del agente (`hitstor-agent service ...`, tanto en Linux como en Windows).

## Ejemplos { #examples }

```bash
hitstor --config /etc/hitstor/config.yaml agents list
hitstor --config /etc/hitstor/config.yaml agents add --addr 192.0.2.10:8422 my-agent
hitstor --config /etc/hitstor/config.yaml agents assign-repo my-agent Production
hitstor --config /etc/hitstor/config.yaml agents list --all-repos
```

Véase también: `hitstor jobs` (las tareas hacen referencia a agentes), la gestión de agentes en el dashboard y `hitstor-agent service`.
