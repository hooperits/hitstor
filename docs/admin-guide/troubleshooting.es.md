# Solución de problemas

Esta página cubre los problemas comunes de HitStor, sus causas y sus soluciones. Para cada problema, el mensaje de error se muestra tal como aparece en los registros o en la salida de la terminal.

## Errores del repositorio { #repository-errors }

### "repository not initialized" { #repository-not-initialized }

**Causa:** El directorio del repositorio no existe o no se ha inicializado con `hitstor init`.

**Solución:** Inicialice el repositorio antes de iniciar el servidor o de ejecutar respaldos:

```bash
hitstor init --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
```

Si el directorio existe pero parece vacío o dañado, verifique que la ruta sea correcta y que los subdirectorios `keys/`, `data/`, `index/` y `snapshots/` estén presentes.

### "incorrect password" { #incorrect-password }

**Causa:** La contraseña proporcionada no coincide con la contraseña usada al inicializar el repositorio.

**Solución:**

1. Revise la variable de entorno `HITSTOR_PASSWORD`:

    ```bash
    echo $HITSTOR_PASSWORD
    ```

2. Si usa un archivo de contraseña (`--password-file`), verifique que el archivo contenga la contraseña correcta sin un salto de línea final:

    ```bash
    wc -c /etc/hitstor/password.txt
    cat -A /etc/hitstor/password.txt
    ```

3. Si usa systemd, revise el archivo de entorno:

    ```bash
    sudo cat /etc/hitstor/server.env
    ```

4. Asegúrese de que no haya espacios adicionales, comillas ni caracteres de escape en el valor de la contraseña.

### "lock repository: repository is locked" { #lock-repository-repository-is-locked }

**Causa:** Otro proceso de HitStor está usando el repositorio en este momento, o un proceso anterior terminó sin liberar el bloqueo.

**Solución:**

1. Busque otros procesos de HitStor en ejecución:

    ```bash
    ps aux | grep hitstor
    ```

2. Si no hay ningún otro proceso en ejecución, el bloqueo quedó huérfano. Elimínelo:

    ```bash
    hitstor unlock --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
    ```

3. Si usa el servidor, asegúrese de que solo un proceso `hitstor serve` se ejecute sobre un repositorio determinado.

## Errores de TLS { #tls-errors }

### "TLS connection error" { #tls-connection-error }

**Causa:** El agente no puede verificar el certificado TLS del servidor. Las causas más comunes son:

1. El agente no tiene el certificado del servidor (falta `--ca-cert`)
2. El certificado no incluye la IP del servidor en sus SAN
3. El binario del servidor es una versión antigua que genera certificados solo para `127.0.0.1`

**Diagnóstico:** Revise qué IP incluye el certificado:

```bash
openssl x509 -in /var/lib/hitstor/repo/server.crt -noout -text | grep -A1 "Subject Alternative Name"
```

Si solo aparecen `127.0.0.1` y `localhost`, el certificado no contiene la IP real del servidor.

**Solución:**

1. Asegúrese de que el binario del servidor esté actualizado (`hitstor version`)
2. Elimine el certificado antiguo para forzar su regeneración:

    ```bash
    rm -f /var/lib/hitstor/repo/server.crt /var/lib/hitstor/repo/server.key
    ```

3. Reinicie el servidor; este genera un certificado nuevo con todas las IP locales:

    ```bash
    hitstor serve --repo /var/lib/hitstor/repo ...
    ```

4. Verifique que el certificado nuevo incluya la IP correcta:

    ```bash
    openssl x509 -in /var/lib/hitstor/repo/server.crt -noout -text | grep -A1 "Subject Alternative Name"
    # Esperado: IP Address:192.168.x.x en la lista
    ```

5. Copie el certificado al agente y configúrelo:

    ```bash
    scp hits@SERVER_IP:/var/lib/hitstor/repo/server.crt /etc/hitstor/server.crt
    hitstor-agent --ca-cert /etc/hitstor/server.crt ping
    ```

    O mediante la configuración:

    ```yaml
    agent_ca_cert: /etc/hitstor/server.crt
    ```

### "certificate signed by unknown authority" { #certificate-signed-by-unknown-authority }

**Causa:** El agente no confía en el certificado TLS que presenta el servidor. Esto es común con certificados autofirmados o con CA privadas.

**Solución:** Configure el agente para que confíe en el certificado del servidor:

```bash
hitstor-agent --ca-cert /etc/hitstor/server.crt ping
```

