(function () {
  'use strict';

  var plans = [];
  var selected = '';
  var plan = null;
  var agents = [];
  var selectedJobs = {};

  function $(id) { return document.getElementById(id); }

  function formatBytes(b) {
    b = Number(b) || 0;
    if (b >= 1099511627776) return (b / 1099511627776).toFixed(1) + ' TiB';
    if (b >= 1073741824) return (b / 1073741824).toFixed(1) + ' GiB';
    if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MiB';
    if (b >= 1024) return (b / 1024).toFixed(1) + ' KiB';
    return b + ' B';
  }

  function shortID(id) {
    return id && id.length > 12 ? id.substring(0, 12) : (id || '—');
  }

  function setError(msg) {
    var el = $('dr-error');
    if (!msg) {
      el.style.display = 'none';
      el.textContent = '';
      return;
    }
    el.style.display = '';
    el.textContent = msg;
  }

  function loadPlans() {
    return fetch('/api/disaster/plans').then(function (r) { return r.json(); }).then(function (data) {
      plans = Array.isArray(data) ? data : [];
      var ready = 0;
      plans.forEach(function (p) { if (p.ready) ready++; });
      $('dr-stat-systems').textContent = String(plans.length);
      $('dr-stat-ready').textContent = String(ready);
      renderSystemList();
    });
  }

  function loadAgents() {
    return fetch('/api/agents').then(function (r) { return r.json(); }).then(function (data) {
      agents = Array.isArray(data) ? data : [];
      var sel = $('dr-target-agent');
      var cur = sel.value;
      sel.innerHTML = '<option value="">Select registered agent…</option>';
      agents.forEach(function (a) {
        var name = a.name || a.Name;
        if (!name) return;
        var opt = document.createElement('option');
        opt.value = name;
        opt.textContent = name + (a.suspended ? ' (suspended)' : '');
        if (a.suspended) opt.disabled = true;
        sel.appendChild(opt);
      });
      if (cur) sel.value = cur;
      updateStartEnabled();
    });
  }

  function renderSystemList() {
    var ul = $('dr-system-list');
    ul.innerHTML = '';
    if (!plans.length) {
      ul.innerHTML = '<li class="text-muted">No protected systems with jobs/snapshots yet.</li>';
      return;
    }
    plans.forEach(function (p) {
      var li = document.createElement('li');
      li.className = 'disaster-system-item' + (p.source_agent === selected ? ' selected' : '');
      li.setAttribute('role', 'option');
      li.setAttribute('aria-selected', p.source_agent === selected ? 'true' : 'false');
      li.innerHTML =
        '<strong>' + escapeHtml(p.source_agent) + '</strong>' +
        '<span class="text-muted">' + p.ready_units + '/' + p.job_count + ' units · ' +
        formatBytes(p.total_size) + '</span>' +
        (p.ready ? '<span class="badge badge-ok">ready</span>' : '<span class="badge badge-stale">incomplete</span>');
      li.addEventListener('click', function () { selectSystem(p.source_agent); });
      ul.appendChild(li);
    });
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c];
    });
  }

  function selectSystem(name) {
    selected = name;
    selectedJobs = {};
    renderSystemList();
    $('dr-plan-empty').classList.add('hidden');
    $('dr-plan').classList.remove('hidden');
    setError('');
    fetch('/api/disaster/plans/' + encodeURIComponent(name))
      .then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t || r.statusText); });
        return r.json();
      })
      .then(function (data) {
        plan = data;
        renderPlan();
        updateStartEnabled();
      })
      .catch(function (err) {
        setError(err.message || String(err));
      });
  }

  function renderPlan() {
    if (!plan) return;
    var meta = $('dr-plan-meta');
    meta.innerHTML =
      '<p><strong>' + escapeHtml(plan.source_agent) + '</strong> · ' +
      plan.ready_units + ' restorable unit(s) · ' + formatBytes(plan.total_size) + ' · ' +
      (plan.total_files || 0) + ' files' +
      (plan.last_backup_str ? ' · last backup ' + escapeHtml(plan.last_backup_str) : '') +
      '</p>' +
      (plan.source_online_hint ? '<p class="text-muted">' + escapeHtml(plan.source_online_hint) + '</p>' : '');

    var warn = $('dr-warnings');
    if (plan.warnings && plan.warnings.length) {
      warn.classList.remove('hidden');
      warn.className = 'settings-banner settings-banner--warn';
      warn.innerHTML = '<ul style="margin:0;padding-left:1.2rem;">' +
        plan.warnings.map(function (w) { return '<li>' + escapeHtml(w) + '</li>'; }).join('') +
        '</ul>';
    } else {
      warn.classList.add('hidden');
      warn.innerHTML = '';
    }

    var body = $('dr-units-body');
    body.innerHTML = '';
    (plan.units || []).forEach(function (u) {
      var tr = document.createElement('tr');
      var checked = u.ready && selectedJobs[u.job_id] !== false;
      if (u.ready && selectedJobs[u.job_id] === undefined) selectedJobs[u.job_id || u.snapshot_id] = true;
      var key = u.job_id || u.snapshot_id;
      tr.innerHTML =
        '<td><input type="checkbox" data-unit="' + escapeHtml(key) + '" ' +
        (u.ready ? '' : 'disabled ') + (checked && u.ready ? 'checked' : '') + '></td>' +
        '<td>' + escapeHtml(u.job_name || '—') +
        (u.skip_reason ? '<div class="text-muted">' + escapeHtml(u.skip_reason) + '</div>' : '') + '</td>' +
        '<td><code>' + shortID(u.snapshot_id) + '</code>' +
        (u.snapshot_time_str ? '<div class="text-muted">' + escapeHtml(u.snapshot_time_str) + '</div>' : '') + '</td>' +
        '<td class="paths-cell">' + escapeHtml((u.paths || []).join(', ') || '—') + '</td>' +
        '<td>' + formatBytes(u.total_size) + '</td>' +
        '<td>' + (u.verify_status ? escapeHtml(u.verify_status) : '—') + '</td>';
      body.appendChild(tr);
      var cb = tr.querySelector('input[type=checkbox]');
      if (cb) {
        cb.addEventListener('change', function () {
          selectedJobs[key] = cb.checked;
          updateStartEnabled();
        });
      }
    });

    var cl = $('dr-checklist');
    cl.innerHTML = '';
    (plan.checklist || []).forEach(function (item) {
      var li = document.createElement('li');
      li.textContent = item;
      cl.appendChild(li);
    });

    // Default staging root suggestion
    var staging = $('dr-staging-root');
    if (!staging.value) {
      staging.value = '/recover/' + plan.source_agent;
    }
  }

  function currentMode() {
    var el = document.querySelector('input[name="dr-mode"]:checked');
    return el ? el.value : 'staging';
  }

  function updateStartEnabled() {
    var btn = $('dr-start-btn');
    var target = $('dr-target-agent').value;
    var mode = currentMode();
    var staging = $('dr-staging-root').value.trim();
    var anyJob = false;
    Object.keys(selectedJobs).forEach(function (k) {
      if (selectedJobs[k]) anyJob = true;
    });
    var ok = !!(plan && plan.ready_units > 0 && target && anyJob && (mode !== 'staging' || staging));
    btn.disabled = !ok;
    $('dr-staging-group').style.display = mode === 'staging' ? '' : 'none';
  }

  function startRecovery() {
    setError('');
    var jobIds = [];
    (plan.units || []).forEach(function (u) {
      var key = u.job_id || u.snapshot_id;
      if (u.ready && selectedJobs[key] && u.job_id) jobIds.push(u.job_id);
    });
    var body = {
      source_agent: plan.source_agent,
      target_agent: $('dr-target-agent').value,
      mode: currentMode(),
      staging_root: $('dr-staging-root').value.trim(),
      job_ids: jobIds
    };
    var btn = $('dr-start-btn');
    btn.disabled = true;
    btn.textContent = 'Starting…';
    fetch('/api/disaster/recover', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t || r.statusText); });
      return r.json();
    }).then(function () {
      btn.textContent = 'Start full-system recovery';
      pollRecoveries();
    }).catch(function (err) {
      setError(err.message || String(err));
      btn.textContent = 'Start full-system recovery';
      updateStartEnabled();
    });
  }

  function pollRecoveries() {
    fetch('/api/disaster/recoveries').then(function (r) { return r.json(); }).then(function (data) {
      var list = Array.isArray(data) ? data : [];
      var active = list.filter(function (x) { return x.status === 'running'; }).length;
      $('dr-stat-active').textContent = String(active);
      var box = $('dr-progress-list');
      var empty = $('dr-progress-empty');
      if (!list.length) {
        empty.style.display = '';
        box.innerHTML = '';
        return;
      }
      empty.style.display = 'none';
      box.innerHTML = list.map(function (rec) {
        var units = (rec.units || []).map(function (u) {
          return '<li><strong>' + escapeHtml(u.job_name || u.job_id) + '</strong>: ' +
            escapeHtml(u.status) +
            (u.restore_id ? ' <code>' + shortID(u.restore_id) + '</code>' : '') +
            (u.error ? ' — ' + escapeHtml(u.error) : '') + '</li>';
        }).join('');
        return '<div class="disaster-recovery-card">' +
          '<div><strong>' + escapeHtml(rec.source_agent) + ' → ' + escapeHtml(rec.target_agent) +
          '</strong> · ' + escapeHtml(rec.mode) + ' · <span class="badge ' +
          (rec.status === 'completed' ? 'badge-ok' : rec.status === 'failed' ? 'badge-stale' : 'badge-running') +
          '">' + escapeHtml(rec.status) + '</span></div>' +
          '<ul>' + units + '</ul></div>';
      }).join('');
    }).catch(function () { /* ignore poll errors */ });
  }

  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('input[name="dr-mode"]').forEach(function (el) {
      el.addEventListener('change', updateStartEnabled);
    });
    $('dr-target-agent').addEventListener('change', updateStartEnabled);
    $('dr-staging-root').addEventListener('input', updateStartEnabled);
    $('dr-start-btn').addEventListener('click', startRecovery);
    Promise.all([loadPlans(), loadAgents()]).then(function () {
      pollRecoveries();
      setInterval(pollRecoveries, 3000);
    });
  });
})();
