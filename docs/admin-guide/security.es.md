# Seguridad

Esta página documenta el modelo de seguridad de HitStor: cifrado, gestión de claves, autenticación y endurecimiento del sistema.

## Cifrado { #encryption }

### Cifrado de datos { #data-encryption }

Todos los datos almacenados en el repositorio se cifran con **AES-256-GCM** (Galois/Counter Mode). El cifrado se aplica del lado del cliente antes de que los datos se escriban en el almacenamiento, lo que garantiza que el contenido del repositorio sea ilegible sin la contraseña.

Cada blob (fragmento (chunk) de datos) se cifra con un nonce único. El cifrado autenticado que ofrece GCM garantiza tanto la confidencialidad como la integridad: cualquier manipulación de los datos cifrados se detecta al leerlos.

### Derivación de claves { #key-derivation }

La clave de cifrado se deriva de la contraseña del repositorio mediante **scrypt** con los siguientes parámetros:

| Parámetro | Valor | Propósito |
|-----------|-------|---------|
| N | 32768 | Factor de costo de CPU/memoria |
| r | 8 | Tamaño de bloque |
| p | 1 | Factor de paralelización |

Estos parámetros están diseñados para que los ataques de fuerza bruta resulten computacionalmente costosos. La salida de scrypt produce dos claves:

- **EncKey**: se usa para el cifrado AES-256-GCM de los blobs de datos
- **IDKey**: se usa para la identificación de blobs con HMAC-SHA256

### Identificación de blobs { #blob-identification }

Los ID de blob se calculan como **HMAC-SHA256** usando la IDKey derivada de la contraseña. Esto evita los ataques de adivinación de contenido: un atacante que conozca el contenido en texto plano de un archivo no puede determinar si existe en el repositorio sin conocer la contraseña.

```
BlobID = HMAC-SHA256(IDKey, plaintext)
```

Sin la IDKey, un atacante no puede correlacionar texto plano conocido con los blobs almacenados.

## Seguridad del transporte { #transport-security }

### TLS { #tls }

Toda la comunicación gRPC entre los agentes y el servidor se cifra con TLS. El servidor genera automáticamente un certificado autofirmado o usa certificados personalizados proporcionados por el administrador.

| Configuración | Cifrado | Verificación del certificado |
|---------------|-----------|-------------------------|
| Generado automáticamente (predeterminado) | Sí | El agente debe configurar `agent_ca_cert` |
| Certificado personalizado | Sí | Verificación estándar de CA o `agent_ca_cert` |
| Let's Encrypt | Sí | Verificación estándar de CA (sin configuración adicional) |

TLS siempre está habilitado. No existe una opción para deshabilitar el cifrado del transporte.

Consulte [Certificados TLS](tls-certificates.md) para ver la configuración detallada.

## Autenticación { #authentication }

### Autenticación de agentes { #agent-authentication }

HitStor usa dos mecanismos de autenticación para los agentes:

**Tokens de agente** (agentes gestionados desde el dashboard):

- Cadena hexadecimal de 64 caracteres (256 bits de entropía)
- Se generan automáticamente durante la instalación del agente
- Se almacenan en `/etc/hitstor/agent-token` en el agente (modo 0600)
- Se almacenan cifrados (AES-256-GCM) en el servidor, en el registro de agentes
- El servidor los usa para autenticar las llamadas gRPC al agente (exploración del sistema de archivos, configuración)
- Se pueden recuperar en cualquier momento con `hitstor-agent show-id`
- Se verifican mediante una comparación de tiempo constante para evitar ataques de temporización

**Claves de API** (todos los agentes):

- Cadena hexadecimal de 64 caracteres (256 bits de entropía)
- Una clave por agente
- Se generan con un generador de números aleatorios criptográficamente seguro
- Para agentes gestionados desde el dashboard: se envían al agente automáticamente durante el registro
- Para agentes registrados con la CLI: se muestran una sola vez al momento del registro y no se pueden volver a recuperar
- Se almacenan como hash en el registro cifrado de agentes

Un agente debe presentar una clave de API válida en cada solicitud gRPC de respaldo. El servidor verifica la clave contra el valor hash almacenado en el registro.

