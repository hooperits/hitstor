# hitstor-agent service

Gestiona el servicio de Windows (solo Windows).

## Sinopsis { #synopsis }

```
hitstor-agent service install
hitstor-agent service start
hitstor-agent service stop
hitstor-agent service uninstall
```

## Descripción { #description }

Gestiona el agente de HitStor como servicio de Windows. El servicio se ejecuta en modo daemon y consulta periódicamente al servidor para obtener los respaldos programados.

## Subcomandos { #subcommands }

### `service install` { #service-install }

Registra el agente como servicio de Windows. Requiere privilegios de administrador.

### `service start` { #service-start }

Inicia el servicio de Windows.

### `service stop` { #service-stop }

Detiene el servicio de Windows.

### `service uninstall` { #service-uninstall }

Elimina el registro del servicio de Windows.

## Ejemplos { #examples }

Instalar e iniciar el servicio:

```powershell
hitstor-agent service install
hitstor-agent service start
```

Verificar el estado del servicio:

```powershell
Get-Service HitStorAgent
```

Detener y desinstalar:

```powershell
hitstor-agent service stop
hitstor-agent service uninstall
```

## Notas { #notes }

- El servicio se ejecuta como `LocalSystem` de forma predeterminada.
- La configuración se lee de `%AppData%\hitstor\config.yaml` o de variables de entorno.
- Los registros del servicio se escriben en el Registro de eventos de Windows.
- Este comando no hace nada en Linux (Linux usa systemd en su lugar).
