# Seguridad

Esta página documenta el modelo de seguridad de HitStor: cifrado, gestión de claves, autenticación y endurecimiento del sistema.

## Cifrado { #encryption }

### Cifrado de datos { #data-encryption }

Todos los datos almacenados en el repositorio se cifran con **AES-256-GCM** (Galois/Counter Mode). El cifrado se aplica antes de que los datos se escriban en el almacenamiento (lo hace `hitstor` en los respaldos locales y el servidor en los respaldos de agentes; los agentes fragmentan y comprimen localmente y envían los fragmentos por TLS), lo que garantiza que el contenido del repositorio sea ilegible sin la contraseña.

Cada blob (fragmento (chunk) de datos) se cifra con un nonce único. El cifrado autenticado que ofrece GCM garantiza tanto la confidencialidad como la integridad: cualquier manipulación de los datos cifrados se detecta al leerlos.

### Derivación de claves { #key-derivation }

Cada repositorio tiene una **clave maestra** aleatoria de 64 bytes: una **EncKey** (cifrado AES-256-GCM de los blobs) y una **IDKey** (identificación de blobs con HMAC-SHA256). La clave maestra se guarda en el directorio `keys/` del repositorio, cifrada con una clave que se deriva de la contraseña del repositorio mediante **scrypt**, con los siguientes parámetros y una sal aleatoria de 64 bytes:

| Parámetro | Valor | Propósito |
|-----------|-------|---------|
| N | 32768 | Factor de costo de CPU/memoria |
| r | 8 | Tamaño de bloque |
| p | 1 | Factor de paralelización |

Estos parámetros están diseñados para que los ataques de fuerza bruta resulten computacionalmente costosos. Como las claves de datos son aleatorias y no se derivan de la contraseña, adivinar la contraseña es la única forma de llegar a ellas.

### Identificación de blobs { #blob-identification }

Los ID de blob se calculan como **HMAC-SHA256** usando la IDKey del repositorio. Esto evita los ataques de adivinación de contenido: un atacante que conozca el contenido en texto plano de un archivo no puede determinar si existe en el repositorio sin conocer la contraseña.

```
BlobID = HMAC-SHA256(IDKey, plaintext)
```

Sin la IDKey, un atacante no puede correlacionar texto plano conocido con los blobs almacenados.

## Seguridad del transporte { #transport-security }

### TLS { #tls }

Toda la comunicación gRPC entre los agentes y el servidor se cifra con TLS. El servidor genera automáticamente un certificado autofirmado o usa certificados personalizados proporcionados por el administrador.

| Configuración | Cifrado | Verificación del certificado |
|---------------|-----------|-------------------------|
| Generado automáticamente (predeterminado) | Sí | El agente fija el certificado del servidor en la primera conexión, o confía en él mediante `agent_ca_cert` |
| Certificado personalizado | Sí | Verificación estándar de CA o `agent_ca_cert` |
| Let's Encrypt | Sí | Verificación estándar de CA (sin configuración adicional) |

TLS siempre está habilitado. No existe una opción para deshabilitar el cifrado del transporte.

Consulte [Certificados TLS](tls-certificates.md) para ver la configuración detallada.

## Autenticación { #authentication }

### Autenticación de agentes { #agent-authentication }

HitStor usa dos mecanismos de autenticación para los agentes:

**Tokens de agente** (agentes gestionados desde el dashboard):

- Cadena hexadecimal de 64 caracteres (256 bits de entropía)
- Se generan automáticamente la primera vez que se inicia el agente
- Se almacenan en `/etc/hitstor/agent-token` en el agente (modo 0600; `C:\ProgramData\HitStor\agent-token` en Windows, legible solo por SYSTEM y Administradores)
- Se almacenan cifrados (AES-256-GCM) en el servidor, en el registro de agentes
- El servidor los usa para autenticar las llamadas gRPC al agente (registro, exploración, instalación de plugins, restauraciones)
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

- Se proporciona en tiempo de ejecución mediante un archivo de contraseña (`password_file` en cada repositorio), la variable de entorno `HITSTOR_PASSWORD` o `--password`/`--password-file`. Los repositorios creados desde el dashboard la guardan en un archivo de contraseña junto al archivo de configuración (`<name>.password`, modo `0600`): proteja ese directorio
- Se usa, mediante scrypt, para desbloquear la clave maestra (EncKey e IDKey)
- Es necesaria para leer o escribir cualquier dato en el repositorio

