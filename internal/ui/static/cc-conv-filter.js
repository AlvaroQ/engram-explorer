// Client-side conversation filter for the Claude Code session detail view.
// The full transcript is already in the DOM, so filtering is pure show/hide:
// each content block carries data-cc-block (thinking | tool | attachment |
// text | other) and each toggle carries data-cc-toggle for one of the hideable
// groups. Hiding is done with a class on the .cc-conv wrapper (CSS does the rest)
// plus a per-turn "empty" class so turns left with no visible block disappear.

(function () {
  "use strict";

  // Recompute visibility for one .cc-conv wrapper from its toggle states.
  function apply(conv) {
    if (!conv) return;
    var toggles = conv.querySelectorAll("[data-cc-toggle]");
    var hidden = {};
    toggles.forEach(function (t) {
      var type = t.getAttribute("data-cc-toggle");
      hidden[type] = !t.checked;
      conv.classList.toggle("cc-hide-" + type, !t.checked);
    });

    // Hide any turn whose every block is currently hidden. Blocks of a type with
    // no toggle (text, other) keep the turn visible.
    conv.querySelectorAll(".timeline-event").forEach(function (ev) {
      var blocks = ev.querySelectorAll("[data-cc-block]");
      if (blocks.length === 0) {
        ev.classList.remove("cc-turn-empty");
        return;
      }
      var anyVisible = false;
      blocks.forEach(function (b) {
        if (!hidden[b.getAttribute("data-cc-block")]) anyVisible = true;
      });
      ev.classList.toggle("cc-turn-empty", !anyVisible);
    });

    // Reflect compact state: the button is "active" when all groups are hidden.
    var btn = conv.querySelector("[data-cc-compact]");
    if (btn) {
      var allHidden = true;
      toggles.forEach(function (t) {
        if (t.checked) allHidden = false;
      });
      btn.classList.toggle("is-active", allHidden);
      btn.setAttribute("aria-pressed", String(allHidden));
    }
  }

  // Compact toggle: if anything is still visible, hide all groups; otherwise
  // restore everything. Then re-apply.
  function compact(conv) {
    if (!conv) return;
    var toggles = conv.querySelectorAll("[data-cc-toggle]");
    var anyChecked = false;
    toggles.forEach(function (t) {
      if (t.checked) anyChecked = true;
    });
    var next = !anyChecked; // something visible -> uncheck all; all hidden -> check all
    toggles.forEach(function (t) {
      t.checked = next;
    });
    apply(conv);
  }

  // Exposed for any caller; the listeners below drive it via event delegation.
  window.ccApplyConvFilters = apply;

  // Bind once per document (idempotent across HTMX swaps / re-injection).
  if (!window.__ccConvFilterBound) {
    window.__ccConvFilterBound = true;

    document.addEventListener("change", function (e) {
      var t = e.target;
      if (t && t.matches && t.matches("[data-cc-toggle]")) {
        apply(t.closest(".cc-conv"));
      }
    });

    document.addEventListener("click", function (e) {
      var b = e.target.closest && e.target.closest("[data-cc-compact]");
      if (b) compact(b.closest(".cc-conv"));
    });
  }
})();
