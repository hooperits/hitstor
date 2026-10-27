# Descubrimiento automático de agentes

El dashboard puede descubrir instancias de `hitstor-agent` en su red local sin necesidad de ingresar manualmente cada nombre de host.

## Flujo de trabajo { #workflow }

1. Abra **Agentes** en el dashboard
2. Haga clic en **Descubrimiento de red**
3. Seleccione una subred (de `/api/discover/subnets`) o ingrese un CIDR
4. Ejecute **Escanear**: el servidor sondea los puertos habituales del agente
5. Revise los hosts descubiertos y **agréguelos** con un clic

## Entrada manual { #manual-entry }

Use **Entrada manual** cuando el descubrimiento no sea posible (NAT, sitios remotos, puertos no estándar). Indique:

- **Nombre**: nombre visible del agente
- **Dirección del agente**: `host:port` (puerto de administración predeterminado del agente: **8422**)

## API { #api }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/discover/subnets` | Listar las subredes locales |
| `POST` | `/api/discover/scan` | Escanear una subred |
| `POST` | `/api/discover/probe` | Sondear una dirección específica |

## Requisitos { #requirements }

- Los agentes deben ejecutarse en **modo daemon** con AgentService escuchando (puerto predeterminado 8422)
- El servidor debe poder alcanzar las direcciones de los agentes en la red (reglas de firewall)
- Tras el descubrimiento, complete el registro asignando una clave de API y las rutas de respaldo

## Véase también { #see-also }

- [Administración de agentes](agent-management.md)
- [Despliegue de agentes](agent-deployment.md)
