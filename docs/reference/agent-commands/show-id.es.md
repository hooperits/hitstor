# hitstor-agent show-id

Muestra el token del agente para registrarlo en el dashboard.

## Sinopsis { #synopsis }

```
hitstor-agent show-id
```

## Descripción { #description }

Lee e imprime el token del agente desde `/etc/hitstor/agent-token`. El token es una cadena hexadecimal de 64 caracteres que se genera durante la instalación del agente o en el primer arranque del daemon.

Use este token al agregar el agente desde el dashboard de HitStor:

1. Abra el dashboard y vaya a **Agentes**
2. Haga clic en **+ Agregar agente**
3. Ingrese la dirección IP del agente, el puerto (predeterminado: 8422) y este token

## Ejemplo { #example }

```bash
sudo hitstor-agent show-id
```

Salida:

```
a3f8c1d9e4b72a5601df83be9c7f4a2d0e6b1893fc5d74a0e2c8f961b3d507ea
```

## Notas { #notes }

- El archivo del token se encuentra en `/etc/hitstor/agent-token` con modo `0600` (solo root puede leerlo)
- Si el archivo del token no existe, inicie primero el daemon del agente (`hitstor-agent daemon`); este genera el token automáticamente
- El servidor usa el token para autenticar las llamadas gRPC de administración (exploración del sistema de archivos, configuración)