O de forma permanente mediante el archivo de configuración:

```yaml
# ~/.config/hitstor/config.yaml
agent_ca_cert: /etc/hitstor/server.crt
```

Para certificados autofirmados (el valor predeterminado), copie el `server.crt` del servidor desde el directorio del repositorio. Consulte [Certificados TLS](tls-certificates.md) para más detalles.

### "tls: bad certificate" { #tls-bad-certificate }

**Causa:** El certificado TLS del servidor no coincide con el nombre de host o la dirección IP a la que se conecta el agente.

**Solución:** Verifique que los SAN del certificado incluyan la IP del servidor:

```bash
openssl x509 -in /var/lib/hitstor/repo/server.crt -noout -text | grep -A1 "Subject Alternative Name"
```

Si falta la IP, elimine el certificado y reinicie el servidor para regenerarlo con todas las IP actuales.

### "tls: private key does not match public key" { #tls-private-key-does-not-match-public-key }

**Causa:** El archivo del certificado y el archivo de la clave no se corresponden entre sí.

**Solución:** Verifique que los archivos coincidan:

```bash
openssl x509 -noout -modulus -in server.crt | md5sum
openssl rsa -noout -modulus -in server.key | md5sum
```

Si las sumas de verificación difieren, los archivos no corresponden. Regenérelos o vuelva a emparejarlos.

## Errores de conexión { #connection-errors }

### "connection refused" { #connection-refused }

**Causa:** El servidor no está en ejecución o un firewall está bloqueando el puerto.

**Solución:**

1. Verifique que el servidor esté en ejecución:

    ```bash
    sudo systemctl status hitstor-server
    ```

2. Verifique que el servidor esté escuchando en el puerto esperado:

    ```bash
    ss -tlnp | grep 8420
    ```

3. Verifique que las reglas de firewall permitan el tráfico en los puertos 8420 y 8421:

    ```bash
    sudo firewall-cmd --list-ports
    # o
    sudo ufw status
    ```

4. Si se conecta entre redes distintas, asegúrese de que no haya firewalls intermedios ni grupos de seguridad bloqueando el puerto.

### El agente no aparece en el dashboard { #agent-not-appearing-in-dashboard }

**Causa:** El agente no se ha conectado correctamente al servidor o no ha completado un respaldo.

**Solución:**

1. Verifique que el agente pueda comunicarse con el servidor:

    ```bash
    hitstor-agent ping
    ```

2. Verifique que la clave de API sea correcta en `/etc/hitstor/agent.env`.

3. Revise los registros del agente en busca de errores:

    ```bash
    sudo journalctl -u hitstor-agent -n 50
    ```

4. Asegúrese de que el nombre del agente coincida con el que se registró con `hitstor serve agent add`.

## Problemas con las alertas { #alert-issues }

### Alertas de agente inactivo { #stale-agent-alerts }

**Causa:** Un agente no ha completado un respaldo dentro del umbral de inactividad configurado (predeterminado: 25 horas).

**Solución:**

1. Verifique que el servicio del agente esté en ejecución:

    ```bash
    sudo systemctl status hitstor-agent
    ```

2. Revise los registros del agente en busca de errores:

    ```bash
    sudo journalctl -u hitstor-agent --since "24 hours ago"
    ```

3. Verifique la conectividad de red entre el agente y el servidor.

4. Si el agente está fuera de línea de forma intencional (mantenimiento, dado de baja), elimínelo:

    ```bash
    hitstor serve agent remove AGENT_NAME
    ```

5. Ajuste el umbral de inactividad si la programación de respaldos es menos frecuente que cada 25 horas:

    ```yaml
    # /etc/hitstor/config.yaml
    alert_stale_threshold: 49  # horas
    ```

## Errores de bloqueo por mantenimiento { #maintenance-lock-errors }

### "repository is locked for maintenance" { #repository-is-locked-for-maintenance }

**Causa:** Se está ejecutando una operación de verificación (check) o de depuración (prune) en el repositorio. Solo se puede ejecutar una operación de mantenimiento a la vez por repositorio.

**Solución:**

1. Espere a que termine la operación en curso. Consulte el progreso de la operación en la página Almacenamiento del dashboard.

2. Si la operación parece atascada, reinicie el servidor. El bloqueo de mantenimiento en memoria se libera cuando el proceso termina.

3. Si necesita ejecutar una operación de mantenimiento con urgencia, use la opción "Force" del dashboard, que aborta las sesiones de respaldo activas y adquiere el bloqueo.

