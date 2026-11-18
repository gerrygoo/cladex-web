package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
)

// StagingRule says what building staging does with a table (#81): staging is an empty
// database with production's configuration copied in and synthetic data on top, so no
// customer data reaches it.
type StagingRule int

const (
	// StagingCopy tables are company configuration, copied from production as they are
	// (with the per-table adjustments in stagingCopies).
	StagingCopy StagingRule = iota + 1
	// StagingSynthetic tables hold customer and business data: only what seed-staging
	// makes.
	StagingSynthetic
	// StagingEmpty tables are operational history; they start empty.
	StagingEmpty
	// StagingSkip tables aren't the app's data (the migrations' own bookkeeping).
	StagingSkip
)

// StagingTables classifies every table. TestStagingTablesClassified fails when a
// migration adds a table missing here, so a new table can't put real data on staging
// without someone deciding it should.
var StagingTables = map[string]StagingRule{
	"quote_types":              StagingCopy,
	"product_families":         StagingCopy,
	"products":                 StagingCopy,
	"product_materials":        StagingCopy,
	"materials":                StagingCopy,
	"units":                    StagingCopy,
	"product_unit_conversions": StagingCopy,
	"margin_options":           StagingCopy,
	"settings":                 StagingCopy,
	"users":                    StagingCopy,
	"user_avisos":              StagingCopy,
	"document_sections":        StagingCopy,
	"official_documents":       StagingCopy,

	"customers":             StagingSynthetic,
	"suppliers":             StagingSynthetic,
	"supplier_contacts":     StagingSynthetic,
	"projects":              StagingSynthetic,
	"quotes":                StagingSynthetic,
	"quote_lines":           StagingSynthetic,
	"quote_comments":        StagingSynthetic,
	"pendientes":            StagingSynthetic,
	"purchase_orders":       StagingSynthetic,
	"purchase_order_lines":  StagingSynthetic,
	"supplier_quotes":       StagingSynthetic,
	"supplier_quote_files":  StagingSynthetic,
	"deliveries":            StagingSynthetic,
	"payments":              StagingSynthetic,
	"project_files":         StagingSynthetic,
	"comprobantes":          StagingSynthetic,
	"comprobante_conceptos": StagingSynthetic,
	"comprobante_events":    StagingSynthetic,
	"cfdi_folio_sequences":  StagingSynthetic,
	"leads":                 StagingSynthetic,
	"lead_comments":         StagingSynthetic,

	"sessions":            StagingEmpty,
	"login_attempts":      StagingEmpty,
	"audit_log":           StagingEmpty,
	"audit_actor":         StagingEmpty,
	"jobs":                StagingEmpty,
	"notifications":       StagingEmpty,
	"notification_events": StagingEmpty,
	"aviso_rounds":        StagingEmpty,

	"schema_migrations": StagingSkip,
}

// stagingCopy is how one StagingCopy table is copied: the rows kept (a WHERE on the
// source, "" for all) and column values replaced (SQL expressions over the source row,
// by column name). Columns are copied by name, so ids and references are kept.
type stagingCopy struct {
	table string
	where string
	set   map[string]string
}

// stagingCopies is in foreign-key order. {users}, {uploader} and {factor} are filled in
// by CopyStagingConfig.
var stagingCopies = []stagingCopy{
	// Only the users in STAGING_USERS, with no password (StagingPasswords gives them
	// one) and no sessions; everyone else isn't on staging at all.
	{table: "users", where: "username IN ({users})", set: map[string]string{"password_hash": "'!'"}},
	{table: "user_avisos", where: "user_id IN (SELECT id FROM src.users WHERE username IN ({users}))"},
	{table: "units"},
	// default_supplier_id points at a real supplier; suppliers are synthetic.
	{table: "product_families", set: map[string]string{"default_supplier_id": "NULL"}},
	// Costs are scaled by one random factor per build (#81, decision 1): margins and
	// prices keep their proportions, absolute amounts aren't production's.
	{table: "materials", set: map[string]string{"price_micros": "CAST(round(price_micros * {factor}) AS INTEGER)", "updated_by": "{userref:updated_by}"}},
	{table: "products", set: map[string]string{"cost_micros": "CAST(round(cost_micros * {factor}) AS INTEGER)"}},
	{table: "product_materials"},
	{table: "product_unit_conversions"},
	{table: "quote_types"},
	{table: "margin_options", set: map[string]string{"updated_by": "{userref:updated_by}"}},
	// Staging sets its own emisor and series (staging-refresh.sh); seed_staging_at is
	// the seed's own marker.
	{table: "settings", where: "key NOT LIKE 'emisor\\_%' ESCAPE '\\' AND key NOT IN ('factura_serie', 'pago_serie', 'seed_staging_at')",
		set: map[string]string{"updated_by": "{userref:updated_by}"}},
	{table: "document_sections", set: map[string]string{"deleted_by": "{userref:deleted_by}"}},
	// Each file becomes a one-page placeholder PDF with the same title (#81, decision 2).
	{table: "official_documents", set: map[string]string{
		"data": "?", "size": "?", "content_type": "'application/pdf'",
		"filename":    "CASE WHEN instr(filename, '.') > 0 THEN rtrim(filename, replace(filename, '.', '')) || 'pdf' ELSE filename || '.pdf' END",
		"uploaded_by": "{uploader}", "deleted_by": "{userref:deleted_by}"}},
}

