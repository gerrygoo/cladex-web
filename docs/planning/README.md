# Planning

Forward-looking scoping for Cladex: where the tool is going, not what it does today.
`docs/PLAN.md` is the as-built spec and session-sized slice log; this directory is the
scratch space that precedes it — sketches, state diagrams, and open questions for
features not yet designed into a slice. Something here graduates into `docs/PLAN.md`
(or a GitHub issue) once it's actually scheduled; until then it's a proposal, not
documentation of shipped behavior.

## Conventions

- **Diagrams are Mermaid**, in fenced ` ```mermaid ` blocks inside plain markdown.
  GitHub renders them natively, they're plain text (diffable, editable without a
  drawing tool), and they're the same format `docs/guia/` already leans on for
  structure. `stateDiagram-v2` is the default kind, since most of what we're modeling
  here is a quote or project moving between named states.
- **Every diagram doc says what's shipped vs. proposed.** These started as whiteboard
  photos of the *next* state machine, layered on top of one that's partly already
  built (see `internal/store/pipeline.go`, `docs/guia/glosario.md`'s "Estados"
  section). A diagram that silently mixes current and proposed states is misleading,
  so each doc calls out the delta explicitly.
- **Source sketches live in `sketches/`**, named `YYYY-MM-DD-topic.png`. Keep the
  original alongside its translation — the Mermaid version is an interpretation, and
  disagreements about what a hand-drawn arrow meant get resolved by looking at the
  source, not by trusting the transcription.
- Spanish terms are kept as the actual state/label names (they're product
  vocabulary, same as `docs/PLAN.md`'s "borrador"/"emitida"); surrounding prose is
  English, matching the rest of `docs/`.

## Index

| Doc | Covers |
|---|---|
| [Ciclo de vida: cotización → proyecto → cobro](ciclo-de-vida-cotizacion-a-cobro.md) | The 2026-10-05 whiteboard: quote states, post-issue project pipeline, and a proposed payment sub-status (PPD/PUE) |
| [Facturas: timbrado digital y envío](facturas-timbrado-y-envio.md) | CFDI stamping through an external PAC and delivery to recipients: states before/after each step, cancellation, tracking model, dependencies |
