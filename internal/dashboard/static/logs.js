(function () {
  'use strict';

  var app = document.getElementById('logs-app');
  if (!app) return;

  var tbodyEl = document.getElementById('logs-tbody');
  var tablePanel = document.getElementById('logs-table-panel');
  var metaEl = document.getElementById('logs-meta');
  var warningsWrap = document.getElementById('logs-warnings-wrap');
  var warningsEl = document.getElementById('logs-warnings');
  var flashEl = document.getElementById('logs-flash');
  var sourceEl = document.getElementById('logs-source');
  var levelEl = document.getElementById('logs-level');
  var sinceEl = document.getElementById('logs-since');
  var queryEl = document.getElementById('logs-query');
  var queryClearEl = document.getElementById('logs-query-clear');
  var autoRefreshEl = document.getElementById('logs-auto-refresh');
  var followTailEl = document.getElementById('logs-follow-tail');
  var refreshBtn = document.getElementById('logs-refresh');
  var exportToggle = document.getElementById('logs-export-toggle');
  var exportMenu = document.getElementById('logs-export-menu');
  var countErrors = document.getElementById('logs-count-errors');
  var countWarns = document.getElementById('logs-count-warns');
  var countInfo = document.getElementById('logs-count-info');
  var countTotal = document.getElementById('logs-count-total');
  var statErrors = document.getElementById('logs-stat-errors');

  var refreshTimer = null;
  var searchTimer = null;
  var agentSources = [];
  var lastUpdated = null;
  var isLoading = false;
  var lastSources = { server: true, system: true, agents: [] };
  var lastEntries = [];
  var MESSAGE_TRUNCATE = 140;
  var jobIDRe = /^[a-f0-9]{16}$/;

  function byId(id) {
    return document.getElementById(id);
  }

  function escapeHTML(s) {
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function showFlash(message, isError) {
    if (!flashEl) return;
    flashEl.textContent = message;
    flashEl.classList.toggle('logs-flash--error', !!isError);
    flashEl.classList.remove('hidden');
    window.setTimeout(function () {
      flashEl.classList.add('hidden');
    }, 5000);
  }

  function readFiltersFromURL() {
    var params = new URLSearchParams(window.location.search);
    if (params.get('source')) sourceEl.value = params.get('source');
    if (params.get('level')) levelEl.value = params.get('level');
    if (params.get('since')) sinceEl.value = params.get('since');
    if (params.get('q')) queryEl.value = params.get('q');
    toggleQueryClear();
  }

  function syncURL() {
    var params = new URLSearchParams();
    if (sourceEl.value && sourceEl.value !== 'all') params.set('source', sourceEl.value);
    if (levelEl.value) params.set('level', levelEl.value);
    if (sinceEl.value) params.set('since', sinceEl.value);
    if (queryEl.value.trim()) params.set('q', queryEl.value.trim());
    var qs = params.toString();
    var next = window.location.pathname + (qs ? ('?' + qs) : '');
    window.history.replaceState({}, '', next);
  }

  function buildQuery(includeRotated) {
    var params = new URLSearchParams();
    if (sourceEl.value && sourceEl.value !== 'all') params.set('source', sourceEl.value);
    if (levelEl.value) params.set('level', levelEl.value);
    if (sinceEl.value) params.set('since', sinceEl.value);
    if (queryEl.value.trim()) params.set('q', queryEl.value.trim());
    params.set('limit', '500');
    if (includeRotated) params.set('include_rotated', '1');
    return params.toString();
  }

  function formatTime(iso) {
    if (!iso) return { rel: '—', abs: '—', datetime: '' };
    var d = new Date(iso);
    if (isNaN(d.getTime())) return { rel: '—', abs: '—', datetime: '' };
    var now = Date.now();
    var diff = Math.max(0, now - d.getTime());
    var rel;
    if (diff < 60000) rel = t('{n}s ago', {n: Math.floor(diff / 1000)});
    else if (diff < 3600000) rel = t('{n}m ago', {n: Math.floor(diff / 60000)});
    else if (diff < 86400000) rel = t('{n}h ago', {n: Math.floor(diff / 3600000)});
    else rel = t('{n}d ago', {n: Math.floor(diff / 86400000)});
    var abs = d.toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, ' UTC');
    return { rel: rel, abs: abs, datetime: d.toISOString() };
  }

  function levelBadgeClass(level) {
    var l = (level || 'info').toLowerCase();
    if (l === 'error') return 'badge-critical';
    if (l === 'warn' || l === 'warning') return 'badge-warning';
    if (l === 'debug') return 'badge-never';
    return 'badge-ok';
  }

  function sourceClass(source) {
    var s = (source || 'server').toLowerCase();
    if (s.indexOf('agent:') === 0) return 'logs-source--agent';
    if (s === 'system') return 'logs-source--system';
    return 'logs-source--server';
  }

  function sourceLabel(source) {
    var s = source || 'server';
    if (s.indexOf('agent:') === 0) return s.slice(6);
    if (s === 'server' || s === 'system') return t(s);
    return s;
  }

  function highlightQuery(text, query) {
    if (!query) return escapeHTML(text);
    var lower = text.toLowerCase();
    var q = query.toLowerCase();
    var idx = lower.indexOf(q);
    if (idx < 0) return escapeHTML(text);
    return (
      escapeHTML(text.slice(0, idx)) +
      '<mark class="logs-highlight">' +
      escapeHTML(text.slice(idx, idx + q.length)) +
      '</mark>' +
      escapeHTML(text.slice(idx + q.length))
    );
  }

  function jobLink(fields) {
    if (!fields) return '';
    var id = fields.job_id || fields.job;
    if (!id || !jobIDRe.test(id)) return '';
    return '/backups/' + encodeURIComponent(id);
  }

  function isNearBottom(el) {
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < 48;
  }

  function renderSummary(summary) {
    summary = summary || {};
    var by = summary.by_level || {};
    var errors = by.error || 0;
    var warns = by.warn || 0;
    var info = by.info || 0;
    if (countErrors) countErrors.textContent = String(errors);
    if (countWarns) countWarns.textContent = String(warns);
    if (countInfo) countInfo.textContent = String(info);
    if (countTotal) countTotal.textContent = String(summary.total || 0);
    if (statErrors) {
      statErrors.classList.toggle('stat-card--stale', errors > 0);
      var val = statErrors.querySelector('.stat-card-value');
      if (val) val.classList.toggle('stat-card-value--stale', errors > 0);
    }
  }

  function renderMeta(data) {
    var count = (data.entries || []).length;
    var retention = data.retention || {};
    var parts = [t('{n} entries', {n: count})];
    if (data.summary && data.summary.truncated) {
      parts.push(t('truncated at limit'));
    }
    if (lastUpdated) {
      parts.push(t('updated {time}', {time: lastUpdated.toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, ' UTC')}));
    }
    if (retention.log_max_entries) {
      parts.push(t('retention {n} in memory', {n: retention.log_max_entries}));
    }
    if (retention.log_file_enabled) {
      parts.push(t('file rotation {mb}MB × {files}', {mb: retention.log_rotate_max_mb, files: retention.log_rotate_max_files}));
    }
    metaEl.textContent = parts.join(' · ');
  }

  function renderWarnings(warnings) {
    if (!warnings || !warnings.length) {
      warningsWrap.classList.add('hidden');
      warningsEl.innerHTML = '';
      return;
    }
    warningsEl.innerHTML = warnings.map(function (w) {
      return '<li>' + escapeHTML(w) + '</li>';
    }).join('');
    warningsWrap.classList.remove('hidden');
  }

  function updateSourceOptions(sources) {
    sources = sources || {};
    lastSources = sources;
    var agents = sources.agents || [];
    var changed = agents.length !== agentSources.length;
    if (!changed) {
      for (var i = 0; i < agents.length; i++) {
        if (agents[i] !== agentSources[i]) changed = true;
      }
    }
    if (changed) {
      agentSources = agents.slice();
      var current = sourceEl.value;
      Array.from(sourceEl.querySelectorAll('option[data-agent]')).forEach(function (opt) {
        opt.remove();
      });
      agents.sort().forEach(function (name) {
        var opt = document.createElement('option');
        opt.value = 'agent:' + name;
        opt.textContent = t('Agent: {name}', {name: name});
        opt.setAttribute('data-agent', '1');
        sourceEl.appendChild(opt);
      });
      if (current) sourceEl.value = current;
    }

    var systemOpt = sourceEl.querySelector('option[value="system"]');
    if (systemOpt) systemOpt.disabled = !sources.system;
  }

  function emptyMessage() {
    var src = sourceEl.value;
    if (src === 'system' && !lastSources.system) {
      return t('System logs are not available on this platform.');
    }
    if (src && src.indexOf('agent:') === 0) {
      return t('No log entries from this agent for the current filters.');
    }
    return t('No log entries match the current filters.');
  }

  function renderEntries(entries) {
    var wasNearBottom = isNearBottom(tablePanel);
    var query = queryEl.value.trim();

    if (!entries.length) {
      tbodyEl.innerHTML = '<tr class="logs-row-empty"><td colspan="5">' + escapeHTML(emptyMessage()) + '</td></tr>';
      return;
    }

    var html = '';
    entries.forEach(function (e, idx) {
      var tm = formatTime(e.time);
      var src = e.source || 'server';
      var comp = e.component ? '/' + e.component : '';
      var msg = e.message || '';
      var truncated = msg.length > MESSAGE_TRUNCATE;
      var displayMsg = truncated ? msg.slice(0, MESSAGE_TRUNCATE) + '…' : msg;
      var fields = e.fields || {};
      var fieldKeys = Object.keys(fields).sort();
      var hasFields = fieldKeys.length > 0;
      var link = jobLink(fields);
      var rowId = 'logs-row-' + idx;

      html += '<tr class="logs-row" data-idx="' + idx + '">';
      html += '<td class="logs-col-time"><time datetime="' + escapeHTML(tm.datetime) + '" title="' + escapeHTML(tm.abs) + '">' + escapeHTML(tm.rel) + '</time></td>';
      html += '<td class="logs-col-level"><span class="badge ' + levelBadgeClass(e.level) + '">' + escapeHTML(t((e.level || 'info').toLowerCase()).toUpperCase()) + '</span></td>';
      html += '<td class="logs-col-source"><span class="logs-source ' + sourceClass(src) + '" title="' + escapeHTML(src + comp) + '">' + escapeHTML(sourceLabel(src)) + '</span></td>';
      html += '<td class="logs-col-message">';
      html += '<span class="logs-message" id="' + rowId + '-msg">' + highlightQuery(displayMsg, query) + '</span>';
      if (truncated) {
        html += ' <button type="button" class="logs-expand-btn" data-idx="' + idx + '" data-target="' + rowId + '-msg" aria-label="' + escapeHTML(t('Expand message')) + '">' + escapeHTML(t('more')) + '</button>';
      }
      if (link) {
        html += ' <a class="logs-job-link" href="' + escapeHTML(link) + '">' + escapeHTML(t('job')) + '</a>';
      }
      html += '</td>';
      html += '<td class="logs-col-actions">';
      if (hasFields) {
        html += '<button type="button" class="btn btn-secondary btn-sm logs-details-btn" data-details="' + rowId + '-details" aria-expanded="false">' + escapeHTML(t('Fields')) + '</button>';
      }
      html += '<button type="button" class="btn btn-secondary btn-sm logs-copy-line" data-idx="' + idx + '" title="' + escapeHTML(t('Copy line')) + '">' + escapeHTML(t('Copy')) + '</button>';
      html += '</td>';
      html += '</tr>';

      if (hasFields) {
        html += '<tr class="logs-row-details hidden" id="' + rowId + '-details"><td colspan="5"><div class="logs-fields">';
        fieldKeys.forEach(function (k) {
          var v = fields[k];
          html += '<button type="button" class="logs-field-chip" data-filter="' + escapeHTML(k + '=' + v) + '"><span class="logs-field-key">' + escapeHTML(k) + '</span>=<span class="logs-field-val">' + escapeHTML(v) + '</span></button>';
        });
        html += '</div></td></tr>';
      }
    });

    tbodyEl.innerHTML = html;
    lastEntries = entries;

    if (followTailEl && followTailEl.checked && wasNearBottom && tablePanel) {
      tablePanel.scrollTop = tablePanel.scrollHeight;
    }
  }

  function formatEntryLine(e) {
    var tm = formatTime(e.time);
    var level = (e.level || 'info').toUpperCase();
    var src = e.source || 'server';
    var comp = e.component ? '/' + e.component : '';
    var line = tm.abs + ' [' + level + '] [' + src + comp + '] ' + (e.message || '');
    if (e.fields) {
      Object.keys(e.fields).sort().forEach(function (k) {
        line += ' ' + k + '=' + e.fields[k];
      });
    }
    return line;
  }

  function setLoading(loading) {
    isLoading = loading;
    if (refreshBtn) {
      refreshBtn.disabled = loading;
      refreshBtn.textContent = loading ? t('Refreshing…') : t('Refresh');
    }
    if (loading && !tbodyEl.querySelector('.logs-row')) {
      tbodyEl.innerHTML = '<tr class="logs-row-empty"><td colspan="5">' + escapeHTML(t('Loading logs…')) + '</td></tr>';
    }
  }

  function loadLogs() {
    if (isLoading) return;
    syncURL();
    setLoading(true);
    fetch('/api/logs?' + buildQuery(false))
      .then(function (r) {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      })
      .then(function (data) {
        lastUpdated = new Date();
        updateSourceOptions(data.sources_available || {});
        renderSummary(data.summary);
        renderMeta(data);
        renderWarnings(data.warnings);
        renderEntries(data.entries || []);
      })
      .catch(function (err) {
        tbodyEl.innerHTML = '<tr class="logs-row-empty logs-row-error"><td colspan="5">' + escapeHTML(t('Failed to load logs: {error}', {error: err.message})) + '</td></tr>';
        showFlash(t('Failed to load logs: {error}', {error: err.message}), true);
      })
      .finally(function () {
        setLoading(false);
      });
  }

  function scheduleRefresh() {
    if (refreshTimer) window.clearInterval(refreshTimer);
    refreshTimer = null;
    if (autoRefreshEl && autoRefreshEl.checked) {
      refreshTimer = window.setInterval(loadLogs, 10000);
    }
  }

  function exportLogs(format) {
    exportMenu.classList.add('hidden');
    if (exportToggle) exportToggle.setAttribute('aria-expanded', 'false');
    var url = '/api/logs/export?' + buildQuery(true) + '&format=' + encodeURIComponent(format);
    window.location.href = url;
  }

  function toggleQueryClear() {
    if (!queryClearEl) return;
    var has = !!queryEl.value.trim();
    queryClearEl.classList.toggle('hidden', !has);
  }

  function debouncedSearch() {
    if (searchTimer) window.clearTimeout(searchTimer);
    searchTimer = window.setTimeout(loadLogs, 300);
  }

  tbodyEl.addEventListener('click', function (e) {
    var target = e.target;
    if (target.classList.contains('logs-expand-btn')) {
      var expandIdx = parseInt(target.getAttribute('data-idx'), 10);
      var entry = lastEntries[expandIdx];
      var id = target.getAttribute('data-target');
      var el = id ? document.getElementById(id) : null;
      if (el && entry) {
        el.textContent = entry.message || '';
        target.remove();
      }
      return;
    }
    if (target.classList.contains('logs-details-btn')) {
      var detailsId = target.getAttribute('data-details');
      var row = detailsId ? document.getElementById(detailsId) : null;
      if (row) {
        var open = row.classList.toggle('hidden');
        target.setAttribute('aria-expanded', open ? 'false' : 'true');
        target.textContent = open ? t('Fields') : t('Hide');
      }
      return;
    }
    if (target.classList.contains('logs-field-chip')) {
      queryEl.value = target.getAttribute('data-filter') || '';
      toggleQueryClear();
      loadLogs();
      return;
    }
    if (target.classList.contains('logs-copy-line')) {
      var copyIdx = parseInt(target.getAttribute('data-idx'), 10);
      var copyEntry = lastEntries[copyIdx];
      if (copyEntry && navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(formatEntryLine(copyEntry)).then(function () {
          showFlash(t('Copied log line'), false);
        });
      }
      return;
    }
  });

  readFiltersFromURL();
  loadLogs();
  scheduleRefresh();

  refreshBtn.addEventListener('click', loadLogs);
  [sourceEl, levelEl, sinceEl].forEach(function (el) {
    el.addEventListener('change', loadLogs);
  });
  queryEl.addEventListener('input', function () {
    toggleQueryClear();
    debouncedSearch();
  });
  queryEl.addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
      if (searchTimer) window.clearTimeout(searchTimer);
      loadLogs();
    }
  });
  if (queryClearEl) {
    queryClearEl.addEventListener('click', function () {
      queryEl.value = '';
      toggleQueryClear();
      loadLogs();
    });
  }
  if (autoRefreshEl) {
    autoRefreshEl.addEventListener('change', scheduleRefresh);
  }
  if (exportToggle && exportMenu) {
    exportToggle.addEventListener('click', function (e) {
      e.stopPropagation();
      var open = exportMenu.classList.toggle('hidden');
      exportToggle.setAttribute('aria-expanded', open ? 'false' : 'true');
    });
    document.addEventListener('click', function () {
      exportMenu.classList.add('hidden');
      exportToggle.setAttribute('aria-expanded', 'false');
    });
    exportMenu.querySelectorAll('[data-export]').forEach(function (btn) {
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        exportLogs(btn.getAttribute('data-export'));
      });
    });
  }
  byId('logs-copy-link').addEventListener('click', function () {
    syncURL();
    var url = window.location.href;
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(url).then(function () {
        showFlash(t('Link copied to clipboard'), false);
      });
    }
  });
})();