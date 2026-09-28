// Timestamps (<time class="ts" datetime="…Z">) arrive rendered in UTC; show them in the
// browser's own time zone as 28/09/2026 - 16:42. Runs again for anything htmx swaps in.
(function () {
  function pad(n) {
    return String(n).padStart(2, "0");
  }
  function localize(root) {
    root.querySelectorAll("time.ts[datetime]").forEach(function (el) {
      var d = new Date(el.getAttribute("datetime"));
      if (isNaN(d)) return;
      el.textContent =
        pad(d.getDate()) + "/" + pad(d.getMonth() + 1) + "/" + d.getFullYear() +
        " - " + pad(d.getHours()) + ":" + pad(d.getMinutes());
    });
  }
  // Tell the server our zone so the tables' Desde/Hasta date filters use our days.
  try {
    var zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    if (zone) document.cookie = "tz=" + zone + "; path=/; max-age=31536000; samesite=lax";
  } catch (e) {}
  localize(document);
  document.addEventListener("htmx:load", function (e) {
    localize(e.target);
  });
})();
