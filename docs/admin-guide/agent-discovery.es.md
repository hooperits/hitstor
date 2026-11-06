# Descubrimiento automático de agentes

El dashboard puede descubrir instancias de `hitstor-agent` en su red local sin necesidad de ingresar manualmente cada nombre de host.

## Flujo de trabajo { #workflow }

1. Abra **Agentes** en el dashboard y haga clic en **+ Agregar agente** (administradores)
2. Permanezca en la pestaña **Escaneo de red**
3. Seleccione una de las subredes del servidor (de `/api/discover/subnets`)
4. Haga clic en **Escanear red**: el servidor sondea el puerto **8422** en cada dirección de la subred (hasta un `/16`; el escaneo se detiene a los 30 segundos) y muestra los agentes encontrados con su nombre de host, sistema operativo, versión y si ya están registrados
5. Haga clic en **Registrar** en un agente, ingrese su nombre y su token, y haga clic en **Registrar agente**

## Entrada manual { #manual-entry }

Use **Entrada manual** para un solo equipo: escriba la dirección IP del agente y haga clic en **Conectar**. El servidor sondea esa dirección en el puerto **8422** y, si un agente responde, ofrece registrarlo.

El descubrimiento y la entrada manual solo usan el puerto 8422. Para un agente en otro puerto, o detrás de NAT, regístrelo mediante la API con su dirección completa: `POST /api/agents` con `name`, `agent_addr` (`host:port`) y `token` (consulte [Dashboard → API JSON](dashboard.md#json-api)).

## API { #api }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/discover/subnets` | Listar las subredes IPv4 del servidor |
| `POST` | `/api/discover/scan` | Escanear una subred (`{"cidr": "..."}`, IPv4, `/16` o menor); devuelve un ID de operación cuyo progreso se transmite desde `/api/operations/{id}/events` |
| `POST` | `/api/discover/probe` | Sondear una dirección IP específica |

## Requisitos { #requirements }

- Los agentes deben ejecutarse en **modo daemon** con AgentService escuchando (puerto predeterminado 8422)
- El servidor debe poder alcanzar las direcciones de los agentes en la red (reglas de firewall)
- Tras el descubrimiento, complete el registro con el token del agente (`hitstor-agent show-id`); el servidor le envía entonces su clave de API. Lo que se respalda se elige después en una [tarea de respaldo](backup-jobs.md)

## Véase también { #see-also }

- [Administración de agentes](agent-management.md)
- [Despliegue de agentes](agent-deployment.md)