// StagingEmisor is the emisor and CFDI series staging uses: the SAT's test company
// (ESCUELA KEMPER URGATE), whose CSD the Facturoporti sandbox accepts.
var StagingEmisor = map[string]string{
	"emisor_rfc": "EKU9003173C9", "emisor_nombre": "ESCUELA KEMPER URGATE", "emisor_regimen": "601",
	"emisor_cp": "42501", "factura_serie": "PRUEBAF", "pago_serie": "PRUEBAP",
}

// ErrStagingNotEmpty is returned when the database to build staging in already has data.
var ErrStagingNotEmpty = errors.New("store: staging database already has data; build it in a new one")

// CopyStagingConfig copies production's configuration (the StagingCopy tables) from
// the database at srcPath into this one, which must be freshly migrated: the rows the
// migrations put in those tables are replaced. users are the usernames that get a
// staging account (STAGING_USERS); at least one must exist in production, since copied
// documents need an uploader. Costs are scaled by a random factor that isn't reported.
func (s *Store) CopyStagingConfig(ctx context.Context, srcPath string, users []string) error {
	factor, err := stagingCostFactor()
	if err != nil {
		return err
	}
	return s.copyStagingConfig(ctx, srcPath, users, factor)
}

func (s *Store) copyStagingConfig(ctx context.Context, srcPath string, users []string, factor float64) error {
	var names []string
	for _, u := range users {
		if u = strings.TrimSpace(u); u != "" {
			names = append(names, sqlString(u))
		}
	}
	if len(names) == 0 {
		return errors.New("store: no STAGING_USERS to copy")
	}
	// A database the app or the seed has used has users, customers or proyectos; a new
	// one has none (some migrations seed other tables, cleared below).
	for _, table := range []string{"users", "customers", "projects"} {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM "`+table+`"`).Scan(&n); err != nil {
			return fmt.Errorf("store: staging copy: %w", err)
		}
		if n > 0 {
			return ErrStagingNotEmpty
		}
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Foreign keys can only be switched outside a transaction; they are checked as a
	// whole before committing.
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`)
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS src`, srcPath); err != nil {
		return fmt.Errorf("store: staging copy: attach: %w", err)
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `DETACH DATABASE src`)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	userList := strings.Join(names, ", ")
	var uploader int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM src.users WHERE username IN (`+userList+`) ORDER BY id LIMIT 1`).Scan(&uploader); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("store: none of STAGING_USERS exists in production")
		}
		return err
	}
	// Migrations seed some synthetic tables (0033 adds a real supplier and its
	// contacts): staging's synthetic data is only the seed's.
	for table, rule := range StagingTables {
		if rule == StagingSynthetic {
			if _, err := tx.ExecContext(ctx, `DELETE FROM main."`+table+`"`); err != nil {
				return fmt.Errorf("store: staging copy: clear %s: %w", table, err)
			}
		}
	}
	placeholder := placeholderPDF()
	for _, c := range stagingCopies {
		cols, err := sharedColumns(ctx, tx, c.table)
		if err != nil {
			return err
		}
		var exprs []string
		var args []any
		for _, col := range cols {
			e, ok := c.set[col]
			if !ok {
				exprs = append(exprs, `"`+col+`"`)
				continue
			}
			if strings.HasPrefix(e, "{userref:") {
				// A reference to a user who isn't on staging is dropped.
				ref := strings.TrimSuffix(strings.TrimPrefix(e, "{userref:"), "}")
				e = fmt.Sprintf(`CASE WHEN "%s" IN (SELECT id FROM src.users WHERE username IN ({users})) THEN "%s" END`, ref, ref)
			}
			if e == "?" {
				switch col {
				case "data":
					args = append(args, placeholder)
				case "size":
					args = append(args, len(placeholder))
				}
			}
			exprs = append(exprs, e)
		}
		q := fmt.Sprintf(`INSERT INTO main."%s" (%s) SELECT %s FROM src."%s"`, c.table, quoteCols(cols), strings.Join(exprs, ", "), c.table)
		if c.where != "" {
			q += " WHERE " + c.where
		}
		q = strings.NewReplacer("{users}", userList, "{uploader}", fmt.Sprint(uploader), "{factor}", fmt.Sprintf("%.6f", factor)).Replace(q)
		if _, err := tx.ExecContext(ctx, `DELETE FROM main."`+c.table+`"`); err != nil {
			return fmt.Errorf("store: staging copy %s: %w", c.table, err)
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("store: staging copy %s: %w", c.table, err)
		}
	}
	// Staging stamps with the SAT's test CSD, so its emisor is that company, and its CFDIs
	// get their own series so they never look like production's.
	for k, v := range StagingEmisor {
		if _, err := tx.ExecContext(ctx, `INSERT INTO main.settings (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	// The copy writes audit rows through the triggers (audit_log is append-only): they
	// record how staging was built, and are all its history starts with.
	rows, err := tx.QueryContext(ctx, `PRAGMA main.foreign_key_check`)
	if err != nil {
		return err
	}
	var bad []string
	for rows.Next() {
		var table, parent string
		var rowid, fkid sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			rows.Close()
			return err
		}
		bad = append(bad, fmt.Sprintf("%s row %d → %s", table, rowid.Int64, parent))
	}
	rows.Close()
	if len(bad) > 0 {
		return fmt.Errorf("store: staging copy leaves broken references: %s", strings.Join(bad, "; "))
	}
	return tx.Commit()
}

// sharedColumns are the columns a table has in both databases, in this one's order.
func sharedColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	cols := func(schema string) ([]string, error) {
		rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT name FROM pragma_table_info('%s', '%s')`, table, schema))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, rows.Err()
	}
	dst, err := cols("main")
	if err != nil {
		return nil, err
	}
	src, err := cols("src")
	if err != nil {
		return nil, err
	}
	if len(src) == 0 {
		return nil, fmt.Errorf("store: staging copy: production has no table %s", table)
	}
	in := map[string]bool{}
	for _, c := range src {
		in[c] = true
	}
	var out []string
	for _, c := range dst {
		if in[c] {
			out = append(out, c)
		}
	}
	return out, nil
}

