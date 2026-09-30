// Behaviour every page shares. Each part only enhances: without JS the forms still post.
(function () {
  // Confirmation before a destructive or final action: a submit button with
  // data-confirm="…" asks first, and the form isn't sent if the user cancels.
  document.addEventListener("submit", function (e) {
    var btn = e.submitter;
    var msg = btn && btn.dataset.confirm;
    if (msg && !window.confirm(msg)) e.preventDefault();
  }, true);

  // Mark the nav link of the section this page belongs to (/cotizaciones/QS0001 is under
  // Cotizaciones).
  document.addEventListener("DOMContentLoaded", function () {
    var section = "/" + location.pathname.split("/")[1];
    document.querySelectorAll("nav.app-nav a, div.nav-user a").forEach(function (a) {
      if (a.getAttribute("href") === section) a.setAttribute("aria-current", "page");
    });
  });

  // The "Base de datos" menu is a <details>: it closes on Escape (focus back on its
  // summary) or on a click anywhere outside it, like any other menu.
  document.addEventListener("keydown", function (e) {
    if (e.key !== "Escape") return;
    var open = document.querySelector("details.nav-menu[open]");
    if (!open) return;
    open.open = false;
    open.querySelector("summary").focus();
  });
  document.addEventListener("click", function (e) {
    document.querySelectorAll("details.nav-menu[open]").forEach(function (d) {
      if (!d.contains(e.target)) d.open = false;
    });
  });

  // After a failed submit the server re-renders the form with aria-invalid fields;
  // start the user on the first one.
  document.addEventListener("DOMContentLoaded", function () {
    var first = document.querySelector('[aria-invalid="true"]');
    // A radio group is marked on its fieldset; focus its first radio.
    if (first && first.matches("fieldset")) first = first.querySelector("input");
    if (first) first.focus();
  });

  // htmx swaps. The swapped-in fragment can carry data-announce="…", which is read out
  // through the page's live region (#live-status). If the swap removed the element
  // that had focus (e.g. a deleted line's button), focus goes to the fragment itself
  // (tabindex="-1") instead of falling back to the top of the page. htmx already
  // restores focus to an element whose id survives the swap.
  // Keep the control the user just used under the cursor. Adding a line from the product
  // picker grows the lines table above it; without this the next product would slide
  // out from under the pointer. Whatever moved is compensated by scrolling the page.
  var anchor = null;
  document.addEventListener("htmx:beforeRequest", function (e) {
    var elt = e.detail.elt;
    anchor = elt ? { elt: elt, top: elt.getBoundingClientRect().top } : null;
  });
  document.addEventListener("htmx:afterSettle", function () {
    if (!anchor || !anchor.elt.isConnected) return;
    var moved = anchor.elt.getBoundingClientRect().top - anchor.top;
    if (Math.abs(moved) > 1) window.scrollBy(0, moved);
    anchor = null;
  });

  var hadFocus = false;
  document.addEventListener("htmx:beforeSwap", function (e) {
    var t = e.detail.target;
    hadFocus = !!(t && t.contains(document.activeElement));
  });
  document.addEventListener("htmx:afterSettle", function (e) {
    var t = e.detail.target;
    var el = t && t.id ? document.getElementById(t.id) : null;
    if (!el) return;
    if (hadFocus && (document.activeElement === document.body || !document.activeElement)) {
      el.focus();
    }
    hadFocus = false;
    var src = el.matches("[data-announce]") ? el : el.querySelector("[data-announce]");
    var live = document.getElementById("live-status");
    if (src && live) {
      // Clear first so the same text (e.g. an unchanged total) is announced again.
      live.textContent = "";
      setTimeout(function () { live.textContent = src.dataset.announce; }, 100);
    }
  });

  // Unsaved-changes guard for forms that keep edits in the page until an explicit save
  // (the quote builder). form[data-guard-unsaved] becomes dirty on any htmx POST from
  // inside it or on typing in a field, except fields marked data-guard-ignore (search
  // boxes). A submit through a button marked data-guard-save clears it. While dirty,
  // leaving the page asks first. [data-unsaved-status] always shows the current state,
  // swapping between its data-saved-text and data-unsaved-text, so nothing appears or
  // disappears and shifts the layout.
  document.addEventListener("DOMContentLoaded", function () {
    var form = document.querySelector("form[data-guard-unsaved]");
    if (!form) return;
    var dirty = false;
    var status = form.querySelector("[data-unsaved-status]");
    function show(state) {
      if (!status) return;
      status.textContent = status.dataset[state + "Text"];
      status.classList.remove("saved", "unsaved", "busy");
      status.classList.add(state);
    }
    function setDirty(v) {
      dirty = v;
      show(v ? "unsaved" : "saved");
    }
    // While a recalculation is in flight the same element says so.
    document.addEventListener("htmx:beforeRequest", function (e) {
      var cfg = e.detail.requestConfig;
      if (cfg && cfg.verb === "post" && form.contains(e.detail.elt)) show("busy");
    });
    form.addEventListener("input", function (e) {
      if (!e.target.closest("[data-guard-ignore]")) setDirty(true);
    });
    document.addEventListener("htmx:afterRequest", function (e) {
      var cfg = e.detail.requestConfig;
      if (!cfg || cfg.verb !== "post" || !form.contains(e.detail.elt)) return;
      setDirty(e.detail.successful ? true : dirty);
    });
    form.addEventListener("submit", function (e) {
      if (e.defaultPrevented) return;
      if (e.submitter && e.submitter.hasAttribute("data-guard-save")) setDirty(false);
    });
    window.addEventListener("beforeunload", function (e) {
      if (!dirty) return;
      e.preventDefault();
      e.returnValue = "";
    });
  });
})();
