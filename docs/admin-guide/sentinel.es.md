# Sentinel contra ransomware

HitStor Sentinel (`internal/sentinel`) analiza la telemetría de las sesiones de respaldo en busca de comportamientos propios de ransomware mediante **Google Gemini**. Cuando el riesgo es elevado, puede alertar a los administradores y aplicar acciones de protección, como la suspensión del agente y bloqueos de retención en el almacenamiento.

## Cómo funciona { #how-it-works }

Cada vez que finaliza una sesión de respaldo, el servidor:

1. Reúne hasta 10 instantáneas (snapshots) anteriores del mismo host como línea base.
2. Envía a Gemini la telemetría de la sesión actual junto con esa línea base para su análisis.
3. Si el análisis informa una amenaza con una confianza igual o superior a `HITSTOR_SENTINEL_THRESHOLD` (predeterminado `0.8`), el servidor:
   - Suspende las credenciales del agente, lo que bloquea nuevos respaldos desde ese host.
   - Bloquea el bucket de almacenamiento configurado con una retención inmutable, de modo que el ransomware no pueda eliminar las instantáneas históricas ni siquiera con credenciales válidas.
   - Emite una alerta, visible mediante `GET /api/alerts` y en cualquier webhook o notificador configurado.

Los pasos de contención anteriores solo se ejecutan por encima del umbral de confianza: en la mayoría de los respaldos, Sentinel no encuentra nada y no hace ruido.

## Señales de telemetría { #telemetry-signals }

- Tasas de compresión de FastCDC (los datos cifrados o aleatorios se comprimen mal)
- Tasas masivas de modificación o eliminación de archivos
- Patrones de anomalías a nivel de sesión comparados con el propio historial del host

## Configuración { #configuration }

Sentinel está habilitado de forma predeterminada al iniciar el servidor. Se configura completamente mediante variables de entorno que se leen al iniciar `hitstor serve`; no existe un equivalente con `hitstor config set`. Cuando las variables específicas de Sentinel no están definidas, la clave de API y el modelo de Gemini se toman de los valores cifrados `integrations.*` de `config.yaml` (definidos en **Configuración → Integraciones**).

Consulte [Variables de entorno → Sentinel](../reference/environment-variables.md#sentinel-server) para ver la lista completa de variables, sus valores predeterminados y los tres backends de bloqueo de retención admitidos (GCS, S3, Azure Blob).

Configure las alertas junto con el webhook y los plugins notificadores existentes:

```yaml
alert_webhook_url: https://hooks.slack.com/services/...
alert_notifier_plugins:
  - slack
plugin_dir: /usr/local/lib/hitstor/plugins
```

## Alertas en el dashboard { #dashboard-alerts }

Las alertas activas de Sentinel se obtienen mediante `GET /api/alerts`. Resuelva los incidentes atendidos con `POST /api/alerts/{id}/resolve`.

## Bloqueo de retención { #retention-lock }

En los backends de almacenamiento de objetos compatibles (GCS, S3, Azure Blob), Sentinel puede activar bloqueos de retención inmutables en el bucket de respaldo para impedir que el ransomware elimine las instantáneas históricas, incluso con credenciales válidas y no revocadas. Esto es independiente del backend propio del repositorio (local/S3); el bloqueo se aplica al bucket indicado por `HITSTOR_SENTINEL_BACKEND_TYPE` y su variable de bucket.

## Véase también { #see-also }

- [Monitoreo](monitoring.md)
- [Seguridad](security.md)
- [Variables de entorno](../reference/environment-variables.md)