func quoteCols(cols []string) string {
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = `"` + c + `"`
	}
	return strings.Join(q, ", ")
}

func sqlString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// stagingCostFactor is between 0.70 and 1.30 but never within 10% of 1, so scaled costs
// are never mistaken for real ones.
func stagingCostFactor() (float64, error) {
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(600_001))
		if err != nil {
			return 0, err
		}
		f := 0.7 + float64(n.Int64())/1_000_000
		if f < 0.9 || f > 1.1 {
			return f, nil
		}
	}
}

// placeholderPDF is a one-page PDF that says it stands in for a document that isn't on
// staging.
func placeholderPDF() []byte {
	text := "Documento de prueba: el archivo original no se copia a staging."
	stream := fmt.Sprintf("BT /F1 14 Tf 72 720 Td (%s) Tj ET", text)
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return []byte(b.String())
}

// StagingPasswordHashes reads the password hashes of the staging database at path (the
// one a refresh replaces), by username, so the rebuilt staging keeps them: people's
// staging passwords survive refreshes. Users without a working password are left out. A
// missing file (the first build) gives none.
func StagingPasswordHashes(ctx context.Context, path string) (map[string]string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT username, password_hash FROM users WHERE disabled_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("store: staging passwords from %s: %w", path, err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var u, h string
		if err := rows.Scan(&u, &h); err != nil {
			return nil, err
		}
		if strings.HasPrefix(h, "$2") {
			out[u] = h
		}
	}
	return out, rows.Err()
}
