// Bare Metal tab on the Disaster Recovery page.
//
// Recovery media is built on the protected host and cached here, so this view
// is mostly about two questions an operator needs answered before a disaster:
// can each host build media, and how old is the image we hold for it.
(function () {
  'use strict';

  var hosts = [];
  var buildSource = null;

  function $(id) { return document.getElementById(id); }

  function escapeHtml(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function formatBytes(b) {
    b = Number(b) || 0;
    if (b >= 1073741824) return (b / 1073741824).toFixed(1) + ' GiB';
    if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MiB';
    if (b >= 1024) return (b / 1024).toFixed(1) + ' KiB';
    return b + ' B';
  }

  function formatWhen(iso) {
    if (!iso) return '—';
    var d = new Date(iso);
    return isNaN(d.getTime()) ? '—' : d.toLocaleString();
  }

  function setError(msg) {
    var el = $('bmr-error');
    if (!el) return;
    el.style.display = msg ? '' : 'none';
    el.textContent = msg || '';
  }

  // --- Tabs -----------------------------------------------------------------

  function showTab(name) {
    var bare = name === 'baremetal';
    $('dr-panel-guided').classList.toggle('hidden', bare);
    $('dr-panel-baremetal').classList.toggle('hidden', !bare);
    $('dr-tab-guided').classList.toggle('active', !bare);
    $('dr-tab-baremetal').classList.toggle('active', bare);
    $('dr-tab-guided').setAttribute('aria-selected', String(!bare));
    $('dr-tab-baremetal').setAttribute('aria-selected', String(bare));
    if (bare) { loadHosts(); loadTokens(); }
  }

  // --- Host readiness -------------------------------------------------------

  // readinessCell answers "can this host build media, and if not, why".
  // An unreachable host stays in the table saying so: dropping it would hide
  // exactly the host that needs attention.
  function readinessCell(h) {
    if (!h.reachable) {
      return '<span class="badge badge-stale">Unknown</span>' +
        '<div class="text-muted">' + escapeHtml(h.error || 'agent did not answer') + '</div>';
    }
    if (h.ready) {
      var version = h.rear_version ? '<div class="text-muted">' + escapeHtml(h.rear_version) + '</div>' : '';
      return '<span class="badge badge-ok">Ready</span>' + version;
    }
    var blockers = (h.blockers || []).map(function (b) {
      return '<li>' + escapeHtml(b) + '</li>';
    }).join('');
    return '<span class="badge badge-stale">Not ready</span>' +
      (blockers ? '<ul class="text-muted">' + blockers + '</ul>' : '');
  }

  // mediaCell shows the image's age, since a stale one recreates the old disk
  // layout and is the most common way this feature quietly fails.
  function mediaCell(h) {
    if (!h.has_media) {
      return '<span class="text-muted">Not built</span>';
    }
    var age = Number(h.media_age_days) || 0;
    var ageClass = age >= 90 ? 'badge-stale' : 'badge-ok';
    var ageText = age === 0 ? 'today' : age + ' day' + (age === 1 ? '' : 's') + ' old';
    return '<span class="badge ' + ageClass + '">' + escapeHtml(ageText) + '</span>' +
      '<div class="text-muted">' + formatBytes(h.media_size_bytes) +
      ' · ' + escapeHtml(formatWhen(h.media_built_at)) + '</div>';
  }

  function actionsCell(h) {
    var agent = escapeHtml(h.agent);
    var buildLabel = h.has_media ? 'Rebuild' : 'Build media';
    var buttons =
      '<button type="button" class="btn btn-sm" data-bmr-build="' + agent + '"' +
      (h.building ? ' disabled' : '') + '>' +
      (h.building ? 'Building…' : buildLabel) + '</button> ';

    if (h.has_media) {
      buttons +=
        '<a class="btn btn-sm" href="/api/bmr/media/' + encodeURIComponent(h.agent) + '/download">Download</a> ' +
        '<button type="button" class="btn btn-sm btn-danger" data-bmr-delete="' + agent + '">Delete</button> ';
    }
    buttons += '<button type="button" class="btn btn-sm" data-bmr-prepare="' + agent + '">Issue token</button>';
    return buttons;
  }

  function renderHosts() {
    var body = $('bmr-hosts-body');
    if (!body) return;

    if (!hosts.length) {
      body.innerHTML = '<tr><td colspan="4" class="text-muted">' +
        'No agents registered. Add one from the Agents page first.</td></tr>';
      return;
    }

    body.innerHTML = hosts.map(function (h) {
      var name = '<strong>' + escapeHtml(h.agent) + '</strong>';
      if (h.recovering) {
        name += ' <span class="badge badge-running">Recovering</span>';
      }
      if (h.addr) {
        name += '<div class="text-muted">' + escapeHtml(h.addr) + '</div>';
      }
      return '<tr>' +
        '<td>' + name + '</td>' +
        '<td>' + readinessCell(h) + '</td>' +
        '<td>' + mediaCell(h) + '</td>' +
        '<td>' + actionsCell(h) + '</td>' +
        '</tr>';
    }).join('');
  }

  function loadHosts() {
    // Readiness is probed live on every agent, so this is slower than a
    // normal list load; show what we had rather than blanking the table.
    return fetch('/api/bmr/hosts')
      .then(function (r) { return r.json(); })
      .then(function (data) {
        hosts = Array.isArray(data) ? data : [];
        renderHosts();
      })
      .catch(function (err) { setError('Could not load host readiness: ' + err); });
  }

  // --- Media build ----------------------------------------------------------

  function appendLog(line) {
    var log = $('bmr-build-log');
    log.textContent += line + '\n';
    log.scrollTop = log.scrollHeight;
  }

  function buildMedia(agent) {
    setError('');
    if (!window.confirm(
      'Build recovery media on ' + agent + '?\n\n' +
      'This runs ReaR on the protected host and reads every disk on it. ' +
      'It takes several minutes and adds load to a machine that may be in production.')) {
      return;
    }

    $('bmr-build-section').classList.remove('hidden');
    $('bmr-build-host').textContent = 'Building recovery media on ' + agent + '…';
    $('bmr-build-log').textContent = '';

    fetch('/api/bmr/media/' + encodeURIComponent(agent) + '/build', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({})
    }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
      return r.json();
    }).then(function (data) {
      followBuild(data.operation_id, agent);
      loadHosts();
    }).catch(function (err) {
      setError('Could not start the build: ' + err.message);
      $('bmr-build-host').textContent = '';
    });
  }

  function followBuild(opID, agent) {
    if (buildSource) buildSource.close();
    buildSource = new EventSource('/api/operations/' + opID + '/events');

    buildSource.onmessage = function (e) { appendLog(e.data); };
    buildSource.addEventListener('complete', function (e) {
      buildSource.close();
      buildSource = null;

      var data = {};
      try { data = JSON.parse(e.data); } catch (err) { /* keep the log as-is */ }

      if (data.status === 'completed') {
        $('bmr-build-host').textContent = 'Recovery media for ' + agent + ' is ready to download.';
      } else {
        $('bmr-build-host').textContent = 'The build on ' + agent + ' failed.';
        if (data.result && data.result.error) setError(data.result.error);
      }
      loadHosts();
    });
    buildSource.onerror = function () {
      // The stream ends when the operation does; a lost connection mid-build
      // should not look like a failed build.
      if (buildSource) { buildSource.close(); buildSource = null; }
      loadHosts();
    };
  }

  function deleteMedia(agent) {
    if (!window.confirm('Delete the cached recovery media for ' + agent + '?\n\n' +
      'You will not be able to recover this host from bare metal until it is rebuilt — ' +
      'which needs the host itself to be running.')) {
      return;
    }
    fetch('/api/bmr/media/' + encodeURIComponent(agent), { method: 'DELETE' })
      .then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        loadHosts();
      })
      .catch(function (err) { setError('Could not delete the media: ' + err.message); });
  }

  // --- Recovery tokens ------------------------------------------------------

  function prepareRecovery(agent) {
    setError('');
    fetch('/api/bmr/prepare/' + encodeURIComponent(agent), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({})
    }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
      return r.json();
    }).then(function (t) {
      // Shown once, and never readable again: prompt() keeps it selectable so
      // an operator can copy it before it is gone.
      window.prompt(
        'Recovery token for ' + agent + '\n\n' +
        'Type this at the rescue console. It works once, only for this host, ' +
        'and expires ' + formatWhen(t.expires_at) + '.\n\n' +
        'Copy it now — it cannot be shown again.',
        t.token);
      loadTokens();
    }).catch(function (err) {
      setError('Could not issue a recovery token: ' + err.message);
    });
  }

  function revokeToken(id) {
    fetch('/api/bmr/tokens/' + encodeURIComponent(id), { method: 'DELETE' })
      .then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        loadTokens();
      })
      .catch(function (err) { setError('Could not revoke the token: ' + err.message); });
  }

  function loadTokens() {
    return fetch('/api/bmr/tokens')
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var tokens = Array.isArray(data) ? data : [];
        var body = $('bmr-tokens-body');
        if (!body) return;

        if (!tokens.length) {
          body.innerHTML = '<tr><td colspan="4" class="text-muted">None outstanding.</td></tr>';
          return;
        }
        body.innerHTML = tokens.map(function (t) {
          return '<tr>' +
            '<td>' + escapeHtml(t.agent) + '</td>' +
            '<td>' + escapeHtml(formatWhen(t.created_at)) + '</td>' +
            '<td>' + escapeHtml(formatWhen(t.expires_at)) + '</td>' +
            '<td><button type="button" class="btn btn-sm btn-danger" data-bmr-revoke="' +
            escapeHtml(t.id) + '">Revoke</button></td>' +
            '</tr>';
        }).join('');
      })
      .catch(function () { /* the token list is not worth an error banner */ });
  }

  // --- Wiring ---------------------------------------------------------------

  document.addEventListener('DOMContentLoaded', function () {
    if (!$('dr-tab-baremetal')) return;

    $('dr-tab-guided').addEventListener('click', function () { showTab('guided'); });
    $('dr-tab-baremetal').addEventListener('click', function () { showTab('baremetal'); });

    // One delegated listener, so rows re-rendered after every refresh stay
    // live without rebinding.
    document.addEventListener('click', function (e) {
      var el = e.target.closest ? e.target.closest('[data-bmr-build],[data-bmr-delete],[data-bmr-prepare],[data-bmr-revoke]') : null;
      if (!el) return;
      if (el.dataset.bmrBuild) { buildMedia(el.dataset.bmrBuild); }
      else if (el.dataset.bmrDelete) { deleteMedia(el.dataset.bmrDelete); }
      else if (el.dataset.bmrPrepare) { prepareRecovery(el.dataset.bmrPrepare); }
      else if (el.dataset.bmrRevoke) { revokeToken(el.dataset.bmrRevoke); }
    });

    if (window.location.hash === '#baremetal') showTab('baremetal');
  });
})();
