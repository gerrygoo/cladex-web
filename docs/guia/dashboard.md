# Dashboard

El menú **Dashboard** reúne lo que tienes que hacer. **Mis pendientes** es la pantalla
que ves al entrar a la app.

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

Los seguimientos vencidos no son privados: son de tus proyectos, que el equipo también
ve. Un seguimiento se queda con quien tenía el proyecto
cuando empezó a vencer, aunque después se reasigne.

Debajo de tus pendientes sigue la tabla [Vendedores](README.md#la-página-de-inicio).
