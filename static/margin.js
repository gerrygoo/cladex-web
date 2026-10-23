// Quote builder: the custom margin's two fields ("Margen (%)" and the price per kg —
// CCS 30% on QS, the products' own cost per kg on QG) stay in step as either is typed
// in. The server treats whichever was edited last (margin_edited) as the source of truth
// and derives the other, so this script only mirrors that math for display: price =
// cost / (1 - margin). margin_edited stays empty until a field is typed in; the server
// then keeps the saved margin.
//
// The controls can be swapped out by a recalculation (a QG quote whose lines changed
// the cost per kg, issue #43), so the listeners sit on the document and look the
// fields up each time.
(function () {
  function el(id) {
    return document.getElementById(id);
  }

  function num(input) {
    var v = parseFloat(input.value.replace("$", "").trim());
    return isNaN(v) ? null : v;
  }

  function cost() {
    var box = el("margin-controls");
    return box ? parseFloat(box.dataset.copperCost) : NaN;
  }

  function pctToPrice() {
    var pct = el("margin-pct");
    var price = el("copper-price");
    if (!price) return;
    var c = cost();
    var p = num(pct);
    if (isNaN(c) || p === null || p < 0 || p >= 100) {
      price.value = "";
      return;
    }
    price.value = (c / (1 - p / 100)).toFixed(2);
  }

  function priceToPct() {
    var pct = el("margin-pct");
    var c = cost();
    var v = num(el("copper-price"));
    if (isNaN(c) || v === null || v <= c) {
      pct.value = "";
      return;
    }
    // Shown to two decimals; the server keeps the full margin of the typed price.
    pct.value = ((1 - c / v) * 100).toFixed(2).replace(/\.?0+$/, "");
  }

  document.addEventListener("input", function (e) {
    var edited = el("margin-edited");
    if (!edited) return; // a QL quote: no custom margin
    if (e.target.id === "margin-pct") {
      edited.value = "pct";
      pctToPrice();
    } else if (e.target.id === "copper-price") {
      edited.value = "copper";
      priceToPct();
    }
  });

  // Both fields always show the selected margin. A menu option fills them in read-only;
  // "Otro margen" keeps what's there and unlocks them.
  document.addEventListener("change", function (e) {
    var select = e.target;
    if (select.id !== "margin-select") return;
    var pct = el("margin-pct");
    if (!pct) return; // a QL quote: no custom margin
    var price = el("copper-price");
    var opt = select.options[select.selectedIndex];
    var isCustom = opt.value === "custom";
    pct.readOnly = !isCustom;
    if (price) price.readOnly = !isCustom;
    // Without the price per kg, the percentage only shows while "Otro margen" is picked.
    if (!price) el("margin-custom").hidden = !isCustom;
    if (isCustom) {
      el("margin-edited").value = "pct";
    } else {
      pct.value = opt.dataset.pct || "";
      pctToPrice();
    }
  }, true);
})();
