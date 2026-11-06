# Actualización

Esta página explica cómo actualizar una instalación existente de HitStor. Los repositorios son compatibles hacia adelante: las versiones más nuevas de HitStor pueden leer y escribir repositorios creados con versiones anteriores sin ningún paso de migración.

## Antes de actualizar { #before-you-upgrade }

1. **Revise su versión actual y las actualizaciones disponibles:**

    ```bash
    hitstor version
    hitstor version --check
    ```

    !!! tip "Consejo"
        Con `update_check: true` en la configuración del servidor (o en **Configuración → General**), el servidor busca actualizaciones cada 24 horas y muestra un aviso en el dashboard web cuando hay una versión más nueva disponible. La comprobación está desactivada si no se habilita.

2. **Respalde sus archivos de configuración.** Aunque las actualizaciones conservan la configuración, es una buena práctica guardar una copia antes de continuar:

    ```bash
    # Linux
    sudo cp -r /etc/hitstor /etc/hitstor.bak

    # Windows (PowerShell)
    Copy-Item -Recurse "$env:ProgramData\HitStor" "$env:ProgramData\HitStor.bak"
    ```

3. **Revise las notas de la versión** en la página de [GitHub Releases](https://github.com/hooperits/hitstor/releases) para conocer cualquier cambio incompatible o paso manual requerido.

## Instalación rápida en Linux (.run) { #linux-quick-install-run }

Ejecute el nuevo instalador `.run` sobre una instalación existente. El instalador detecta automáticamente la actualización, muestra la comparación de versiones y se encarga de todo el proceso:

```bash
sudo ./hitstor-server-X.Y.Z-linux-amd64.run
```

El proceso de actualización:

1. Muestra la versión actual y la nueva para que usted confirme
2. Respalda el binario actual
3. Detiene el servicio e instala el nuevo binario y las unidades de systemd
4. **Nunca sobrescribe** sus archivos de configuración en uso (solo actualiza los archivos de ejemplo)
5. Reinicia el servicio y verifica que esté en buen estado
6. Si la verificación falla: **revierte automáticamente** a la versión anterior

Use `--no-wizard` para omitir la solicitud de confirmación en actualizaciones automatizadas.

El mismo proceso funciona para el agente:

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

## Paquetes de Linux { #linux-packages }

Descargue el nuevo paquete desde [GitHub Releases](https://github.com/hooperits/hitstor/releases) e instálelo sobre el existente.

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -U hitstor-x.y.z-1.x86_64.rpm
    ```

Lo mismo aplica al paquete del agente:

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor-agent_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -U hitstor-agent-x.y.z-1.x86_64.rpm
    ```

Reinicie los servicios después de actualizar:

```bash
sudo systemctl restart hitstor-server
sudo systemctl restart hitstor-agent
```

Verifique la nueva versión:

```bash
hitstor version
```

## Docker { #docker }

Vuelva a compilar la imagen y reinicie el contenedor:

```bash
# Obtener el código fuente más reciente (si compila desde el código fuente)
git pull

# Volver a compilar la imagen
docker build -t hitstor:latest .

# Reiniciar con la nueva imagen
docker compose -f docker-compose.prod.yml down
docker compose -f docker-compose.prod.yml up -d
```

O bien, si usa una etiqueta de versión específica:

```bash
docker build -t hitstor:0.2.0 \
  --build-arg VERSION=0.2.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%d) .
```

Actualice la variable `HITSTOR_VERSION` en su archivo `.env` y luego ejecute:

```bash
docker compose -f docker-compose.prod.yml down
docker compose -f docker-compose.prod.yml up -d
```

Verifique:

```bash
docker compose -f docker-compose.prod.yml exec hitstor hitstor version
```

Los datos del repositorio en el volumen `/var/lib/hitstor` se conservan entre reinicios del contenedor.

## Windows { #windows }

Ejecute el nuevo instalador sobre la instalación existente. El instalador NSIS sobrescribe el binario anterior y actualiza las entradas del registro de Windows, conservando sus archivos de configuración en `%ProgramData%\HitStor\`.

1. Descargue el nuevo instalador desde [GitHub Releases](https://github.com/hooperits/hitstor/releases).
2. Ejecute el instalador:

    ```
    hitstor-x.y.z-windows-amd64-setup.exe
    ```

3. Siga el asistente de instalación. Use el mismo directorio de instalación que en la versión anterior.
4. Abra una nueva ventana de PowerShell y verifique:

    ```powershell
    hitstor version
    ```

Para el agente, descargue y ejecute el instalador del agente:

```
hitstor-agent-x.y.z-windows-amd64-setup.exe
```

Si el agente se ejecuta como servicio de Windows, se detiene automáticamente durante la actualización y se reinicia al finalizar.

## Compilar desde el código fuente { #build-from-source }

Obtenga los cambios más recientes y vuelva a compilar:

```bash
cd /path/to/hitstor
git pull
make build
make agent
```

Reemplace los binarios instalados:

```bash
sudo cp bin/hitstor /usr/local/bin/
sudo cp bin/hitstor-agent /usr/local/bin/
```

Verifique:

```bash
hitstor version
```

Si se ejecuta como servicio de systemd, reinícielo:

```bash
sudo systemctl restart hitstor-server
```

## Después de actualizar desde 0.1.x { #after-upgrading-from-01x }

La versión 0.2.0 reemplaza la contraseña única y compartida del dashboard por cuentas personales, roles y verificación en dos pasos obligatoria. Después de la actualización, el dashboard se abre en **Cree el primer administrador**:

1. Ingrese la contraseña actual del dashboard (la que definió en `serve_dashboard_password` o la que el servidor mostró en su registro).
2. Elija el nombre de usuario y la contraseña del administrador (al menos 12 caracteres).
3. Escanee el código QR con una app de autenticación, ingrese un código y guarde los códigos de recuperación.

A partir de ese momento, la contraseña compartida ya no permite iniciar sesión. Cree cuentas para las demás personas que usan el dashboard en **Usuarios y acceso**, o con `hitstor serve user add` en el servidor. Las cuentas se guardan en el primer repositorio, junto al registro de agentes. Consulte [Usuarios y acceso](../admin-guide/users.md).

## Compatibilidad del repositorio { #repository-compatibility }

Los repositorios de HitStor son compatibles hacia adelante. Un repositorio creado con una versión anterior puede ser leído y escrito por cualquier versión más nueva sin migración. No existe un comando de migración aparte ni un paso de actualización del repositorio.

Si alguna versión futura llegara a requerir un cambio en el formato del repositorio, se documentará en las notas de la versión con instrucciones de migración explícitas.

## Reversión { #rollback }

**Instalación rápida en Linux (.run):** El instalador `.run` revierte automáticamente si la verificación posterior a la actualización falla. Se restaura el binario anterior y el servicio se reinicia con la versión anterior, sin necesidad de intervención manual.

**Otros métodos:** Si necesita volver a una versión anterior, instale el paquete o binario más antiguo con el mismo método de instalación y reinicie el servicio.

Los repositorios son compatibles hacia adelante, por lo que volver a una versión anterior es seguro siempre que no se hayan introducido cambios incompatibles en el formato del repositorio (lo cual se documentaría en las notas de la versión).