### "acquire lock: ..." { #acquire-lock }

**Causa:** Una condición de carrera en la que otra solicitud de mantenimiento adquirió el bloqueo entre la verificación inicial y el intento de bloqueo.

**Solución:** Vuelva a intentar la operación cuando termine la tarea de mantenimiento en curso.

## Problemas de rendimiento { #performance-issues }

### Respaldos lentos { #slow-backups }

**Causa:** Limitaciones del ancho de banda de red, cuellos de botella de E/S de disco o una configuración de paralelismo poco óptima.

**Solución:**

1. Revise la opción `--parallel`. El valor predeterminado es adecuado para la mayoría de los sistemas, pero ajústelo según los núcleos de CPU disponibles y la velocidad del disco:

    ```bash
    hitstor-agent backup --parallel 4 /data
    ```

2. Supervise el rendimiento de la red durante el respaldo:

    ```bash
    # En el servidor
    iftop -i eth0 -f "port 8420"
    ```

3. Revise la E/S de disco tanto en el agente como en el servidor:

    ```bash
    iostat -x 1
    ```

4. Para respaldos iniciales grandes, considere ejecutar el primer respaldo en la red local en lugar de hacerlo a través de un enlace WAN.

5. Revise las estadísticas de deduplicación en la salida del respaldo. Una tasa de deduplicación baja en respaldos posteriores puede indicar una alta rotación de datos.

## Integridad del repositorio { #repository-integrity }

### Verificar el estado del repositorio { #verifying-repository-health }

Ejecute una verificación estructural:

```bash
hitstor check --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
```

Para una verificación exhaustiva que lee y descifra cada blob:

```bash
hitstor check --repo /var/lib/hitstor/repo --password "REPO_PASSWORD" --read-data
```

El flag `--read-data` aumenta considerablemente la duración de la verificación, pero confirma que todos los datos se pueden descifrar y que coinciden con sus sumas de verificación esperadas.

### Salida de la verificación { #check-output }

Un repositorio en buen estado produce:

```
check: no errors found
```

Si se encuentran errores, la salida enumera los archivos de paquete o las instantáneas (snapshots) afectados. Comuníquese con soporte o consulte la documentación de recuperación del repositorio.

## Análisis de registros { #log-analysis }

HitStor usa el paquete `slog` de Go para generar registros estructurados en JSON. Las entradas de registro incluyen campos para la marca de tiempo, el nivel, el mensaje y datos de contexto.

### Ver los registros { #viewing-logs }

```bash
# Seguir los registros del servidor
sudo journalctl -u hitstor-server -f

# Seguir los registros del agente
sudo journalctl -u hitstor-agent -f
```

### Filtrar con jq { #filtering-with-jq }

Analice y filtre los registros estructurados en JSON:

```bash
# Mostrar solo los errores
sudo journalctl -u hitstor-server -o cat | jq 'select(.level == "ERROR")'

# Mostrar los mensajes relacionados con respaldos
sudo journalctl -u hitstor-server -o cat | jq 'select(.msg | contains("backup"))'

# Mostrar los mensajes de un agente específico
sudo journalctl -u hitstor-server -o cat | jq 'select(.agent == "web-server-01")'

# Contar los errores de las últimas 24 horas
sudo journalctl -u hitstor-server --since "24 hours ago" -o cat | jq 'select(.level == "ERROR")' | wc -l
```

### Campos de registro comunes { #common-log-fields }

| Campo | Descripción |
|-------|-------------|
| `time` | Marca de tiempo ISO 8601 |
| `level` | Nivel de registro (`DEBUG`, `INFO`, `WARN`, `ERROR`) |
| `msg` | Mensaje legible para personas |
| `agent` | Nombre del agente (para eventos relacionados con agentes) |
| `snapshot` | ID de la instantánea (para eventos relacionados con instantáneas) |
| `duration` | Duración de la operación (para operaciones cronometradas) |
| `bytes` | Tamaño de los datos (para operaciones de transferencia) |
| `error` | Mensaje de error (para entradas de nivel error) |

## Obtener ayuda { #getting-help }

Si los pasos de solución de problemas anteriores no resuelven el inconveniente:

1. Recopile los registros del servidor y del agente que abarquen el momento del problema.
2. Anote la versión de HitStor (`hitstor version`).
3. Anote el sistema operativo y su versión.
4. Abra un issue en el repositorio de HitStor con la información recopilada.
