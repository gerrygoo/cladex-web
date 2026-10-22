// Behaviour every page shares. Each part only enhances: without JS the forms still post.
(function () {
  // Confirmation before a destructive or final action: a submit button with
  // data-confirm="…" asks first, and the form isn't sent if the user cancels.
  document.addEventListener("submit", function (e) {
    var btn = e.submitter;
    var msg = btn && btn.dataset.confirm;
    if (msg && !window.confirm(msg)) e.preventDefault();
  }, true);

  // A submit button with data-confirm-dialog="id" opens that <dialog> instead of sending
  // the form; the dialog's own submit button sends it, and a [data-close-dialog] button
  // (or Escape) closes it, leaving what was typed. Like New producto (issue #41).
  document.addEventListener("submit", function (e) {
    var btn = e.submitter;
    var dialog = btn && btn.dataset.confirmDialog && document.getElementById(btn.dataset.confirmDialog);
    if (!dialog || !dialog.showModal) return;
    e.preventDefault();
    dialog.showModal();
  }, true);
  document.addEventListener("click", function (e) {
    var btn = e.target.closest && e.target.closest("[data-close-dialog]");
    if (btn) btn.closest("dialog").close();
  });

  // Nueva cotización's customer list (issue #29) becomes a combobox: a search field in
  // place of the list, whose matches (by name, RFC or contact, ignoring case and
  // accents) show under it as you type, with "Agregar nuevo cliente" always first. The
  // <select> stays, hidden, and carries the choice; without JS it is the plain list.
  // Keys: arrows move, Enter picks, Escape closes.
  var fold = function (t) {
    return (t || "").normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();
  };
  document.querySelectorAll("select[data-combobox]").forEach(function (select) {
    var id = select.id;
    var wrap = document.createElement("div");
    var input = document.createElement("input");
    var list = document.createElement("ul");
    var status = document.createElement("span");
    wrap.className = "combobox";
    input.type = "text";
    input.id = id + "-buscar";
    input.autocomplete = "off";
    input.placeholder = "Escribe para buscar por nombre, RFC o contacto";
    input.setAttribute("role", "combobox");
    input.setAttribute("aria-autocomplete", "list");
    input.setAttribute("aria-expanded", "false");
    input.setAttribute("aria-controls", id + "-opciones");
    ["aria-invalid", "aria-describedby"].forEach(function (a) {
      if (select.hasAttribute(a)) input.setAttribute(a, select.getAttribute(a));
    });
    list.id = id + "-opciones";
    list.setAttribute("role", "listbox");
    list.hidden = true;
    status.className = "visually-hidden";
    status.setAttribute("role", "status");
    wrap.append(input, list, status);
    select.after(wrap);
    select.hidden = true;
    select.required = false;
    var label = document.querySelector('label[for="' + id + '"]');
    if (label) label.htmlFor = input.id;
    list.setAttribute("aria-label", label ? label.textContent.trim() : "Opciones");

    var choices = Array.prototype.filter.call(select.options, function (o) { return o.value; });
    var picked = function () { return select.selectedOptions[0]; };
    var shown = [];
    var active = -1;
    var nameOf = function (o) { return o.value === "nuevo" ? "" : o.textContent.trim(); };
    input.value = picked() && picked().value ? nameOf(picked()) : "";

    var setActive = function (i) {
      var items = list.querySelectorAll("[role=option]");
      if (active >= 0 && items[active]) items[active].setAttribute("aria-selected", "false");
      active = i;
      if (i < 0 || !items[i]) { input.removeAttribute("aria-activedescendant"); return; }
      items[i].setAttribute("aria-selected", "true");
      input.setAttribute("aria-activedescendant", items[i].id);
      items[i].scrollIntoView({ block: "nearest" });
    };
    var close = function () {
      list.hidden = true;
      input.setAttribute("aria-expanded", "false");
      setActive(-1);
    };
    var open = function (q) {
      q = fold(q.trim());
      shown = choices.filter(function (o) {
        return o.value === "nuevo" || !q || fold(o.textContent + " " + o.dataset.search).indexOf(q) !== -1;
      });
      list.replaceChildren();
      active = -1;
      shown.forEach(function (o, i) {
        var li = document.createElement("li");
        li.id = id + "-opcion-" + i;
        li.setAttribute("role", "option");
        li.setAttribute("aria-selected", "false");
        li.textContent = o.textContent.trim();
        if (o.dataset.detail) {
          var small = document.createElement("small");
          small.textContent = o.dataset.detail;
          li.append(small);
        }
        li.addEventListener("mousedown", function (e) { e.preventDefault(); });
        li.addEventListener("click", function () { choose(o); });
        list.append(li);
      });
      var found = shown.length - 1;
      if (found === 0 && q) {
        var none = document.createElement("li");
        none.className = "combobox-empty";
        none.textContent = "Ningún cliente coincide.";
        list.append(none);
      }
      status.textContent = found === 1 ? "1 cliente" : found + " clientes";
      list.hidden = false;
      input.setAttribute("aria-expanded", "true");
      // Typing puts the first matching customer under Enter.
      if (q && found > 0) setActive(1);
    };
    var choose = function (o) {
      if (o.value === "nuevo") {
        var serie = select.form.querySelector("[name=prefix]:checked");
        location.href = "/clientes/nuevo?volver=cotizacion" + (serie ? "&serie=" + encodeURIComponent(serie.value) : "");
        return;
      }
      select.value = o.value;
      input.value = nameOf(o);
      input.setCustomValidity("");
      close();
    };

    input.addEventListener("input", function () {
      select.value = "";
      input.setCustomValidity("");
      open(input.value);
    });
    input.addEventListener("focus", function () { open(select.value ? "" : input.value); });
    input.addEventListener("click", function () { if (list.hidden) open(select.value ? "" : input.value); });
    input.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        if (list.hidden) { open(select.value ? "" : input.value); return; }
        var n = shown.length;
        if (!n) return;
        setActive(e.key === "ArrowDown" ? (active + 1) % n : (active <= 0 ? n - 1 : active - 1));
      } else if (e.key === "Enter" && !list.hidden) {
        e.preventDefault();
        if (active >= 0) choose(shown[active]);
      } else if (e.key === "Escape" && !list.hidden) {
        e.preventDefault();
        close();
      }
    });
    // Leaving the field without picking puts back the customer chosen before, if any.
    input.addEventListener("blur", function () {
      close();
      input.value = select.value ? nameOf(picked()) : input.value;
    });
    select.form.addEventListener("submit", function (e) {
      if (select.value) return;
      e.preventDefault();
      input.setCustomValidity("Elige un cliente de la lista.");
      input.reportValidity();
    });
  });

  // "Regresar a editar" on a warning: closes it and goes back to the form's first field.
  // Without JS it is a plain link to that field, and the warning stays.
  document.addEventListener("click", function (e) {
    var link = e.target.closest && e.target.closest("[data-dismiss]");
    if (!link) return;
    var box = document.getElementById(link.dataset.dismiss);
    if (box) box.hidden = true;
  });

  // The follow-up form's "Perdida" radio swaps the dates and comment for the Motivo of
  // the loss, and O.C. esperada only shows with Inminente 90%. The server renders the
  // fields as they are for the saved probabilidad, and checks which ones apply.
  document.addEventListener("change", function (e) {
    var form = e.target.closest && e.target.closest("form[data-followup]");
    if (!form || e.target.name !== "probabilidad") return;
    var lost = e.target.value === "perdida";
    form.querySelector("[data-imminent-only]").hidden = e.target.value !== "90";
    var reason = form.querySelector("[data-lost-only]");
    var rest = form.querySelector("[data-not-lost]");
    reason.hidden = !lost;
    rest.hidden = lost;
    reason.querySelector("textarea").required = lost;
    rest.querySelectorAll("[name=proximo_seguimiento], [name=note]").forEach(function (el) {
      el.required = !lost;
    });
  });

  // Mark the nav link of the page, or else of the section it belongs to
  // (/cotizaciones/nueva is Nueva cotización; /cotizaciones/QS0001 is under Ver
  // cotizaciones).
  document.addEventListener("DOMContentLoaded", function () {
    var links = document.querySelectorAll("nav.app-nav a, div.nav-user a");
    var section = "/" + location.pathname.split("/")[1];
    var match = function (path) {
      return Array.prototype.find.call(links, function (a) { return a.getAttribute("href") === path; });
    };
    var current = match(location.pathname) || match(section);
    if (current) current.setAttribute("aria-current", "page");
  });

  // The nav menus are <details>: it closes on Escape (focus back on its
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

  // Pronóstico's Último comentario shows two lines and scrolls inside its cell (issue
  // #42). One that overflows becomes a tab stop named after its proyecto, so it also
  // scrolls from the keyboard, and is marked "more" (a fade) until read to the end.
  document.addEventListener("DOMContentLoaded", function () {
    document.querySelectorAll("table.forecast-table .comment-body").forEach(function (el) {
      if (el.scrollHeight <= el.clientHeight + 1) return;
      el.tabIndex = 0;
      el.setAttribute("role", "region");
      el.setAttribute("aria-label", "Último comentario de " + el.closest("tr").querySelector("th").textContent.trim());
      var mark = function () {
        el.classList.toggle("more", el.scrollTop + el.clientHeight < el.scrollHeight - 1);
      };
      el.addEventListener("scroll", mark, { passive: true });
      mark();
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

  // Column filter panels are popovers (top layer, so a scrolling table can't clip
  // them). Place each under its funnel when it opens, keep it inside the viewport, and
  // follow the funnel if the page or the table scrolls or the window resizes.
  (function () {
    var openPanel = null;
    function place() {
      if (!openPanel) return;
      var btn = document.querySelector('[popovertarget="' + openPanel.id + '"]');
      if (!btn) return;
      var r = btn.getBoundingClientRect();
      var top = r.bottom + 4;
      openPanel.style.maxHeight = Math.max(160, window.innerHeight - top - 8) + "px";
      var left = Math.min(r.left, window.innerWidth - openPanel.offsetWidth - 8);
      openPanel.style.top = top + "px";
      openPanel.style.left = Math.max(8, left) + "px";
    }
    document.addEventListener("toggle", function (e) {
      if (!e.target.matches || !e.target.matches("form.col-filter-panel")) return;
      openPanel = e.newState === "open" ? e.target : null;
      place();
    }, true);
    window.addEventListener("scroll", place, true);
    window.addEventListener("resize", place);
  })();

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
    // Notes outside the form that only apply while there are unsaved changes (the PDF
    // link's warning). Hidden with visibility so their space stays reserved.
    var notes = document.querySelectorAll("[data-unsaved-note]");
    function show(state) {
      if (!status) return;
      status.textContent = status.dataset[state + "Text"];
      status.classList.remove("saved", "unsaved", "busy");
      status.classList.add(state);
    }
    function setDirty(v) {
      dirty = v;
      show(v ? "unsaved" : "saved");
      notes.forEach(function (n) { n.classList.toggle("active", v); });
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