Si se pierde la contraseña, los datos del repositorio son irrecuperables.

### Autenticación del dashboard { #dashboard-authentication }

El dashboard usa cuentas personales con roles limitados por agente y verificación en dos pasos (TOTP) obligatoria. Las contraseñas se guardan como hashes argon2id; las sesiones expiran por inactividad, los cambios sensibles piden iniciar sesión de nuevo, las cuentas se bloquean tras fallos repetidos y una auditoría encadenada por hashes registra inicios de sesión, cambios y solicitudes denegadas. Consulte [Usuarios y acceso](users.md).

Las cookies de sesión tienen las siguientes propiedades de seguridad:

| Propiedad | Valor | Propósito |
|----------|-------|---------|
| `HttpOnly` | `true` | Impide que JavaScript acceda a la cookie |
| `SameSite` | `Lax` | No se envía en subsolicitudes entre sitios; además, los cambios desde otros sitios se rechazan por verificación de origen |
| `Secure` | con HTTPS | No se envía por HTTP sin cifrar |

El endpoint `/metrics` está excluido de la autenticación para permitir la recolección de métricas por Prometheus. Si es necesario, restrinja el acceso a este endpoint a nivel de red.

## Endurecimiento con systemd { #systemd-hardening }

La unidad systemd del servidor HitStor incluye directivas de endurecimiento de seguridad que restringen el proceso del servidor:

```ini
[Service]
User=hitstor
Group=hitstor

# Filesystem protection
ProtectSystem=strict
ReadWritePaths=/var/lib/hitstor /var/log/hitstor /etc/hitstor
ProtectHome=true
PrivateTmp=true

# Privilege restrictions
NoNewPrivileges=true

# Kernel protection
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
```

Estas directivas:

- Ejecutan el servidor con el usuario sin privilegios `hitstor`
- Impiden que el servidor escriba fuera de `/var/lib/hitstor`, `/var/log/hitstor` y `/etc/hitstor`
- Bloquean la escalada de privilegios
- Aíslan el proceso de los directorios personales, los parámetros ajustables del kernel, los módulos del kernel y los grupos de control

Si el servicio falla con `status=226/NAMESPACE` (algunas VM de Hyper-V, contenedores, WSL), el archivo de la unidad indica qué líneas comentar.

### Verificar el endurecimiento { #verifying-hardening }

Revise la puntuación de seguridad de la unidad:

```bash
systemd-analyze security hitstor-server
```

Cuanto menor es la puntuación, más restringido está el servicio. Revise los elementos marcados como "UNSAFE" y evalúe si puede restringirlos más con un archivo complementario (`systemctl edit hitstor-server`).

## Lista de verificación de seguridad { #security-checklist }

Use esta lista de verificación al implementar HitStor en producción:

- [ ] Use una contraseña de repositorio robusta (al menos 16 caracteres, generada aleatoriamente)
- [ ] Almacene la contraseña en un archivo con permisos `0600`, no en la línea de comandos
- [ ] Use certificados TLS personalizados emitidos por una CA de confianza (el dashboard usa el mismo certificado)
- [ ] Cree cuentas personales con el menor rol que cada persona necesite y elimine las que ya no se usen
- [ ] Restrinja el puerto del dashboard (8421) a las redes de administración
- [ ] Restrinja el endpoint `/metrics` a las IP del recolector de Prometheus
- [ ] Habilite el endurecimiento con systemd (incluido de forma predeterminada en las unidades empaquetadas)
- [ ] Asegúrese de que los archivos de token de agente (`/etc/hitstor/agent-token`) tengan permisos `0600`
- [ ] Restrinja el puerto gRPC del agente (8422) a la dirección IP del servidor
- [ ] Rote las claves de API si sospecha que fueron comprometidas (vuelva a registrar el agente; consulte [Gestión de agentes](agent-management.md#rotating-an-api-key))
- [ ] Ejecute `hitstor check --read-data` periódicamente para verificar la integridad del repositorio
- [ ] Mantenga HitStor actualizado para recibir las correcciones de seguridad

## Próximos pasos { #next-steps }

- [Certificados TLS](tls-certificates.md): configure TLS para producción
- [Gestión de agentes](agent-management.md): registro y rotación de claves de API
- [Solución de problemas](troubleshooting.md): diagnostique problemas relacionados con la seguridad
