(function () {
  'use strict';

  var app = document.getElementById('settings-app');
  if (!app) return;

  var statusEl = document.getElementById('settings-status');
  var restartBanner = document.getElementById('settings-restart-banner');
  var activeTab = app.getAttribute('data-active-tab') || 'general';
  var state = null;

  function showStatus(message, isError, persistMs) {
    if (!statusEl) return;
    statusEl.textContent = message;
    statusEl.className = 'settings-status' + (isError ? ' settings-status--error' : ' settings-status--ok');
    if (message) {
      var ttl = typeof persistMs === 'number' ? persistMs : 5000;
      if (ttl > 0) {
        setTimeout(function () {
          statusEl.textContent = '';
          statusEl.className = 'settings-status';
        }, ttl);
      }
    }
  }

  function showRestartBanner(show) {
    if (!restartBanner) return;
    restartBanner.classList.toggle('hidden', !show);
  }

  function switchTab(tab) {
    activeTab = tab;
    document.querySelectorAll('.settings-tab').forEach(function (btn) {
      btn.classList.toggle('active', btn.getAttribute('data-tab') === tab);
      btn.setAttribute('aria-selected', btn.getAttribute('data-tab') === tab ? 'true' : 'false');
    });
    document.querySelectorAll('.settings-panel').forEach(function (panel) {
      panel.classList.toggle('hidden', panel.getAttribute('data-panel') !== tab);
    });
    if (tab === 'security') {
      loadTLSStatus();
    }
  }

  function putJSON(url, body) {
    return fetch(url, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(function (r) {
      if (!r.ok) {
        return r.text().then(function (t) { throw new Error(t || r.statusText); });
      }
      return r.json();
    });
  }

  function postJSON(url, body) {
    return fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body || {})
    }).then(function (r) {
      if (!r.ok) {
        return r.text().then(function (t) { throw new Error(t || r.statusText); });
      }
      return r.json();
    });
  }

  function loadSettings() {
    return fetch('/api/settings').then(function (r) {
      if (!r.ok) throw new Error(t('Failed to load settings'));
      return r.json();
    }).then(function (data) {
      state = data;
      populateForms(data);
    });
  }

  function populateForms(data) {
    var g = data.general || {};
    setChecked('settings-update-check', g.update_check);
    setValue('settings-update-url', g.update_url || '');
    setValue('settings-parallel', g.parallel);
    setChecked('settings-sync-writes', g.sync_writes);
    setValue('settings-min-reserve', g.min_free_reserve_bytes);
    setValue('settings-log-max-entries', g.log_max_entries);
    setChecked('settings-log-file-enabled', g.log_file_enabled);
    setValue('settings-log-file-path', g.log_file_path || '');
    setValue('settings-log-rotate-mb', g.log_rotate_max_mb);
    setValue('settings-log-rotate-files', g.log_rotate_max_files);

    var s = data.security || {};
    setText('settings-password-hint', s.dashboard_password_configured
      ? t('Current: {hint}', {hint: s.dashboard_password_hint || t('configured')})
      : t('No password set (open access)'));

    var n = data.network || {};
    setValue('settings-serve-port', n.serve_port);
    setValue('settings-dashboard-port', n.serve_dashboard_port);
    setValue('settings-advertise-addr', n.serve_advertise_addr || '');

    var i = data.integrations || {};
    setText('settings-gemini-hint', i.gemini_configured ? t('Current: {hint}', {hint: i.gemini_api_key_hint || t('configured')}) : '');
    setValue('settings-gemini-model', i.gemini_model || '');
    updateIntegrationCards(i);
    populateIntegrationModals(i);
    var envBanner = document.getElementById('settings-env-override-banner');
    if (envBanner) {
      envBanner.classList.toggle('hidden', !i.env_overrides_present);
    }

    var note = data.notifications || {};
    setValue('settings-webhook-url', note.alert_webhook_url || '');
    setValue('settings-stale-threshold', note.alert_stale_threshold);
    setValue('settings-notifier-plugins', (note.alert_notifier_plugins || []).join(', '));

    populateEmail(data.email || {});

    showRestartBanner(!!data.restart_required);
  }

  function setValue(id, value) {
    var el = document.getElementById(id);
    if (el) el.value = value != null ? value : '';
  }

  function setChecked(id, value) {
    var el = document.getElementById(id);
    if (el) el.checked = !!value;
  }

  function setText(id, value) {
    var el = document.getElementById(id);
    if (el) el.textContent = value || '';
  }

  function escapeHTML(s) {
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function loadTLSStatus() {
    fetch('/api/settings/tls/status').then(function (r) {
      if (!r.ok) throw new Error('TLS status unavailable');
      return r.json();
    }).then(function (status) {
      var el = document.getElementById('settings-tls-status');
      if (!el) return;
      el.innerHTML = '';
      if (status.source === 'self_signed' && !status.cert_path) {
        el.innerHTML = '<dt>' + escapeHTML(t('Source')) + '</dt><dd>' + escapeHTML(t('Auto-generated self-signed certificate')) + '</dd>';
        return;
      }
      appendDL(el, t('Source'), t(status.source));
      if (status.cert_path) appendDL(el, t('Certificate path'), status.cert_path);
      if (status.subject) appendDL(el, t('Subject'), status.subject);
      if (status.issuer) appendDL(el, t('Issuer'), status.issuer);
      if (status.not_after) {
        var exp = status.expired
          ? t('{date} (expired)', {date: status.not_after})
          : t('{date} ({n} days left)', {date: status.not_after, n: status.days_left});
        appendDL(el, t('Expires'), exp);
      }
      if (status.sans && status.sans.length) {
        appendDL(el, t('SANs'), status.sans.join(', '));
      }
    }).catch(function () {});
  }

  function appendDL(dl, label, value) {
    var dt = document.createElement('dt');
    dt.textContent = label;
    var dd = document.createElement('dd');
    dd.textContent = value;
    dl.appendChild(dt);
    dl.appendChild(dd);
  }

  document.querySelectorAll('.settings-tab').forEach(function (btn) {
    btn.addEventListener('click', function () {
      switchTab(btn.getAttribute('data-tab'));
    });
  });

  var generalForm = document.getElementById('settings-general-form');
  if (generalForm) {
    generalForm.addEventListener('submit', function (e) {
      e.preventDefault();
      putJSON('/api/settings/general', {
        update_check: document.getElementById('settings-update-check').checked,
        update_url: document.getElementById('settings-update-url').value,
        parallel: parseInt(document.getElementById('settings-parallel').value, 10) || 0,
        sync_writes: document.getElementById('settings-sync-writes').checked,
        min_free_reserve_bytes: parseInt(document.getElementById('settings-min-reserve').value, 10) || 0,
        log_max_entries: parseInt(document.getElementById('settings-log-max-entries').value, 10) || 0,
        log_file_enabled: document.getElementById('settings-log-file-enabled').checked,
        log_file_path: document.getElementById('settings-log-file-path').value,
        log_rotate_max_mb: parseInt(document.getElementById('settings-log-rotate-mb').value, 10) || 0,
        log_rotate_max_files: parseInt(document.getElementById('settings-log-rotate-files').value, 10) || 0
      }).then(function () {
        showStatus(t('General settings saved.'));
        return loadSettings();
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var securityForm = document.getElementById('settings-security-form');
  if (securityForm) {
    securityForm.addEventListener('submit', function (e) {
      e.preventDefault();
      var pw = document.getElementById('settings-dashboard-password').value;
      var body = {};
      if (pw) body.dashboard_password = pw;
      putJSON('/api/settings/security', body).then(function (res) {
        showStatus(t('Security settings saved.'));
        if (res.restart_required) showRestartBanner(true);
        document.getElementById('settings-dashboard-password').value = '';
        return loadSettings();
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var networkForm = document.getElementById('settings-network-form');
  if (networkForm) {
    networkForm.addEventListener('submit', function (e) {
      e.preventDefault();
      putJSON('/api/settings/network', {
        serve_port: parseInt(document.getElementById('settings-serve-port').value, 10),
        serve_dashboard_port: parseInt(document.getElementById('settings-dashboard-port').value, 10),
        serve_advertise_addr: document.getElementById('settings-advertise-addr').value
      }).then(function (res) {
        showStatus(t('Network settings saved.'));
        if (res.restart_required) showRestartBanner(true);
        return loadSettings();
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var notificationsForm = document.getElementById('settings-notifications-form');
  if (notificationsForm) {
    notificationsForm.addEventListener('submit', function (e) {
      e.preventDefault();
      var plugins = document.getElementById('settings-notifier-plugins').value;
      putJSON('/api/settings/notifications', {
        alert_webhook_url: document.getElementById('settings-webhook-url').value,
        alert_stale_threshold: parseInt(document.getElementById('settings-stale-threshold').value, 10),
        alert_notifier_plugins: plugins ? plugins.split(',').map(function (s) { return s.trim(); }).filter(Boolean) : []
      }).then(function () {
        showStatus(t('Notification settings saved.'));
        return loadSettings();
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var integrationsForm = document.getElementById('settings-integrations-form');
  if (integrationsForm) {
    integrationsForm.addEventListener('submit', function (e) {
      e.preventDefault();
      // Filesystem product focus: Gemini (Sentinel) is the only integration surface.
      var modelEl = document.getElementById('settings-gemini-model');
      var body = {
        gemini_model: modelEl ? modelEl.value : ''
      };
      var keyEl = document.getElementById('settings-gemini-key');
      var key = keyEl ? keyEl.value : '';
      if (key) body.gemini_api_key = key;
      putJSON('/api/settings/integrations', body).then(function () {
        showStatus(t('Integration settings saved.'));
        if (keyEl) keyEl.value = '';
        return loadSettings();
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var tlsForm = document.getElementById('settings-tls-upload-form');
  if (tlsForm) {
    tlsForm.addEventListener('submit', function (e) {
      e.preventDefault();
      var fd = new FormData(tlsForm);
      fetch('/api/settings/tls/upload', { method: 'POST', body: fd }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t || r.statusText); });
        return r.json();
      }).then(function (res) {
        showStatus(t('TLS certificate uploaded.'));
        if (res.restart_required) showRestartBanner(true);
        loadTLSStatus();
        tlsForm.reset();
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var importEnvBtn = document.getElementById('settings-import-env');
  if (importEnvBtn) {
    importEnvBtn.addEventListener('click', function () {
      postJSON('/api/settings/integrations/import-env').then(function () {
        showStatus(t('Imported integration settings from environment.'));
        return loadSettings();
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var testGeminiBtn = document.getElementById('settings-test-gemini');
  if (testGeminiBtn) {
    testGeminiBtn.addEventListener('click', function () {
      var body = {};
      var key = document.getElementById('settings-gemini-key').value;
      if (key) body.gemini_api_key = key;
      postJSON('/api/settings/test/gemini', body).then(function (res) {
        showStatus(res.ok ? t('Gemini connection successful.') : (res.error || t('Gemini test failed')), !res.ok);
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var testWebhookBtn = document.getElementById('settings-test-webhook');
  if (testWebhookBtn) {
    testWebhookBtn.addEventListener('click', function () {
      postJSON('/api/settings/test/webhook', {
        alert_webhook_url: document.getElementById('settings-webhook-url').value
      }).then(function (res) {
        showStatus(res.ok ? t('Webhook test sent.') : (res.error || t('Webhook test failed')), !res.ok);
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  function openModal(id) {
    var el = document.getElementById(id);
    if (el) el.hidden = false;
  }

  function closeModal(id) {
    var el = document.getElementById(id);
    if (el) el.hidden = true;
  }

  function closeAllModals() {
    document.querySelectorAll('.dialog-overlay').forEach(function (el) {
      el.hidden = true;
    });
  }

  document.querySelectorAll('[data-close-modal]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      closeModal(btn.getAttribute('data-close-modal'));
    });
  });

  document.querySelectorAll('.dialog-overlay').forEach(function (overlay) {
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) closeModal(overlay.id);
    });
  });

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') closeAllModals();
  });

  function copyFieldValue(fromId, toId) {
    var from = document.getElementById(fromId);
    var to = document.getElementById(toId);
    if (from && to) to.value = from.value;
  }

  function syncGeminiModalToHidden() {
    copyFieldValue('gemini-modal-model', 'settings-gemini-model');
    var key = document.getElementById('gemini-modal-key');
    var hidden = document.getElementById('settings-gemini-key');
    if (key && hidden && key.value) hidden.value = key.value;
  }

  function populateIntegrationModals(i) {
    i = i || {};
    setValue('gemini-modal-model', i.gemini_model || '');
    setText('gemini-modal-hint', i.gemini_configured
      ? t('Current: {hint}', {hint: i.gemini_api_key_hint || t('configured')}) : '');
  }

  function updateIntegrationCards(i) {
    i = i || {};
    var geminiBadge = document.getElementById('integration-gemini-badge');
    if (geminiBadge) {
      if (i.gemini_configured) {
        geminiBadge.textContent = t('Configured');
        geminiBadge.className = 'integration-badge integration-badge--ok';
      } else {
        geminiBadge.textContent = t('Not configured');
        geminiBadge.className = 'integration-badge';
      }
    }
  }

  var geminiOpenBtn = document.getElementById('integration-gemini-open');
  if (geminiOpenBtn) geminiOpenBtn.addEventListener('click', function () { openModal('integration-gemini-modal'); });

  var geminiModalSave = document.getElementById('gemini-modal-save');
  if (geminiModalSave) {
    geminiModalSave.addEventListener('click', function () {
      syncGeminiModalToHidden();
      var body = { gemini_model: document.getElementById('settings-gemini-model').value };
      var key = document.getElementById('settings-gemini-key').value;
      if (key) body.gemini_api_key = key;
      putJSON('/api/settings/integrations', body).then(function () {
        showStatus(t('Gemini settings saved.'));
        document.getElementById('gemini-modal-key').value = '';
        closeModal('integration-gemini-modal');
        return loadSettings();
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  var geminiModalTest = document.getElementById('gemini-modal-test');
  if (geminiModalTest) {
    geminiModalTest.addEventListener('click', function () {
      syncGeminiModalToHidden();
      var body = {};
      var key = document.getElementById('settings-gemini-key').value;
      if (key) body.gemini_api_key = key;
      postJSON('/api/settings/test/gemini', body).then(function (res) {
        showStatus(res.ok ? t('Gemini connection successful.') : (res.error || t('Gemini test failed')), !res.ok);
      }).catch(function (err) { showStatus(err.message, true); });
    });
  }

  // --- Email ---

  function emailAuthChanged() {
    var auth = document.getElementById('settings-email-auth');
    if (!auth) return;
    document.querySelectorAll('[data-email-auth]').forEach(function (el) {
      el.hidden = el.getAttribute('data-email-auth') !== auth.value;
    });
  }

  function populateEmail(e) {
    setValue('settings-email-host', e.host || '');
    setValue('settings-email-port', e.port || '');
    setValue('settings-email-security', e.security || 'starttls');
    setChecked('settings-email-skip-verify', e.tls_skip_verify);
    setValue('settings-email-auth', e.auth || 'none');
    setValue('settings-email-username', e.auth === 'password' ? e.username || '' : '');
    setValue('settings-email-oauth-user', e.auth === 'microsoft365' ? e.username || '' : '');
    setValue('settings-email-password', '');
    setValue('settings-email-secret', '');
    setText('settings-email-password-hint', e.password_set ? t('Saved. Leave it blank to keep it.') : '');
    setText('settings-email-secret-hint', e.client_secret_set ? t('Saved. Leave it blank to keep it.') : '');
    setValue('settings-email-tenant', e.tenant_id || '');
    setValue('settings-email-client', e.client_id || '');
    setValue('settings-email-from', e.from || '');
    setValue('settings-email-from-name', e.from_name || '');
    setValue('settings-email-dashboard-url', e.dashboard_url || '');
    var url = document.getElementById('settings-email-dashboard-url');
    if (url) url.placeholder = location.origin;
    var stateEl = document.getElementById('settings-email-state');
    if (stateEl) {
      stateEl.innerHTML = e.configured
        ? '<span class="badge badge-ok">' + escapeHTML(t('Email on')) + '</span> ' +
          escapeHTML(t('New users get an invitation by email, and anyone can ask for a password reset link from the sign-in page.'))
        : '<span class="badge badge-pending">' + escapeHTML(t('Email off')) + '</span> ' +
          escapeHTML(t('Administrators give new users a temporary password, and reset forgotten passwords themselves.'));
    }
    emailAuthChanged();
  }

  function emailBody() {
    var auth = document.getElementById('settings-email-auth').value;
    var val = function (id) { return document.getElementById(id).value.trim(); };
    return {
      host: val('settings-email-host'),
      port: parseInt(val('settings-email-port'), 10) || 0,
      security: document.getElementById('settings-email-security').value,
      tls_skip_verify: document.getElementById('settings-email-skip-verify').checked,
      auth: auth,
      username: auth === 'microsoft365' ? val('settings-email-oauth-user') : val('settings-email-username'),
      password: document.getElementById('settings-email-password').value,
      tenant_id: val('settings-email-tenant'),
      client_id: val('settings-email-client'),
      client_secret: val('settings-email-secret'),
      from: val('settings-email-from'),
      from_name: val('settings-email-from-name'),
      dashboard_url: val('settings-email-dashboard-url')
    };
  }

  var emailPresets = {
    relay: { port: 25, security: 'none', auth: 'none' },
    microsoft365: { host: 'smtp.office365.com', port: 587, security: 'starttls', auth: 'microsoft365' }
  };
  document.querySelectorAll('[data-email-preset]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var p = emailPresets[btn.getAttribute('data-email-preset')];
      if (p.host) setValue('settings-email-host', p.host);
      setValue('settings-email-port', p.port);
      setValue('settings-email-security', p.security);
      setValue('settings-email-auth', p.auth);
      setChecked('settings-email-skip-verify', false);
      emailAuthChanged();
      document.getElementById(p.host ? 'settings-email-from' : 'settings-email-host').focus();
    });
  });

  var emailAuth = document.getElementById('settings-email-auth');
  if (emailAuth) emailAuth.addEventListener('change', emailAuthChanged);

  var emailSecurity = document.getElementById('settings-email-security');
  if (emailSecurity) emailSecurity.addEventListener('change', function () {
    var port = document.getElementById('settings-email-port');
    var defaults = { none: '25', starttls: '587', tls: '465' };
    if (!port.value || ['25', '587', '465'].indexOf(port.value) >= 0) port.value = defaults[emailSecurity.value];
  });

  var emailForm = document.getElementById('settings-email-form');
  if (emailForm) {
    emailForm.addEventListener('submit', function (e) {
      e.preventDefault();
      putJSON('/api/settings/email', emailBody()).then(function () {
        showStatus(t('Email settings saved.'));
        return loadSettings();
      }).catch(function (err) { showStatus(err.message, true, 0); });
    });
  }

  var emailTest = document.getElementById('settings-email-test');
  if (emailTest) {
    emailTest.addEventListener('click', function () {
      var body = emailBody();
      body.to = document.getElementById('settings-email-test-to').value.trim();
      emailTest.disabled = true;
      showStatus(t('Sending...'), false, 0);
      postJSON('/api/settings/email/test', body).then(function (res) {
        if (res.ok) showStatus(t('Test email sent to {to}. Check that it arrived.', { to: res.to }), false, 0);
        else showStatus(t('The test email was not sent: {error}', { error: res.error || '' }), true, 0);
      }).catch(function (err) { showStatus(err.message, true, 0); })
        .then(function () { emailTest.disabled = false; });
    });
  }

  switchTab(activeTab);
  loadSettings().catch(function (err) { showStatus(err.message, true); });
})();
