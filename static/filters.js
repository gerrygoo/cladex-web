// Column-filter popovers (details.col-filter): only one open at a time, and they close
// on Escape or a click anywhere outside them.
(function () {
  function closeOthers(except) {
    document.querySelectorAll("details.col-filter[open]").forEach(function (d) {
      if (d !== except) d.removeAttribute("open");
    });
  }
  document.addEventListener("toggle", function (e) {
    if (e.target.matches && e.target.matches("details.col-filter") && e.target.open) closeOthers(e.target);
  }, true);
  document.addEventListener("click", function (e) {
    if (!e.target.closest("details.col-filter")) closeOthers(null);
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") closeOthers(null);
  });
})();
