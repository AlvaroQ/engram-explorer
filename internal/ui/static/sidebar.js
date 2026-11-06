// sidebar.js — Material 3 responsive navigation drawer behavior.
//
// Two interaction modes, chosen by viewport width:
//   • >= 640px: the toggle switches between the 64px icon rail (default) and
//     the 236px drawer.
//     The preference is persisted in the "sidebar" cookie so the server can
//     render the correct initial state with no flash on reload.
//   • <  640px: the sidebar is a fixed rail; the toggle expands it to an
//     overlay drawer with a scrim (modal). Esc and scrim-click close it.
//
// No framework. Vanilla, dependency-free. Listeners attach to persistent
// shell elements (outside the HTMX swap target), so they survive partial swaps.
(function () {
  'use strict';

  var COOKIE = 'sidebar';
  var COMPACT = '(max-width: 639px)';

  function setCookie(value) {
    document.cookie = COOKIE + '=' + value + ';path=/;max-age=31536000;samesite=lax';
  }

  function isCompact() {
    return window.matchMedia(COMPACT).matches;
  }

  function init() {
    var shell = document.querySelector('.app-shell');
    var sidebar = document.getElementById('sidebar');
    var toggle = document.getElementById('sidebar-toggle');
    var scrim = document.getElementById('sidebar-scrim');
    if (!shell || !sidebar || !toggle) {
      return;
    }

    var lastFocus = null;

    // Keep the toggle named by the action it performs next (labels are
    // rendered localized on the button as data-label-expand/-collapse).
    function setExpanded(expanded) {
      toggle.setAttribute('aria-expanded', expanded ? 'true' : 'false');
      var label = toggle.getAttribute(expanded ? 'data-label-collapse' : 'data-label-expand');
      if (label) {
        toggle.setAttribute('aria-label', label);
      }
    }

    function onKeydown(e) {
      if (e.key === 'Escape') {
        closeModal();
      }
    }

    function openModal() {
      shell.classList.add('sidebar-open');
      if (scrim) {
        scrim.classList.add('is-visible');
      }
      setExpanded(true);
      lastFocus = document.activeElement;
      var first = sidebar.querySelector('.sidebar-nav a');
      if (first) {
        first.focus();
      }
      document.addEventListener('keydown', onKeydown);
    }

    function closeModal() {
      shell.classList.remove('sidebar-open');
      if (scrim) {
        scrim.classList.remove('is-visible');
      }
      setExpanded(false);
      document.removeEventListener('keydown', onKeydown);
      if (lastFocus && typeof lastFocus.focus === 'function') {
        lastFocus.focus();
      }
    }

    function toggleRail() {
      var collapsed = shell.classList.toggle('sidebar-collapsed');
      sidebar.classList.toggle('sidebar--collapsed', collapsed);
      setCookie(collapsed ? 'collapsed' : 'expanded');
      setExpanded(!collapsed);
    }

    toggle.addEventListener('click', function () {
      if (isCompact()) {
        if (shell.classList.contains('sidebar-open')) {
          closeModal();
        } else {
          openModal();
        }
      } else {
        toggleRail();
      }
    });

    if (scrim) {
      scrim.addEventListener('click', closeModal);
    }

    // Leaving compact width with the modal open: clean up the overlay state.
    window.addEventListener('resize', function () {
      if (!isCompact() && shell.classList.contains('sidebar-open')) {
        closeModal();
      }
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
