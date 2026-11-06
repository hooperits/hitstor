# Monitoreo y alertas

HitStor ofrece métricas de Prometheus y alertas por webhook para integrarse con la infraestructura de monitoreo existente.

## Métricas de Prometheus { #prometheus-metrics }

El servidor expone un endpoint de métricas compatible con Prometheus en:

```
https://SERVER_IP:8421/metrics
```

Este endpoint no requiere autenticación, lo que permite que Prometheus recopile métricas sin configurar credenciales. Restrinja el acceso a nivel de red si es necesario. Existe cuando el servidor ya tiene un repositorio; las métricas de repositorio describen el primer repositorio del servidor.

### Métricas disponibles { #available-metrics }

#### Gauges de repositorio y agentes { #repository-and-agent-gauges }

| Métrica | Tipo | Descripción |
|--------|------|-------------|
| `hitstor_snapshots_total` | gauge | Total de instantáneas (snapshots) en el repositorio |
| `hitstor_packs_total` | gauge | Total de archivos de paquete en el repositorio |
| `hitstor_indexes_total` | gauge | Total de archivos de índice en el repositorio |
| `hitstor_repository_size_bytes` | gauge | Tamaño total del repositorio en bytes |
| `hitstor_active_sessions` | gauge | Sesiones de respaldo activas en este momento |
| `hitstor_registered_agents` | gauge | Total de agentes registrados |
| `hitstor_agents_ok` | gauge | Agentes con un respaldo dentro de `alert_stale_threshold` (25 horas de forma predeterminada) |
| `hitstor_agents_stale` | gauge | Agentes cuyo último respaldo es anterior a `alert_stale_threshold` |
| `hitstor_agents_never` | gauge | Agentes que nunca han realizado un respaldo |

#### Contadores { #counters }

Los contadores se reinician cuando se reinicia el servidor.

| Métrica | Tipo | Descripción |
|--------|------|-------------|
| `hitstor_backups_completed_total` | counter | Respaldos exitosos |
| `hitstor_backups_failed_total` | counter | Sesiones de respaldo fallidas o abortadas |
| `hitstor_bytes_received_total` | counter | Bytes recibidos de los agentes |
| `hitstor_chunks_received_total` | counter | Fragmentos (chunks) recibidos de los agentes |
| `hitstor_checks_total` | counter | Verificaciones de integridad ejecutadas desde el dashboard |
| `hitstor_check_errors_total` | counter | Errores encontrados por esas verificaciones |
| `hitstor_prunes_total` | counter | Operaciones de depuración ejecutadas desde el dashboard |
| `hitstor_prune_freed_bytes_total` | counter | Bytes liberados por esas depuraciones |

#### Métricas por agente { #per-agent-metrics }

| Métrica | Tipo | Etiquetas | Descripción |
|--------|------|--------|-------------|
| `hitstor_agent_last_backup_timestamp_seconds` | gauge | `agent` | Marca de tiempo Unix del último respaldo exitoso del agente (0 si nunca) |
| `hitstor_agent_schedule_enabled` | gauge | `agent` | Indicador heredado de programación por agente (1/0); las tareas de respaldo tienen su propia programación |
| `hitstor_agent_schedule_interval_hours` | gauge | `agent` | Intervalo heredado por agente, en horas |

### Configuración de recopilación de Prometheus { #prometheus-scrape-configuration }

Agregue lo siguiente a su `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: "hitstor"
    scrape_interval: 30s
    scheme: https
    tls_config:
      ca_file: /etc/prometheus/hitstor-server.crt  # el certificado del servidor o su CA
    static_configs:
      - targets: ["backup.example.com:8421"]
```

Para varios servidores HitStor:

