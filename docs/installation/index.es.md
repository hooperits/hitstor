# Instalación

HitStor ofrece dos binarios:

- **`hitstor`**: el servidor de respaldos, la herramienta de línea de comandos (CLI) y el dashboard web.
- **`hitstor-agent`**: un agente remoto liviano que envía respaldos cifrados y deduplicados a un servidor HitStor mediante gRPC.

Elija el método de instalación que mejor se adapte a su entorno.

**¿Es nuevo en HitStor?** Comience con la guía de [Inicio rápido](quickstart.md): servidor en Docker + binarios del agente en unos 15 minutos.

## Métodos de instalación { #installation-methods }

| Método | Plataformas | Ideal para |
|--------|-----------|----------|
| [Inicio rápido](quickstart.md) | Docker + agentes Linux/Windows | La ruta más rápida: servidor en Docker, instalador en VM o solo agente |
| [Instalación rápida en Linux (.run)](linux-installer.md) | Ubuntu, Debian, RHEL, Fedora, AlmaLinux | Asistente interactivo: configura, inicializa e inicia todo. Incluye actualización con reversión y desinstalador. |
| [Paquetes de Linux (deb/rpm)](linux-packages.md) | Debian, Ubuntu, RHEL, Fedora, AlmaLinux | Servidores de producción con gestor de paquetes |
| [Compilar desde el código fuente](linux-source.md) | Cualquier SO con Go 1.26+ | Desarrollo, compilaciones personalizadas, arquitecturas no soportadas |
| [Windows Server](windows-server.md) | Windows 10+, Windows Server 2016+ | Ejecutar el servidor HitStor en Windows |
| [Agente de Windows](windows-agent.md) | Windows 10+, Windows Server 2016+ | Respaldar equipos Windows en un servidor HitStor remoto |
| [Docker](docker.md) | Cualquier host con Docker | Implementación en contenedores, pipelines de CI/CD |

## Después de la instalación { #after-installation }

Una vez instalado HitStor, continúe con:

1. [Primeros pasos](../user-guide/getting-started.md): tutorial de principio a fin que cubre la inicialización, el respaldo, la restauración, la verificación y la depuración (prune).
2. [Configuración](../user-guide/configuration.md): esquema del archivo de configuración, variables de entorno y precedencia de los flags.
3. [Referencia de comandos](../reference/commands/index.md): documentación detallada de cada comando.

## Actualización { #upgrading }

Consulte [Actualización](upgrading.md) para ver las instrucciones para actualizar una instalación existente de HitStor.
