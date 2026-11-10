-- Alta masiva de la familia Astro (issue #68), from the supplier's list
-- Lista_precios_Astro_ERP.xlsx: 80 products.
--
--   sku          "AST-" plus the supplier's code
--   unit         Caja: Astro sells whole boxes only; each description says how many
--                piezas the box holds
--   cost_micros  per box, with the 5% supplier discount already applied. AST-C09's list
--                price is already for its box of 1000 (428.82); AST-B01's "100 juegos"
--                is a box of 100 piezas.
--
-- Weight and SAT codes stay NULL, like any product nobody has confirmed yet (0032): the
-- product list's "Sin datos para facturar" filter finds them.
--
-- Production's catalog: the products load into its existing familia, found by name in
-- any case (0033 knows it as "ASTRO"). A database without that familia (a
-- fresh one, a test's) is left as it is, like 0033's familias that don't exist here.
--
-- Only adds rows, so it isn't exclusive. Nothing that exists is changed: the unit is
-- reused if there is one coded or named "caja", and a SKU already taken is left as it
-- is. The rows fire audit triggers; they are logged as unattributed, so the Historial
-- says "Sistema".
UPDATE audit_actor SET user_id = NULL, source = NULL WHERE id = 1;

-- XBX is the SAT's c_ClaveUnidad for "Caja"; only a unit created here gets it.
INSERT INTO units (code, name, sat_code)
SELECT 'caja', 'Caja', 'XBX'
WHERE EXISTS (SELECT 1 FROM product_families
              WHERE name = 'Astro' COLLATE NOCASE AND free_lines_only = 0)
  AND NOT EXISTS (SELECT 1 FROM units
                  WHERE code = 'caja' COLLATE NOCASE OR name = 'Caja' COLLATE NOCASE);

WITH astro (sku, description, cost_micros) AS (VALUES
    ('AST-A01B', 'Apagador Sencillo astro Blanco. Caja con 100 piezas.', 453150000),
    ('AST-A01M', 'Apagador Sencillo astro Marfil. Caja con 100 piezas.', 453150000),
    ('AST-A02B', 'Apagador Escalera astro Blanco. Caja con 100 piezas.', 613700000),
    ('AST-A02M', 'Apagador Escalera astro Marfil. Caja con 100 piezas.', 613700000),
    ('AST-BTB', 'Botón Timbre astro Blanco. Caja con 100 piezas.', 436050000),
    ('AST-BTM', 'Botón Timbre astro Marfil. Caja con 100 piezas.', 436050000),
    ('AST-IPT', 'Interruptor un Polo un Tiro. Caja con 100 piezas.', 2303750000),
    ('AST-C01B', 'Contacto Sencillo astro Blanco. Caja con 100 piezas.', 690650000),
    ('AST-C01M', 'Contacto Sencillo astro Marfil. Caja con 100 piezas.', 690650000),
    ('AST-C06B', 'Contacto Dúplex 270 astro Blanco (Oculto). Caja con 100 piezas.', 992750000),
    ('AST-C06M', 'Contacto Dúplex 270 astro Marfil (Oculto). Caja con 100 piezas.', 992750000),
    ('AST-C08B', 'Contacto Dúplex 78 astro Blanco (Visible). Caja con 50 piezas.', 623200000),
    ('AST-C08M', 'Contacto Dúplex 78 astro Marfil (Visible). Caja con 50 piezas.', 623200000),
    ('AST-C04B', 'Contacto Triple con Clavija astro Blanco. Caja con 100 piezas.', 575700000),
    ('AST-C04M', 'Contacto Triple con Clavija astro Marfil. Caja con 100 piezas.', 575700000),
    ('AST-C05B', 'Contacto Triple sin Clavija astro Blanco. Caja con 100 piezas.', 498750000),
    ('AST-C05M', 'Contacto Triple sin Clavija astro Marfil. Caja con 100 piezas.', 498750000),
    ('AST-C09', 'Protector de Contacto. Caja con 1000 piezas.', 428820000),
    ('AST-PA01', 'Placa de Aluminio Línea 3000 Oro 1U. Caja con 100 piezas.', 1087750000),
    ('AST-PA02', 'Placa de Aluminio Línea 3000 Oro 2U. Caja con 100 piezas.', 1087750000),
    ('AST-PA03', 'Placa de Aluminio Línea 3000 Oro 3U. Caja con 100 piezas.', 1087750000),
    ('AST-PA06', 'Placa de Aluminio Línea 3000 Oro Piloto S/CH. Caja con 100 piezas.', 857850000),
    ('AST-PA07', 'Placa de Aluminio Línea 3000 Ciega Oro S/CH. Caja con 100 piezas.', 857850000),
    ('AST-PA08', 'Placa de Aluminio Industrial Oro. Caja con 100 piezas.', 1024100000),
    ('AST-PA09', 'Placa de Aluminio Dúplex Oro. Caja con 100 piezas.', 857850000),
    ('AST-ABS1B', 'Placa Línea 2000 1U Blanca. Caja con 100 piezas.', 739100000),
    ('AST-ABS1M', 'Placa Línea 2000 1U Marfil. Caja con 100 piezas.', 739100000),
    ('AST-ABS2B', 'Placa Línea 2000 2U Blanca. Caja con 100 piezas.', 739100000),
    ('AST-ABS2M', 'Placa Línea 2000 2U Marfil. Caja con 100 piezas.', 739100000),
    ('AST-ABS3B', 'Placa Línea 2000 3U Blanca. Caja con 100 piezas.', 739100000),
    ('AST-ABS3M', 'Placa Línea 2000 3U Marfil. Caja con 100 piezas.', 739100000),
    ('AST-ABSPB', 'Placa Línea 2000 Piloto/Ciega Blanca. Caja con 100 piezas.', 396150000),
    ('AST-ABSPM', 'Placa Línea 2000 Piloto/Ciega Marfil. Caja con 100 piezas.', 396150000),
    ('AST-ABSDXB', 'Placa Línea 2000 Dúplex Blanca. Caja con 100 piezas.', 393300000),
    ('AST-ABSDXM', 'Placa Línea 2000 Dúplex Marfil. Caja con 100 piezas.', 393300000),
    ('AST-N01', 'Placa Dúplex Nylon Naranja Sin Leyenda. Caja con 100 piezas.', 468350000),
    ('AST-N02', 'Placa Dúplex Naranja Isolated Ground. Caja con 100 piezas.', 662150000),
    ('AST-PAI-DX', 'Placa de Acero Inoxidable Dúplex. Caja con 100 piezas.', 1139050000),
    ('AST-PAI-C', 'Placa de Acero Inoxidable Ciega. Caja con 100 piezas.', 1139050000),
    ('AST-PDX+C', 'Placa Armada de Acero Inoxidable con Contacto Dúplex. Caja con 100 piezas.', 2114700000),
    ('AST-B01', 'Base Astro Line. Caja con 100 piezas.', 1018400000),
    ('AST-B02', 'Base Fluorescente astro. Caja con 100 piezas.', 709650000),
    ('AST-CH-T8', 'Conector Chupón T8 para Lámpara Fluorescente y Led. Caja con 100 piezas.', 457900000),
    ('AST-CMC', 'Caja Múltiple para Canaleta. Caja con 50 piezas.', 404230000),
    ('AST-CMC-ES', 'Caja Múltiple para Canaleta Línea Económica Abierta. Caja con 50 piezas.', 209470000),
    ('AST-CMC-EC', 'Caja Múltiple para Canaleta Línea Económica Cerrada. Caja con 50 piezas.', 229900000),
    ('AST-SSN', 'Socket Sencillo astro Negro. Caja con 100 piezas.', 546250000),
    ('AST-SSB', 'Socket Sencillo astro Blanco. Caja con 100 piezas.', 546250000),
    ('AST-SSC', 'Socket Sencillo astro Café. Caja con 100 piezas.', 546250000),
    ('AST-SSM', 'Socket Sencillo astro Marfil. Caja con 100 piezas.', 546250000),
    ('AST-SRN', 'Socket C/Rosca astro Negro. Caja con 100 piezas.', 581400000),
    ('AST-SRM', 'Socket C/Rosca astro Marfil. Caja con 100 piezas.', 581400000),
    ('AST-SRB', 'Socket C/Rosca astro Blanco. Caja con 100 piezas.', 581400000),
    ('AST-SL', 'Socket Ladrón astro. Caja con 100 piezas.', 700150000),
    ('AST-SL+C', 'Socket Ladrón Con Cadena astro. Caja con 100 piezas.', 906300000),
    ('AST-SO04N', 'Socket Intemperie astro Negro. Caja con 100 piezas.', 1096300000),
    ('AST-SR3.5', 'Socket Roseta astro 3.5". Caja con 50 piezas.', 630800000),
    ('AST-SR4', 'Socket Roseta astro 4". Caja con 50 piezas.', 703000000),
    ('AST-BSQ', 'Block Socket. Caja con 100 piezas.', 1062100000),
    ('AST-CV01', 'Clavija Ovalada astro. Caja con 100 piezas.', 564300000),
    ('AST-CBS', 'Clavija Blindada Sencilla astro. Caja con 100 piezas.', 968050000),
    ('AST-CV07N', 'Clavija Convertidora Naranja. Caja con 100 piezas.', 657400000),
    ('AST-C-H01', 'Clavija Hembra Ovalada. Caja con 100 piezas.', 880650000),
    ('AST-CR01', 'Clavija Redonda Reforzada. Caja con 100 piezas.', 863550000),
    ('AST-AEN', 'Aislador Esquinero astro Negro. Caja con 200 piezas.', 763800000),
    ('AST-AEB', 'Aislador Esquinero astro Blanco. Caja con 200 piezas.', 763800000),
    ('AST-AEN-O', 'Aislador Esquinero astro Naranja. Caja con 200 piezas.', 763800000),
    ('AST-APN', 'Aislador Poste I astro Negro. Caja con 300 piezas.', 1145700000),
    ('AST-APB', 'Aislador Poste I astro Blanco. Caja con 300 piezas.', 1145700000),
    ('AST-APN-O', 'Aislador Poste I astro Naranja. Caja con 300 piezas.', 1145700000),
    ('AST-AP2N', 'Aislador Poste II astro Negro. Caja con 200 piezas.', 763800000),
    ('AST-AP2B', 'Aislador Poste II astro Blanco. Caja con 200 piezas.', 763800000),
    ('AST-AVB', 'Aislador Varilla Blanco astro. Caja con 300 piezas.', 1145700000),
    ('AST-ANV', 'Aislador Varilla Naranja astro. Caja con 300 piezas.', 1145700000),
    ('AST-ANN-O', 'Aislador Varilla Negro astro. Caja con 300 piezas.', 1145700000),
    ('AST-AG', 'Aislador Grapa astro. Caja con 1000 piezas.', 2175500000),
    ('AST-MC', 'Aislador Manija astro. Caja con 40 piezas.', 1018780000),
    ('AST-CHN', 'Chalupa 2"x4" Polipropileno astro Negro. Caja con 100 piezas.', 397100000),
    ('AST-ZB1', 'Zumbador Básico. Caja con 50 piezas.', 1209820000),
    ('AST-BF18', 'Base para Fotocelda. Caja con 100 piezas.', 2752150000)
)
INSERT INTO products (family_id, sku, description, cost_micros, unit_id)
SELECT f.id, a.sku, a.description, a.cost_micros,
       (SELECT min(id) FROM units WHERE code = 'caja' COLLATE NOCASE OR name = 'Caja' COLLATE NOCASE)
FROM astro a
JOIN (SELECT min(id) AS id FROM product_families
      WHERE name = 'Astro' COLLATE NOCASE AND free_lines_only = 0) f ON f.id IS NOT NULL
WHERE true
ON CONFLICT (sku) DO NOTHING;
