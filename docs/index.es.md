# HitStor

Herramienta de respaldo cifrada y con deduplicación, escrita en Go.

## Resumen { #overview }

HitStor ofrece respaldos seguros y eficientes con cifrado del lado del cliente, deduplicación definida por contenido y compresión. Admite backends de almacenamiento locales y compatibles con S3, agentes de respaldo remotos sobre gRPC/TLS, un dashboard web y una arquitectura de plugins extensible.

## Características principales { #key-features }

- **Cifrado del lado del cliente**: AES-256-GCM con derivación de claves scrypt
- **Deduplicación definida por contenido**: división en fragmentos (chunks) con FastCDC (256 KiB–4 MiB)
- **Compresión**: compresión Zstd por fragmento antes del cifrado
- **Servidor de respaldo remoto**: servidor gRPC con TLS y autenticación por clave de API
- **Dashboard web**: tareas de respaldo, agentes, almacenamiento, restauración y flujos de DR
- **Restauración bare-metal**: medios de recuperación basados en ReaR para hosts Linux
- **Arquitectura de plugins**: almacenamiento, notificadores, hooks y escáneres de sistemas de archivos (go-plugin gRPC)
- **E/S en paralelo**: pools de workers para respaldos y restauraciones más rápidos
- **Un único binario estático**: despliegue sin dependencias con `CGO_ENABLED=0`

## Inicio rápido { #quick-start }

```bash
# Instalar (deb para Linux)
sudo dpkg -i hitstor_0.1.0_amd64.deb

# Inicializar un repositorio
hitstor init --repo /var/lib/hitstor/repo --password "secret"

# Crear un respaldo
hitstor backup --repo /var/lib/hitstor/repo --password "secret" /data

# Listar instantáneas
hitstor snapshots --repo /var/lib/hitstor/repo --password "secret"

# Restaurar
hitstor restore --repo /var/lib/hitstor/repo --password "secret" latest /restore
```

## Secciones de la documentación { #documentation-sections }

| Sección | Descripción |
|---------|-------------|
| [Instalación](installation/index.md) | Instale HitStor mediante paquetes, código fuente, Docker o instaladores de Windows |
| [Guía del usuario](user-guide/index.md) | Aprenda a respaldar, restaurar, gestionar instantáneas (snapshots) y configurar la retención |
| [Administración](admin-guide/index.md) | Servidores, tareas de respaldo, agentes, descubrimiento, DR, monitoreo |
| [Referencia](reference/index.md) | Comandos, configuración, API gRPC, API HTTP del dashboard, funcionamiento interno |
| [Guía de desarrollo](dev-guide/index.md) | Visión general del código, compilación, pruebas y publicación de versiones |
