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
| remove      | Elimina un agente de respaldo registrado |
| assign-repo | Asigna el repositorio de destino de un agente (para tareas futuras) |

## Descripción { #description }

Los agentes son los que realizan los respaldos. En configuraciones con varios repositorios, use `--repo-name` para operar sobre el registro de agentes de un repositorio específico.

Admite tanto agentes simples como agentes gestionados desde el dashboard.

La gestión del servicio está disponible en el binario del agente (`hitstor-agent service ...`, tanto en Linux como en Windows).

## Ejemplos { #examples }

```bash
hitstor --config /etc/hitstor/config.yaml --repo-name Production agents list
hitstor --config /etc/hitstor/config.yaml agents add --addr 192.168.1.10:8422 my-agent
hitstor --config /etc/hitstor/config.yaml agents assign-repo my-agent Production
hitstor --config /etc/hitstor/config.yaml agents list --all-repos
```

Véase también: `hitstor jobs` (las tareas hacen referencia a agentes), la gestión de agentes en el dashboard y `hitstor-agent service`.
