# Configuración del servidor

La página **Configuración** (`/settings`) centraliza la configuración del servidor que antes estaba repartida entre variables de entorno, archivos YAML y comandos de la CLI. Sigue los mismos patrones que las consolas de respaldo empresariales: secciones agrupadas, secretos enmascarados, probar antes de guardar y un aviso de reinicio cuando los cambios de red o TLS lo requieren.

## Acceso { #access }

Abra **Sistema → Configuración** en la barra lateral del dashboard, o use un enlace directo (`/settings?tab=integrations`).

## Secciones { #sections }

| Pestaña | Qué se configura |
|-----|-------------------|
| **General** | Búsqueda de actualizaciones, workers en paralelo, escrituras sincrónicas, reserva mínima de espacio libre en disco |
| **Seguridad y TLS** | Contraseña del dashboard, carga del certificado TLS, estado del certificado |
| **Red** | Puerto gRPC, puerto HTTPS del dashboard, dirección anunciada a los agentes |
| **Integraciones** | Clave de API de Gemini (Sentinel) |
| **Notificaciones** | Webhook de alertas, umbral de agente inactivo |

## Integraciones { #integrations }

Configure las integraciones desde **Configuración → Integraciones**:

1. **Clave de API de Gemini**: se guarda cifrada en `config.yaml`; la usa [Ransomware Sentinel](sentinel.md) cuando `HITSTOR_SENTINEL_API_KEY` no está definida
2. **Modelo de Gemini**: el valor predeterminado es `gemini-1.5-pro` (o el predeterminado de Sentinel si no se define)

Gemini es la única integración. Las integraciones con SaaS y proveedores de nube
se eliminaron en v2.0.0 junto con las cargas de trabajo a las que servían.

Use **Probar Gemini** antes de guardar. Si ya tiene un valor en una variable de
entorno, haga clic en **Importar desde el entorno** para migrarlo al YAML cifrado.

## Certificados TLS { #tls-certificates }

Cargue un certificado PEM y su clave privada desde **Seguridad y TLS**. Los archivos se guardan en `/etc/hitstor/tls/` y sus rutas se escriben en `serve_tls_cert` y `serve_tls_key` en el archivo de configuración.

**Se requiere reiniciar** después de la carga: TLS se carga al iniciar el servidor.

La tarjeta de estado muestra el origen (personalizado, generado automáticamente por el repositorio o autofirmado), el sujeto, el emisor, el vencimiento y los SAN.

## Notificaciones { #notifications }

`alert_stale_threshold` (25 horas de forma predeterminada) controla cuándo se marca a los agentes como inactivos para las alertas. Configúrelo desde **Configuración → Notificaciones**.

## API { #api }

Consulte [API HTTP del dashboard: Configuración](../reference/dashboard-api.md#settings).

## Relacionado { #related }

- [Sentinel](sentinel.md)
- [Certificados TLS](tls-certificates.md)
- [Referencia de configuración](../reference/config-reference.md)
