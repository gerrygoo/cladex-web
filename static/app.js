// Behaviour every page shares. Each part only enhances: without JS the forms still post.
(function () {
  // Confirmation before a destructive or final action: a submit button with
  // data-confirm="…" asks first, and the form isn't sent if the user cancels.
  document.addEventListener("submit", function (e) {
    var btn = e.submitter;
    var msg = btn && btn.dataset.confirm;
    if (msg && !window.confirm(msg)) e.preventDefault();
  }, true);

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
  // [data-unsaved-status] inside the form is shown and leaving the page asks first.
  document.addEventListener("DOMContentLoaded", function () {
    var form = document.querySelector("form[data-guard-unsaved]");
    if (!form) return;
    var dirty = false;
    var status = form.querySelector("[data-unsaved-status]");
    function setDirty(v) {
      dirty = v;
      if (status) status.hidden = !v;
    }
    form.addEventListener("input", function (e) {
      if (!e.target.closest("[data-guard-ignore]")) setDirty(true);
    });
    document.addEventListener("htmx:afterRequest", function (e) {
      var cfg = e.detail.requestConfig;
      if (e.detail.successful && cfg && cfg.verb === "post" && form.contains(e.detail.elt)) setDirty(true);
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
