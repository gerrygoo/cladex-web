// Keeps glossary tooltips (span.term-tip, shown by CSS on hover/focus) inside the
// viewport: after one opens, shift it left by however much it overflows the right edge.
(function () {
  function place(term) {
    var tip = term.querySelector(".term-tip");
    if (!tip) return;
    tip.style.left = "";
    var overflow = tip.getBoundingClientRect().right - (document.documentElement.clientWidth - 8);
    if (overflow > 0) tip.style.left = -overflow + "px";
  }
  function onEnter(e) {
    var term = e.target.closest && e.target.closest("span.term");
    if (term) place(term);
  }
  document.addEventListener("mouseover", onEnter);
  document.addEventListener("focusin", onEnter);
})();
