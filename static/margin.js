// Quote builder: the custom margin's two fields ("Margen (%)" and the copper price per
// kg) stay in step as either is typed in. The server treats whichever was edited last
// (margin_edited) as the source of truth and derives the other, so this script only
// mirrors that math for display: price = cost / (1 - margin).
(function () {
  var box = document.getElementById("margin-controls");
  if (!box) return;
  var select = document.getElementById("margin-select");
  var custom = document.getElementById("margin-custom");
  var pct = document.getElementById("margin-pct");
  var price = document.getElementById("copper-price");
  var edited = document.getElementById("margin-edited");
  var cost = parseFloat(box.dataset.copperCost);
  var lastOption = select.options[select.selectedIndex];

  function num(el) {
    var v = parseFloat(el.value.replace("$", "").trim());
    return isNaN(v) ? null : v;
  }

  function pctToPrice() {
    var p = num(pct);
    if (!price || isNaN(cost) || p === null || p < 0 || p >= 100) {
      if (price) price.value = "";
      return;
    }
    price.value = (cost / (1 - p / 100)).toFixed(2);
  }

  function priceToPct() {
    var v = num(price);
    if (isNaN(cost) || v === null || v < cost) {
      pct.value = "";
      return;
    }
    pct.value = ((1 - cost / v) * 100).toFixed(4).replace(/\.?0+$/, "");
  }

  pct.addEventListener("input", function () {
    edited.value = "pct";
    pctToPrice();
  });
  if (price) {
    price.addEventListener("input", function () {
      edited.value = "copper";
      priceToPct();
    });
  }

  // Switching to "Personalizado" starts from the option it replaces, so the fields
  // show that option's margin instead of an empty (invalid) one.
  select.addEventListener("change", function () {
    var opt = select.options[select.selectedIndex];
    var isCustom = opt.value === "custom";
    custom.hidden = !isCustom;
    if (isCustom && pct.value === "" && lastOption && lastOption.dataset.pct) {
      pct.value = lastOption.dataset.pct;
      edited.value = "pct";
      pctToPrice();
    }
    if (!isCustom) lastOption = opt;
  }, true);
})();