### Contraseña del repositorio { #repository-password }

La contraseña del repositorio es el secreto raíz de todas las operaciones de cifrado. Esta contraseña:

- **Nunca se almacena en disco** en el servidor (se proporciona en tiempo de ejecución mediante una variable de entorno o un archivo de contraseña)
- Se usa para derivar EncKey e IDKey mediante scrypt
- Es necesaria para leer o escribir cualquier dato en el repositorio

Si se pierde la contraseña, los datos del repositorio son irrecuperables.

### Autenticación del dashboard { #dashboard-authentication }

El dashboard web siempre requiere iniciar sesión. La contraseña proviene de `serve_dashboard_password`; si no está configurada, el servidor genera una contraseña aleatoria al iniciar, almacena su hash scrypt en el archivo de configuración y registra la contraseña en texto plano una sola vez para que el operador pueda iniciar sesión y cambiarla.

Las cookies de sesión tienen las siguientes propiedades de seguridad:

| Propiedad | Valor | Propósito |
|----------|-------|---------|
| `HttpOnly` | `true` | Impide que JavaScript acceda a la cookie |
| `SameSite` | `Strict` | Previene la falsificación de solicitudes entre sitios (CSRF) |

El endpoint `/metrics` está excluido de la autenticación para permitir la recolección de métricas por Prometheus. Si es necesario, restrinja el acceso a este endpoint a nivel de red.

## Endurecimiento con systemd { #systemd-hardening }

La unidad systemd del servidor HitStor incluye directivas de endurecimiento de seguridad que restringen el proceso del servidor:

```ini
[Service]
# Filesystem protection
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/hitstor

# Privilege restrictions
NoNewPrivileges=true
PrivateTmp=true

# Capability restrictions
CapabilityBoundingSet=
AmbientCapabilities=

# Network
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX

# System call filtering
SystemCallFilter=@system-service
SystemCallArchitectures=native

# Namespace isolation
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
```

Estas directivas:

- Impiden que el servidor escriba fuera de su directorio de datos
- Bloquean la escalada de privilegios
- Restringen las llamadas al sistema al conjunto mínimo necesario
- Aíslan el proceso de los parámetros ajustables del kernel y del acceso a dispositivos

### Verificar el endurecimiento { #verifying-hardening }

Revise la puntuación de seguridad de la unidad:

```bash
systemd-analyze security hitstor-server
```

Una puntuación inferior a 5.0 indica un buen endurecimiento. Revise los elementos marcados como "UNSAFE" y evalúe si son necesarios para su implementación.

## Lista de verificación de seguridad { #security-checklist }

Use esta lista de verificación al implementar HitStor en producción:

- [ ] Use una contraseña de repositorio robusta (al menos 16 caracteres, generada aleatoriamente)
- [ ] Almacene la contraseña en un archivo con permisos `0600`, no en la línea de comandos
- [ ] Use certificados TLS personalizados emitidos por una CA de confianza
- [ ] Configure una contraseña para el dashboard (`serve_dashboard_password`)
- [ ] Restrinja el puerto del dashboard (8421) a las redes de administración
- [ ] Restrinja el endpoint `/metrics` a las IP del recolector de Prometheus
- [ ] Habilite el endurecimiento con systemd (incluido de forma predeterminada en las unidades empaquetadas)
- [ ] Asegúrese de que los archivos de token de agente (`/etc/hitstor/agent-token`) tengan permisos `0600`
- [ ] Restrinja el puerto gRPC del agente (8422) a la dirección IP del servidor
- [ ] Rote las claves de API si sospecha que fueron comprometidas (elimine y vuelva a registrar el agente)
- [ ] Ejecute `hitstor check --read-data` periódicamente para verificar la integridad del repositorio
- [ ] Mantenga HitStor actualizado para recibir las correcciones de seguridad

## Próximos pasos { #next-steps }

- [Certificados TLS](tls-certificates.md): configure TLS para producción
- [Gestión de agentes](agent-management.md): registro y rotación de claves de API
- [Solución de problemas](troubleshooting.md): diagnostique problemas relacionados con la seguridad
