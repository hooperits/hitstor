# hitstor-agent bmr

Crea e inspecciona medios de recuperación bare-metal.

## Sinopsis { #synopsis }

```
hitstor-agent bmr check [flags]
hitstor-agent bmr generate-config [flags]
hitstor-agent bmr build-media [flags]
```

## Descripción { #description }

Tiene el mismo árbol de comandos que [`hitstor bmr`](../commands/bmr.md): ambos binarios comparten la misma implementación, ya que los medios de recuperación solo pueden crearse en el host que protegen, y `hitstor-agent` es lo que realmente se instala en un host protegido.

**Una diferencia**: en `hitstor-agent`, el valor predeterminado del flag `--server` se rellena con la dirección del servidor configurada en el propio agente (a la que normalmente informa sus respaldos), de modo que `generate-config`/`build-media` requieren escribir menos en una máquina que ya es un agente registrado. En `hitstor`, `--server` no tiene valor predeterminado.

Consulte [hitstor bmr](../commands/bmr.md) para la referencia completa de flags, las descripciones de los subcomandos y los ejemplos; se aplican aquí sin cambios.

## Notas { #notes }

- La recuperación en sí la conduce [hitstor-agent recover](recover.md), que se invoca automáticamente desde el medio de rescate.
- Consulte [Restauración bare-metal](../../user-guide/bare-metal-restore.md) para el flujo de trabajo completo.
