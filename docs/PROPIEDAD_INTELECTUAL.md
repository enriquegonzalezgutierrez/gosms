# Autoría y propiedad intelectual

GoSMS es un proyecto de **Enrique González Gutiérrez**. El copyright sobre el
código es automático desde el momento de su creación; publicarlo en GitHub no
lo transfiere ni lo convierte en dominio público.

## Qué prueba ya existe

- El historial de Git del repositorio, con fechas y autores.
- El commit inicial y la evolución completa del código.
- Las releases y tags del repositorio.
- La licencia AGPL-3.0-or-later y los avisos de copyright.

## Cómo reforzarlo

1. **Firmar commits y tags** con GPG o SSH. GitHub muestra la verificación y
   dificulta la suplantación.
2. **Crear una release firmada** (`v1.0.0`) con binarios y `SHA256SUMS`.
3. **Archivar el repositorio** en [Software Heritage](https://archive.softwareheritage.org/)
   y en [Zenodo](https://zenodo.org/) para obtener un identificador y una fecha
   públicos.
4. **Registrar la obra** en el Registro de la Propiedad Intelectual (España) o
   en un servicio de registro con fecha como SafeCreative. No es obligatorio,
   pero ayuda a presumir autoría.
5. **Registrar la marca “GoSMS”** si el proyecto va a tener recorrido comercial.
   El copyright no protege nombres ni logos.
6. Guardar **documentación original**: bocetos, correos, primeras versiones,
   decisiones de diseño y capturas del historial.

## Disuasión técnica

- Cabeceras SPDX en el código:
  ```go
  // Copyright (c) 2026 Enrique González Gutiérrez
  // SPDX-License-Identifier: AGPL-3.0-or-later
  ```
- `LICENSE`, `NOTICE`, `AUTHORS` y `CITATION.cff` en la raíz.
- Metadatos de build con versión, commit y un identificador único.
- Binarios firmados y checksums publicados en cada release.

## Vigilancia

- Alertas de Google para `"GoSMS"`, tu nombre y cadenas únicas del README.
- Búsquedas periódicas en GitHub de funciones o textos característicos.
- Si alguien incumple AGPL: contacto primero y, si no responde, DMCA ante
  GitHub o asesoramiento legal.

## Contenido de terceros

Las ROMs, los packs gráficos derivados y otros materiales con copyright de
Sega, Disney u otros titulares **no** forman parte del repositorio y no están
cubiertos por la licencia de GoSMS. Mantén `roms/` y `assets/` fuera de Git.

## Contacto

Para asuntos de licencia, autoría o uso comercial, utiliza el repositorio:
https://github.com/enriquegonzalezgutierrez/gosms
