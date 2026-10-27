# Instalación en Windows Server

HitStor ofrece un instalador NSIS para implementar el servidor de respaldos en Windows. El instalador agrega `hitstor.exe` al PATH del sistema, crea el directorio de configuración, registra HitStor como servicio de Windows y registra un desinstalador en Agregar o quitar programas (Add/Remove Programs).

## Requisitos { #requirements }

- Windows 10 (1809+) o Windows Server 2016+
- Arquitectura: amd64
- Privilegios de administrador (necesarios para la instalación)

## Descarga { #download }

Descargue el instalador del servidor desde la página de [GitHub Releases](https://github.com/hooperits/hitstor/releases):

- `hitstor-x.y.z-windows-amd64-setup.exe`

## Instalación { #install }

1. Ejecute el instalador:

    ```
    hitstor-x.y.z-windows-amd64-setup.exe
    ```

2. Acepte el acuerdo de licencia.
3. Elija el directorio de instalación (predeterminado: `C:\Program Files\HitStor\`).
4. Elija los componentes (ambos están seleccionados de forma predeterminada):
    - **Windows Firewall rules**: permite el tráfico TCP entrante en los puertos 8420 (agentes) y 8421 (dashboard) para `hitstor.exe`.
    - **Install as Windows Service**: registra e inicia el servicio `HitStor`.
5. Haga clic en **Install**.

El instalador realiza los siguientes pasos:

- Detiene el servicio `HitStor` si ya está instalado (actualización).
- Copia `hitstor.exe` al directorio de instalación y lo agrega al `PATH` del sistema.
- Crea `%ProgramData%\HitStor\`, con acceso restringido a Administradores y SYSTEM, junto con `config.yaml.example`.
- Crea `%ProgramData%\HitStor\config.yaml` a partir del ejemplo si no existe (si ya existe, se conserva).
- Registra el servicio de Windows `HitStor`, que ejecuta `hitstor serve --config %ProgramData%\HitStor\config.yaml` como LocalSystem con inicio automático, y lo inicia.
- Registra la aplicación en Agregar o quitar programas.

### Instalación y actualización desatendidas { #unattended-install-and-upgrade }

Ejecute el instalador con `/S` desde una consola con privilegios elevados. Se instalan todos los componentes, y ejecutar el mismo comando con un instalador más nuevo actualiza la instalación en el mismo lugar (se detiene el servicio, se reemplaza el binario y el servicio se vuelve a registrar e iniciar):

```powershell
.\hitstor-x.y.z-windows-amd64-setup.exe /S
```

## Verificar la instalación { #verify-the-installation }

Abra una nueva ventana de PowerShell o del Símbolo del sistema (necesario para que el PATH actualizado surta efecto) y ejecute:

```powershell
hitstor version
Get-Service HitStor
```

`hitstor version` muestra la versión, el hash del commit y la fecha de compilación.

## Primer inicio de sesión { #first-login }

Si no hay repositorios configurados, el servidor arranca en modo de configuración inicial (solo dashboard) y lo sirve por HTTP sin cifrar. Abra `http://<server>:8421`. HTTPS (con certificado autofirmado de forma predeterminada) se habilita una vez que se configura un repositorio y se reinicia el servicio.

No existe una contraseña predeterminada para el dashboard. Si `config.yaml` no tiene una, el servidor genera una en el primer inicio, guarda su hash en `config.yaml` y registra la contraseña en texto plano una sola vez en `%ProgramData%\HitStor\logs\server.log`. Léala en el servidor desde una consola de PowerShell con privilegios elevados:

```powershell
Select-String -Path "$env:ProgramData\HitStor\logs\server.log" -Pattern "generated one" |
  Select-Object -Last 1 |
  ForEach-Object { ($_.Line | ConvertFrom-Json).fields.password }
```

Inicie sesión con ella y cámbiela en **Configuración**. Luego cree un repositorio desde la página **Almacenamiento**. Para restablecer una contraseña perdida, consulte [Contraseña del dashboard](quickstart.md#dashboard-password).

## Configuración { #configuration }

Edite `%ProgramData%\HitStor\config.yaml` (consulte `config.yaml.example` en la misma carpeta para ver todas las opciones) y reinicie el servicio:

```powershell
notepad "$env:ProgramData\HitStor\config.yaml"
Restart-Service HitStor
```

## Administrar el servicio { #managing-the-service }

El instalador administra el servicio por usted. Para hacerlo manualmente (consola con privilegios elevados):

```powershell
hitstor service install --config "$env:ProgramData\HitStor\config.yaml"
hitstor service start
hitstor service stop
hitstor service uninstall
```

Todos los subcomandos aceptan `--name` (predeterminado: `HitStor`). También puede usar `Start-Service`/`Stop-Service HitStor` o la consola de Servicios. El servicio escribe en `%ProgramData%\HitStor\logs\server.log`.

Para ejecutar el servidor en primer plano (para solucionar problemas), detenga el servicio y ejecute:

```powershell
hitstor serve --config "$env:ProgramData\HitStor\config.yaml"
```

## Configuración del firewall { #firewall-configuration }

El componente **Windows Firewall rules** crea las reglas `HitStor Server gRPC` (8420) y `HitStor Server Dashboard` (8421), abiertas a cualquier dirección remota. Para restringirlas, por ejemplo a una red de administración:

```powershell
Set-NetFirewallRule -DisplayName "HitStor Server Dashboard" -RemoteAddress 10.0.0.0/24
```

Si omitió ese componente, cree las reglas usted mismo:

```powershell
New-NetFirewallRule -DisplayName "HitStor gRPC" -Direction Inbound -LocalPort 8420 -Protocol TCP -Action Allow
New-NetFirewallRule -DisplayName "HitStor Dashboard" -Direction Inbound -LocalPort 8421 -Protocol TCP -Action Allow
```

## Desinstalación { #uninstall }

Use **Agregar o quitar programas** en la Configuración de Windows, o ejecute el desinstalador directamente:

```powershell
& "$env:ProgramFiles\HitStor\uninstall.exe"
```

El desinstalador detiene y elimina el servicio y las reglas de firewall, y elimina el binario, la entrada del PATH, los accesos directos del menú Inicio y las entradas del registro de Windows. No se eliminan los archivos de configuración de `%ProgramData%\HitStor\` ni los datos del repositorio.

## Próximos pasos { #next-steps }

- [Instalación del agente en Windows](windows-agent.md): instale el agente en estaciones de trabajo Windows.
- [Primeros pasos](../user-guide/getting-started.md): cree su primer respaldo.
- [Configuración](../user-guide/configuration.md): opciones del archivo de configuración y variables de entorno.
