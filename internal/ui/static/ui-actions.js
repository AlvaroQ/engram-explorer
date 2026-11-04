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
//   data-auto-submit               change: submit the input's form (form.submit())
//   data-submit-confirm="text"     submit: cancel unless confirm(text) is accepted
//   data-merge-confirm-source="p"  submit: set hx-confirm from #merge-target value
//   data-show-on-swap              htmx:afterSwap into this element: clear display
//   data-remove-after-request="id" htmx:afterRequest from this element: remove #id
//   data-poll-visible-only         htmx:beforeRequest after the first one: skip
//                                  while the document is hidden
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
      input.form.submit();
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
      if (source !== null) {
        var sel = document.getElementById('merge-target');
        if (sel) {
          form.setAttribute(
            'hx-confirm',
            'Combine "' + source + '" into "' + sel.value + '"? This moves all its memories and cannot be undone.'
          );
        }
      }
    },
    true
  );

  document.addEventListener('htmx:afterSwap', function (e) {
    var target = e.target;
    if (target instanceof Element && target.hasAttribute('data-show-on-swap')) {
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
