# Dashboard

El menú **Dashboard** reúne lo que tienes que hacer. **Mis pendientes** es la pantalla
que ves al entrar a la app.
:::admin
Los administradores también tienen **Equipo**, con el seguimiento de cada quien
([ver abajo](#equipo)).
:::

## Mis pendientes

Una sola lista con dos tipos de pendientes:

- **Seguimientos**: los [[prospecto|prospectos]] tuyos cuyo día de *Próximo seguimiento*
  ya pasó sin que nadie registrara nada en el proyecto. Aparecen solos al empezar el
  siguiente día hábil (sábado y domingo no cuentan), aunque tengas apagados los avisos por
  correo. Cada uno dice **Seguimiento de** y el folio del proyecto (haz clic para abrirlo),
  el cliente, la **Fecha** de seguimiento y el **Atraso** en días hábiles.
- **Pendientes libres**: lo que tú escribes, como *Mandarle al contador los estados de
  cuenta*. Muestran la descripción y la **Fecha** en que lo agregaste.

### Prioridades

Cada pendiente tiene una de cuatro prioridades, con su color:

| Prioridad | Color |
|---|---|
| Baja | azul |
| Media | amarillo |
| Alta | naranja |
| Muy alta | rojo |

Los seguimientos la toman solos según su atraso y no se puede cambiar: **Media** el
primer día hábil, **Alta** el segundo y **Muy alta** del tercero en adelante. La de un
pendiente libre la eliges tú y la cambias cuando quieras en su lista desplegable; se
guarda al elegirla.

La lista va de mayor a menor prioridad. Haz clic en el encabezado **Prioridad** para
verla de menor a mayor, y otra vez para regresar. Dentro de la misma prioridad, el más
antiguo va primero.

### Agregar un pendiente

1. Haz clic en **Agregar pendiente**. Se abre una línea nueva al final de la lista.
2. Escribe la descripción (hasta 500 caracteres). La fecha es la de hoy.
3. Elige la prioridad; si no, queda en **Media**.
4. Haz clic en **Guardar pendiente**.

Si la dejas vacía verás *Escribe qué tienes pendiente.* Puedes tener todos los pendientes
que quieras.

### Completar un pendiente

- **Un pendiente libre** lo completas tú con **Completar**.
- **Un seguimiento** no se completa a mano: se completa solo cuando alguien
  [comenta el proyecto](proyectos.md#comentar-un-proyecto), le pone una
  [nueva fecha de seguimiento](proyectos.md#anotar-el-seguimiento) o el proyecto deja de
  ser prospecto (por ejemplo, porque llegó la orden de compra o se perdió).

### Completados

A un lado (abajo en el celular), en verde, está lo que completaste en los **últimos 30
días**, del más reciente al más antiguo: los seguimientos, con cómo se completaron (*con
un comentario*, *con una nueva fecha* o *por cambio de etapa*), y tus pendientes libres.
Si completaste uno libre por error, haz clic en **Reabrir** y regresa a la lista.

### Quién ve tus pendientes

**Tus pendientes libres solo los ves tú.** Nadie más los ve en la app, ni los
administradores, y no quedan en el historial de cambios. Quien administra el servidor o
sus respaldos sí podría leer la base de datos, como pasa con cualquier sistema.

Los seguimientos vencidos no son privados: son de tus proyectos, y los administradores
los ven en **Equipo**. Un seguimiento se queda con quien tenía el proyecto
cuando empezó a vencer, aunque después se reasigne.

Debajo de tus pendientes sigue la tabla [Vendedores](README.md#la-página-de-inicio).

:::admin
## Equipo

Solo para administradores: **Dashboard** → **Equipo** muestra qué tan al día está cada
quien con el seguimiento de sus proyectos, y cuánto dinero hay en proyectos con el
seguimiento vencido. Usa la misma regla que [Mis pendientes](#mis-pendientes) y que los
avisos: un seguimiento vence si pasa su día sin comentario, nueva fecha ni cambio de
etapa, y su atraso se cuenta en días hábiles.

### Filtros

Arriba eliges y haces clic en **Aplicar**:

- **Periodo**: *Esta semana* (de lunes a domingo), *Este mes* (el que ves al entrar) o
  *Rango de fechas*, con los días **Desde (rango)** y **Hasta (rango)** (al cambiar uno, el
  periodo pasa solo a *Rango de fechas*). Debajo dice qué días abarca.
- **Probabilidad mínima**: por ejemplo *Alta 75% o más*, para ver solo esos proyectos y sus
  seguimientos. *Todas* no filtra.

Para ver a una o varias personas, usa el embudo de la columna **Persona**
([cómo](README.md#filtrar-las-tablas)).

**El periodo aplica al historial**: avisos recibidos, escalados, atraso promedio y
seguimientos a tiempo. **Lo vencido es siempre lo de hoy**: *Vencidos hoy*, los montos y la
lista de proyectos vencidos no dependen del periodo.

### Seguimiento por persona

Una fila por cada persona activa que tiene prospectos o tuvo seguimientos en el periodo,
ordenadas por **Lugar**:

1. **Seguimientos a tiempo**: de los días de seguimiento del periodo que ya llegaron,
   cuántos se atendieron antes de vencer (por ejemplo, *67% (2 de 3)*). Es lo que decide el
   lugar; si dos personas empatan, va primero quien tiene menos atraso promedio.
2. **Proyectos activos**: sus prospectos de hoy.
3. **Vencidos hoy**, **Valor vencido (sin IVA)** y **Utilidad vencida**: sus proyectos con
   el seguimiento vencido hoy y lo que suman, sin IVA, igual que en el pronóstico.
4. **Avisos recibidos**: los avisos de seguimiento vencido que la app le mandó en el
   periodo (uno por proyecto y envío, aunque llegue por correo y por WhatsApp; los
   cancelados no cuentan).
5. **Escalados al equipo**: sus seguimientos que llegaron al segundo día hábil sin
   atenderse a las 12:00, cuando se avisa al equipo.
6. **Atraso promedio**: los días hábiles de atraso, en promedio, de sus seguimientos
   vencidos que se cerraron en el periodo. *—* si no se cerró ninguno.
7. **Último comentario**: la fecha de su comentario más reciente en cualquier proyecto.

La fila **Equipo**, al final, suma todo.

### Proyectos vencidos hoy

Todos los proyectos con el seguimiento vencido hoy, del más atrasado al menos: folio (haz
clic para abrirlo), cliente, persona, probabilidad, día de seguimiento, **Atraso**, valor y
utilidad sin IVA, con el total abajo.

Al final, **Ver el historial de avisos** abre las
[notificaciones](administracion.md#notificaciones) de seguimiento vencido.

Los pendientes libres de cada quien no aparecen aquí ni en ninguna otra pantalla.
:::
