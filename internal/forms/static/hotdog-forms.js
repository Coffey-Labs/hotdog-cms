// hotdog-cms forms. Fetches each form's anti-spam token when the page loads, sends
// the form without leaving the page, and shows problems next to the fields
// they're about. Without this script the form still posts, and the endpoint
// answers with a page that explains what went wrong.
(function () {
  'use strict';
  function setStatus(form, text) { form.querySelector('.hotdog-status').textContent = text; }
  function clearErrors(form) {
    form.querySelectorAll('.hotdog-error').forEach(function (p) { p.textContent = ''; });
    form.querySelectorAll('[aria-invalid]').forEach(function (el) { el.removeAttribute('aria-invalid'); });
  }
  function token(form) {
    return fetch('/_hotdog/forms/' + form.dataset.hotdogForm + '/token', { credentials: 'omit' })
      .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
      .then(function (d) { form.querySelector('input[name="_t"]').value = d.token; });
  }
  function setup(form) {
    token(form).catch(function () {
      setStatus(form, 'This form can’t be sent right now. Please try again later.');
    });
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      clearErrors(form);
      var button = form.querySelector('button[type="submit"]');
      button.disabled = true;
      setStatus(form, 'Sending…');
      fetch(form.action, {
        method: 'POST',
        credentials: 'omit',
        headers: { 'Accept': 'application/json' },
        body: new URLSearchParams(new FormData(form))
      }).then(function (r) {
        return r.json().catch(function () { return {}; }).then(function (d) { return { ok: r.ok, d: d }; });
      }).then(function (res) {
        if (res.ok) {
          if (form.dataset.success) { location.href = form.dataset.success; return; }
          form.reset();
          setStatus(form, res.d.message || 'Thanks, your message was sent.');
          token(form).catch(function () {});
          return;
        }
        var first = null;
        (res.d.problems || []).forEach(function (p) {
          var field = p.field && form.querySelector('[name="' + p.field + '"]');
          if (field) {
            field.setAttribute('aria-invalid', 'true');
            document.getElementById(field.id + '-error').textContent = p.message;
            first = first || field;
          }
        });
        setStatus(form, res.d.message || 'Please check the form and try again.');
        if (first) first.focus();
        if (res.d.retoken) token(form).catch(function () {});
      }).catch(function () {
        setStatus(form, 'The message couldn’t be sent. Please check your connection and try again.');
      }).then(function () { button.disabled = false; });
    });
  }
  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('form[data-hotdog-form]').forEach(setup);
  });
})();
