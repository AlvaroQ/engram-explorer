// toasts.js — accessible feedback for failed htmx requests.
//
// htmx is configured (htmx-config meta, responseHandling) so 4xx/5xx
// responses never replace their target. This script turns those failures
// into toasts in #toast-region (aria-live="polite"; each error toast is
// role="alert"):
//   htmx:responseError  the server answered 4xx/5xx
//   htmx:sendError      the request never reached the server (network)
//
// The message is the server's user-facing text when the response carries one:
// the .inline-error-message span of an ErrorPartial (HTML) or error.message
// of a JSON error body. Anything else (plain-text http.Error bodies are
// developer strings) falls back to the localized messages the layout renders
// on the region as data-msg-generic / -network / -unavailable.
//
// Requests issued by an element inside [data-no-error-toast] (polled widgets
// that report their own state) raise no toast. Toasts dismiss themselves after
// a delay that pauses while hovered or focused, and have a close button.
// No inline styles: every visual rule lives in styles.css.
(function () {
  'use strict';

  var MAX_MESSAGE = 300;
  var DISMISS_MS = 8000;
  var MAX_TOASTS = 3;

  // toastMessage picks the toast text for a failed request. Pure: the HTML
  // extraction is passed in so it can be checked without a DOM.
  //   failure: { network: bool, status: number, contentType: string, body: string }
  //   msgs:    { generic, network, unavailable }
  //   htmlMessage(body) -> string (text of .inline-error-message, or '')
  function toastMessage(failure, msgs, htmlMessage) {
    if (failure.network) {
      return msgs.network;
    }
    var type = (failure.contentType || '').toLowerCase();
    var text = '';
    if (type.indexOf('application/json') !== -1) {
      try {
        var parsed = JSON.parse(failure.body || '');
        var err = parsed && parsed.error;
        if (err && typeof err.message === 'string') {
          text = err.message;
        }
      } catch (_) {
        text = '';
      }
    } else if (type.indexOf('text/html') !== -1 && htmlMessage) {
      text = htmlMessage(failure.body || '');
    }
    text = (text || '').replace(/\s+/g, ' ').trim();
    if (text && text.length <= MAX_MESSAGE) {
      return text;
    }
    return failure.status === 503 ? msgs.unavailable : msgs.generic;
  }

  var core = { toastMessage: toastMessage };
  if (typeof module === 'object' && module.exports) {
    module.exports = core;
    return;
  }

  function htmlMessage(body) {
    var doc = new DOMParser().parseFromString(body, 'text/html');
    var el = doc.querySelector('.inline-error-message');
    return el ? el.textContent || '' : '';
  }

  function region() {
    return document.getElementById('toast-region');
  }

  function messages(r) {
    return {
      generic: r.getAttribute('data-msg-generic') || '',
      network: r.getAttribute('data-msg-network') || '',
      unavailable: r.getAttribute('data-msg-unavailable') || '',
    };
  }

  function dismiss(toast) {
    if (toast._timer) {
      clearTimeout(toast._timer);
    }
    toast.remove();
  }

  function arm(toast) {
    if (toast._timer) {
      clearTimeout(toast._timer);
    }
    toast._timer = setTimeout(function () {
      dismiss(toast);
    }, DISMISS_MS);
  }

  function pause(toast) {
    if (toast._timer) {
      clearTimeout(toast._timer);
      toast._timer = null;
    }
  }

  function show(text) {
    var r = region();
    if (!r || !text) {
      return;
    }
    // The same failure repeating (e.g. a retried request) restarts the
    // visible toast instead of stacking copies.
    var existing = r.querySelectorAll('.toast');
    for (var i = 0; i < existing.length; i++) {
      if (existing[i].getAttribute('data-message') === text) {
        arm(existing[i]);
        return;
      }
    }
    while (r.querySelectorAll('.toast').length >= MAX_TOASTS) {
      dismiss(r.querySelector('.toast'));
    }

    var toast = document.createElement('div');
    toast.className = 'toast toast--error';
    toast.setAttribute('role', 'alert');
    toast.setAttribute('data-message', text);

    var p = document.createElement('p');
    p.className = 'toast-message';
    p.textContent = text;

    var close = document.createElement('button');
    close.type = 'button';
    close.className = 'toast-close';
    close.setAttribute('aria-label', r.getAttribute('data-label-close') || 'Close');
    close.textContent = '\u00d7';
    close.addEventListener('click', function () {
      dismiss(toast);
    });

    toast.appendChild(p);
    toast.appendChild(close);
    toast.addEventListener('mouseenter', function () {
      pause(toast);
    });
    toast.addEventListener('mouseleave', function () {
      if (!toast.contains(document.activeElement)) {
        arm(toast);
      }
    });
    toast.addEventListener('focusin', function () {
      pause(toast);
    });
    toast.addEventListener('focusout', function (e) {
      if (!toast.contains(e.relatedTarget)) {
        arm(toast);
      }
    });
    r.appendChild(toast);
    arm(toast);
  }

  function optedOut(e) {
    var elt = e.detail && e.detail.elt;
    return elt instanceof Element && elt.closest('[data-no-error-toast]') !== null;
  }

  document.addEventListener('htmx:responseError', function (e) {
    var r = region();
    if (!r || optedOut(e)) {
      return;
    }
    var xhr = e.detail && e.detail.xhr;
    if (!xhr) {
      return;
    }
    show(
      toastMessage(
        {
          network: false,
          status: xhr.status,
          contentType: xhr.getResponseHeader('Content-Type') || '',
          body: typeof xhr.responseText === 'string' ? xhr.responseText : '',
        },
        messages(r),
        htmlMessage,
      ),
    );
  });

  document.addEventListener('htmx:sendError', function (e) {
    var r = region();
    if (!r || optedOut(e)) {
      return;
    }
    show(toastMessage({ network: true }, messages(r), htmlMessage));
  });
})();
