// ui-actions.js — delegated behaviours that replace inline event handlers and
// inline <script> blocks, so the Content-Security-Policy can use
// script-src 'self' without 'unsafe-inline'. htmx runs with allowEval:false,
// so hx-on attributes and hx-trigger [filters] are not used either.
//
// Every listener is attached to document, so markup swapped in later by htmx
// is covered without re-binding. Templates opt in with data-* attributes:
//
//   data-hide-target="id"          click: set #id display to none
//   data-clear-target="id"         click: empty #id (innerHTML = '')
//   data-escape-clear-target="id"  Escape while the element is in the DOM: empty #id
//   data-auto-submit               change: submit the input's form via
//                                  requestSubmit(), so submit listeners such
//                                  as data-submit-confirm still run
//   data-submit-confirm="text"     submit: cancel unless confirm(text) is accepted
//   data-merge-confirm-source="p"  submit: set hx-confirm from #merge-target value,
//                                  using data-merge-confirm-template (localized,
//                                  {source}/{target} placeholders)
//   data-show-on-swap              htmx:afterSwap into this element: clear the
//                                  hidden attribute and any inline display
//   data-remove-after-request="id" htmx:afterRequest from this element: remove #id
//   data-poll-visible-only         htmx:beforeRequest after the first one: skip
//                                  while the document is hidden
//   data-modal-dialog="id"         <dialog> swapped in by htmx: showModal() it
//                                  (focus trap, native Esc). On close: empty #id
//                                  (when given) and refocus the element that
//                                  had focus when it opened
//   data-dialog-close              click: close() the enclosing <dialog>
//   data-menu-button               <button aria-controls="menu-id">: menu-button
//                                  pattern. Click/Enter/Space toggle the menu,
//                                  ArrowDown/ArrowUp open it on the first/last
//                                  item; inside, arrows/Home/End move, Esc closes
//                                  and refocuses the button, Tab closes it and
//                                  lets focus move on, a click outside closes it
(function () {
  'use strict';

  function byId(id) {
    return id ? document.getElementById(id) : null;
  }

  function closestWith(target, attr) {
    return target instanceof Element ? target.closest('[' + attr + ']') : null;
  }

  document.addEventListener('click', function (e) {
    var hide = closestWith(e.target, 'data-hide-target');
    if (hide) {
      var el = byId(hide.getAttribute('data-hide-target'));
      if (el) {
        el.style.display = 'none';
      }
    }
    var clear = closestWith(e.target, 'data-clear-target');
    if (clear) {
      var host = byId(clear.getAttribute('data-clear-target'));
      if (host) {
        host.innerHTML = '';
      }
    }
  });

  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') {
      return;
    }
    var owner = document.querySelector('[data-escape-clear-target]');
    if (owner) {
      var host = byId(owner.getAttribute('data-escape-clear-target'));
      if (host) {
        host.innerHTML = '';
      }
    }
  });

  document.addEventListener('change', function (e) {
    var input = e.target;
    if (input instanceof Element && input.hasAttribute('data-auto-submit') && input.form) {
      input.form.requestSubmit();
    }
  });

  // Capture phase: runs before htmx's own submit listener on the form, so an
  // updated hx-confirm is read by htmx when it issues the request.
  document.addEventListener(
    'submit',
    function (e) {
      var form = e.target;
      if (!(form instanceof HTMLFormElement)) {
        return;
      }
      var message = form.getAttribute('data-submit-confirm');
      if (message !== null && !window.confirm(message)) {
        e.preventDefault();
        return;
      }
      var source = form.getAttribute('data-merge-confirm-source');
      var template = form.getAttribute('data-merge-confirm-template');
      if (source !== null && template !== null) {
        var sel = document.getElementById('merge-target');
        if (sel) {
          // One pass, so a project name containing a placeholder stays literal.
          var target = sel.value;
          form.setAttribute(
            'hx-confirm',
            template.replace(/\{(source|target)\}/g, function (_, k) {
              return k === 'source' ? source : target;
            }),
          );
        }
      }
    },
    true,
  );

  // ---- Menu buttons ----
  function menuOf(button) {
    return byId(button.getAttribute('aria-controls'));
  }

  function menuItems(menu) {
    return Array.prototype.slice.call(menu.querySelectorAll('[role="menuitem"]'));
  }

  function setMenuOpen(button, open, focusAt) {
    var menu = menuOf(button);
    if (!menu) {
      return;
    }
    button.setAttribute('aria-expanded', open ? 'true' : 'false');
    menu.hidden = !open;
    if (!open) {
      return;
    }
    var items = menuItems(menu);
    var item = focusAt === 'last' ? items[items.length - 1] : items[0];
    if (item) {
      item.focus();
    }
  }

  function closeMenus(except) {
    var open = document.querySelectorAll('[data-menu-button][aria-expanded="true"]');
    for (var i = 0; i < open.length; i++) {
      if (open[i] !== except) {
        setMenuOpen(open[i], false);
      }
    }
  }

  document.addEventListener('click', function (e) {
    var button = closestWith(e.target, 'data-menu-button');
    if (button) {
      var opening = button.getAttribute('aria-expanded') !== 'true';
      closeMenus(button);
      setMenuOpen(button, opening, 'first');
      return;
    }
    // A click on a menu item runs its own action (form submit, link) first;
    // the menu closes after it. Any other click closes it right away.
    if (e.target instanceof Element && e.target.closest('[role="menu"]')) {
      setTimeout(function () {
        closeMenus(null);
      }, 0);
      return;
    }
    closeMenus(null);
  });

  document.addEventListener('keydown', function (e) {
    var button = closestWith(e.target, 'data-menu-button');
    if (button && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault();
      setMenuOpen(button, true, e.key === 'ArrowUp' ? 'last' : 'first');
      return;
    }
    var menu = e.target instanceof Element ? e.target.closest('[role="menu"]') : null;
    if (!menu) {
      return;
    }
    var owner = document.querySelector('[data-menu-button][aria-controls="' + menu.id + '"]');
    if (!owner) {
      return;
    }
    var items = menuItems(menu);
    var at = items.indexOf(e.target);
    var next = null;
    if (e.key === 'ArrowDown') {
      next = items[(at + 1) % items.length];
    } else if (e.key === 'ArrowUp') {
      next = items[(at - 1 + items.length) % items.length];
    } else if (e.key === 'Home') {
      next = items[0];
    } else if (e.key === 'End') {
      next = items[items.length - 1];
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setMenuOpen(owner, false);
      owner.focus();
      return;
    } else if (e.key === 'Tab') {
      setMenuOpen(owner, false);
      return;
    }
    if (next) {
      e.preventDefault();
      next.focus();
    }
  });

  // ---- Modal dialogs ----
  // The element that issued the last htmx request: the opener when the
  // browser did not move focus to the clicked button (Safari).
  var lastRequester = null;
  document.addEventListener('htmx:beforeRequest', function (e) {
    if (e.detail && e.detail.elt instanceof HTMLElement) {
      lastRequester = e.detail.elt;
    }
  });

  function openModals(root) {
    if (!(root instanceof Element)) {
      return;
    }
    if (root.matches('dialog[data-modal-dialog]')) {
      openModal(root);
    }
    var list = root.querySelectorAll('dialog[data-modal-dialog]');
    for (var i = 0; i < list.length; i++) {
      openModal(list[i]);
    }
  }

  function openModal(dialog) {
    if (dialog.open || typeof dialog.showModal !== 'function') {
      return;
    }
    var opener =
      document.activeElement && document.activeElement !== document.body
        ? document.activeElement
        : lastRequester;
    dialog.addEventListener(
      'close',
      function () {
        var host = byId(dialog.getAttribute('data-modal-dialog'));
        if (host && host.contains(dialog)) {
          host.innerHTML = '';
        }
        if (opener instanceof HTMLElement && opener.isConnected) {
          opener.focus();
        }
      },
      { once: true },
    );
    dialog.showModal();
  }

  document.addEventListener('click', function (e) {
    var trigger = closestWith(e.target, 'data-dialog-close');
    var dialog = trigger ? trigger.closest('dialog') : null;
    if (dialog && dialog.open) {
      dialog.close();
    }
  });

  // An action inside the dialog can re-render the region that holds it
  // (e.g. assigning an orphan replaces #orphans-list). The dialog then leaves
  // the DOM without a close event; keep focus on the page instead of <body>.
  document.addEventListener('htmx:beforeSwap', function () {
    var open = document.querySelector('dialog[data-modal-dialog][open]');
    if (!open) {
      return;
    }
    setTimeout(function () {
      if (
        !open.isConnected &&
        (document.activeElement === document.body || !document.activeElement)
      ) {
        var main = byId('main-content');
        if (main) {
          main.focus();
        }
      }
    }, 0);
  });

  document.addEventListener('htmx:load', function (e) {
    openModals(e.target);
  });

  document.addEventListener('htmx:afterSwap', function (e) {
    var target = e.target;
    if (target instanceof HTMLElement && target.hasAttribute('data-show-on-swap')) {
      target.hidden = false;
      target.style.display = '';
    }
  });

  document.addEventListener('htmx:afterRequest', function (e) {
    var elt = e.target;
    if (!(elt instanceof Element)) {
      return;
    }
    var id = elt.getAttribute('data-remove-after-request');
    if (id) {
      var row = byId(id);
      if (row) {
        row.remove();
      }
    }
  });

  // The first request of a polled element is its "load" trigger and always
  // runs; later (interval) requests are skipped while the tab is hidden.
  var polled = new WeakSet();
  document.addEventListener('htmx:beforeRequest', function (e) {
    var elt = e.detail && e.detail.elt;
    if (!(elt instanceof Element) || !elt.hasAttribute('data-poll-visible-only')) {
      return;
    }
    if (!polled.has(elt)) {
      polled.add(elt);
      return;
    }
    if (document.visibilityState !== 'visible') {
      e.preventDefault();
    }
  });
})();
