# Monitoreo y alertas

HitStor ofrece métricas de Prometheus y alertas por webhook para integrarse con la infraestructura de monitoreo existente.

## Métricas de Prometheus { #prometheus-metrics }

El servidor expone un endpoint de métricas compatible con Prometheus en:

```
http://SERVER_IP:8421/metrics
```

Este endpoint no requiere autenticación, lo que permite que Prometheus recopile métricas sin configurar credenciales. Restrinja el acceso a nivel de red si es necesario.

### Métricas disponibles { #available-metrics }

HitStor exporta 14 métricas: 9 gauges, 4 contadores y 1 métrica por agente.

#### Gauges { #gauges }

| Métrica | Tipo | Descripción |
|--------|------|-------------|
| `hitstor_agents_total` | gauge | Número total de agentes registrados |
| `hitstor_agents_active` | gauge | Número de agentes con respaldos recientes |
| `hitstor_agents_stale` | gauge | Número de agentes que superaron el umbral de inactividad |
| `hitstor_snapshots_total` | gauge | Número total de instantáneas (snapshots) en el repositorio |
| `hitstor_storage_bytes` | gauge | Tamaño total del repositorio en bytes |
| `hitstor_storage_raw_bytes` | gauge | Tamaño total de los datos originales (antes de la deduplicación) en bytes |
| `hitstor_dedup_ratio` | gauge | Tasa de deduplicación (original / almacenado) |
| `hitstor_packs_total` | gauge | Número de archivos de paquete en el repositorio |
| `hitstor_uptime_seconds` | gauge | Tiempo de actividad del servidor en segundos |

#### Contadores { #counters }

| Métrica | Tipo | Descripción |
|--------|------|-------------|
| `hitstor_backups_total` | counter | Número total de respaldos completados desde el inicio del servidor |
| `hitstor_backups_failed_total` | counter | Número total de respaldos fallidos desde el inicio del servidor |
| `hitstor_restores_total` | counter | Número total de restauraciones completadas desde el inicio del servidor |
| `hitstor_bytes_received_total` | counter | Total de bytes recibidos de los agentes desde el inicio del servidor |

#### Métrica por agente { #per-agent-metric }

| Métrica | Tipo | Etiquetas | Descripción |
|--------|------|--------|-------------|
| `hitstor_agent_last_backup_timestamp` | gauge | `agent` | Marca de tiempo Unix del último respaldo exitoso del agente |

### Configuración de recopilación de Prometheus { #prometheus-scrape-configuration }

Agregue lo siguiente a su `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: "hitstor"
    scrape_interval: 30s
    static_configs:
      - targets: ["backup.example.com:8421"]
```

Para varios servidores HitStor:

```yaml
scrape_configs:
  - job_name: "hitstor"
    scrape_interval: 30s
    static_configs:
      - targets:
          - "backup-01.example.com:8421"
          - "backup-02.example.com:8421"
```

### Dashboard de Grafana { #grafana-dashboard }

Para crear un dashboard de Grafana para HitStor:

1. Agregue su instancia de Prometheus como fuente de datos en Grafana.
2. Cree un nuevo dashboard con paneles para las métricas clave:

**Paneles sugeridos:**

| Panel | Consulta | Visualización |
|-------|-------|---------------|
| Agentes activos | `hitstor_agents_active` | Stat |
| Agentes inactivos | `hitstor_agents_stale` | Stat (umbral rojo > 0) |
| Total de instantáneas | `hitstor_snapshots_total` | Stat |
| Almacenamiento usado | `hitstor_storage_bytes` | Stat (unidad: bytes) |
| Tasa de deduplicación | `hitstor_dedup_ratio` | Stat |
| Ritmo de respaldos | `rate(hitstor_backups_total[1h])` | Time series |
| Respaldos fallidos | `rate(hitstor_backups_failed_total[1h])` | Time series (umbral de alerta) |
| Ingesta de datos | `rate(hitstor_bytes_received_total[5m])` | Time series (unidad: bytes/s) |
| Último respaldo por agente | `hitstor_agent_last_backup_timestamp` | Table |

3. Configure alertas de Grafana para `hitstor_agents_stale > 0` y `rate(hitstor_backups_failed_total[1h]) > 0`.

## Alertas por webhook { #webhook-alerting }

HitStor puede enviar notificaciones por webhook cuando detecta respaldos fallidos o agentes inactivos.

### Configuración { #configuration }

Defina la URL del webhook en el archivo de configuración:

```yaml
# /etc/hitstor/config.yaml
alert_webhook_url: "https://hooks.example.com/hitstor"
alert_stale_threshold: 25
```

| Clave | Tipo | Valor predeterminado | Descripción |
|-----|------|---------|-------------|
| `alert_webhook_url` | string | (ninguno) | URL a la que se envían (POST) las alertas |
| `alert_stale_threshold` | integer | 25 | Horas sin respaldo tras las cuales un agente se considera inactivo |

### Eventos de alerta { #alert-events }

HitStor envía alertas para dos tipos de eventos:

#### `backup_failed`

Se envía inmediatamente cuando un respaldo falla.

```json
{
  "event": "backup_failed",
  "agent": "web-server-01",
  "timestamp": "2026-02-21T03:05:00Z",
  "error": "connection reset by peer",
  "server": "backup.example.com"
}
```

#### `agent_stale`

Se envía cuando un agente no ha completado un respaldo dentro del umbral de inactividad.

```json
{
  "event": "agent_stale",
  "agent": "staging-app",
  "timestamp": "2026-02-21T14:00:00Z",
  "last_backup": "2026-02-19T12:30:00Z",
  "threshold_hours": 25,
  "server": "backup.example.com"
}
```

### Período de espera y recuperación { #cooldown-and-recovery }

Para evitar la fatiga de alertas, HitStor aplica un período de espera (cooldown):

- Después de enviar una alerta de inactividad para un agente, no vuelve a alertar hasta que haya transcurrido nuevamente el tiempo del umbral.
- Cuando un agente inactivo completa un respaldo exitoso, se borra el estado de alerta. Si el agente vuelve a quedar inactivo, se envía una nueva alerta.
- Las alertas de respaldos fallidos no tienen período de espera: cada fallo genera una notificación.

## Plugins notificadores { #notifier-plugins }

Para enviar notificaciones personalizadas (Slack, PagerDuty, correo electrónico, etc.), use plugins notificadores en lugar del webhook o además de él.

Configure los plugins notificadores en el archivo de configuración:

```yaml
# /etc/hitstor/config.yaml
alert_notifier_plugins:
  - /usr/lib/hitstor/plugins/slack-notifier
  - /usr/lib/hitstor/plugins/pagerduty-notifier
```

Los plugins notificadores reciben el mismo payload JSON que el webhook y pueden transformarlo para su sistema de destino. Consulte la documentación de [Plugins](../user-guide/plugins.md) para obtener detalles sobre cómo escribir e instalar plugins.

## Próximos pasos { #next-steps }

- [Dashboard](dashboard.md) -- vea las métricas y el estado de los agentes en la interfaz web
- [Programación](scheduling.md) -- configure las programaciones de respaldo para mantener activos los agentes
- [Solución de problemas](troubleshooting.md) -- diagnostique agentes inactivos y respaldos fallidos