```yaml
scrape_configs:
  - job_name: "hitstor"
    scrape_interval: 30s
    scheme: https
    tls_config:
      ca_file: /etc/prometheus/hitstor-ca.crt
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
| Agentes en buen estado | `hitstor_agents_ok` | Stat |
| Agentes inactivos | `hitstor_agents_stale` | Stat (umbral rojo > 0) |
| Total de instantáneas | `hitstor_snapshots_total` | Stat |
| Almacenamiento usado | `hitstor_repository_size_bytes` | Stat (unidad: bytes) |
| Respaldos en ejecución | `hitstor_active_sessions` | Stat |
| Ritmo de respaldos | `rate(hitstor_backups_completed_total[1h])` | Time series |
| Respaldos fallidos | `rate(hitstor_backups_failed_total[1h])` | Time series (umbral de alerta) |
| Ingesta de datos | `rate(hitstor_bytes_received_total[5m])` | Time series (unidad: bytes/s) |
| Último respaldo por agente | `hitstor_agent_last_backup_timestamp_seconds` | Table |

3. Configure alertas de Grafana para `hitstor_agents_stale > 0` y `rate(hitstor_backups_failed_total[1h]) > 0`.

## Alertas por webhook { #webhook-alerting }

HitStor puede enviar notificaciones por webhook cuando fallan sesiones de respaldo, cuando los agentes quedan inactivos o cuando Sentinel detecta una amenaza. Cada evento es un `POST` JSON con `Content-Type: application/json`.

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
| `alert_notifier_plugins` | list | (ninguno) | [Plugins notificadores](#notifier-plugins) que se cargan |

Estas claves también se pueden definir en **Configuración → Notificaciones**, que permite enviar un evento de prueba. Se leen al iniciar: reinicie el servidor después de cambiarlas.

### Eventos de alerta { #alert-events }

Todos los eventos tienen los mismos campos: `event`, `server` (siempre `hitstor`), `timestamp`, `agent`, `hostname` (cuando se conoce), `message` y `details`.

#### `backup_failed`

Se envía inmediatamente cuando una sesión de respaldo falla en el servidor: el agente se desconecta durante la carga o la instantánea no se puede finalizar. Las fallas que el agente informa antes de cargar datos (por ejemplo, una base de datos que no se puede leer) se muestran en la página de la tarea y en los [reportes](reports.md), pero no envían un webhook.

```json
{
  "event": "backup_failed",
  "server": "hitstor",
  "timestamp": "2026-02-21T03:05:00Z",
  "agent": "web-server-01",
  "hostname": "web-server-01",
  "message": "backup failed: agent disconnected during upload",
  "details": {"session_id": "3f9a...", "chunks_received": 1520}
}
```

#### `agent_stale`

Se envía cuando un agente no ha completado un respaldo dentro del umbral de inactividad. El servidor lo comprueba cada 5 minutos; los agentes que nunca han realizado un respaldo no se notifican.

```json
{
  "event": "agent_stale",
  "server": "hitstor",
  "timestamp": "2026-02-21T14:00:00Z",
  "agent": "staging-app",
  "message": "no backup in 2d1h (threshold: 1d1h)",
  "details": {"last_backup": "2026-02-19T12:30:00Z", "threshold_hours": 25}
}
```

#### `threat_detected`

Lo envía [Ransomware Sentinel](sentinel.md) cuando informa una amenaza con una confianza igual o superior a su umbral.

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
plugin_dir: /usr/lib/hitstor/plugins
alert_notifier_plugins:
  - slack
  - pagerduty
```

Cada nombre corresponde a un binario `hitstor-notifier-<name>`, que se busca en `plugin_dir` y luego en el `PATH`. Los plugins notificadores reciben los mismos eventos que el webhook (mediante la interfaz gRPC de plugins) y pueden transformarlos para su sistema de destino. Consulte la documentación de [Plugins](../user-guide/plugins.md) para obtener detalles sobre cómo escribir e instalar plugins.

## Próximos pasos { #next-steps }

- [Dashboard](dashboard.md) -- vea las métricas y el estado de los agentes en la interfaz web
- [Programación](scheduling.md) -- configure las programaciones de respaldo para mantener activos los agentes
- [Solución de problemas](troubleshooting.md) -- diagnostique agentes inactivos y respaldos fallidos
