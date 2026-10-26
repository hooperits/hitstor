(function () {
    'use strict';

    var selectedAgent = '';
    var sourcesData = [];
    var repoFilter = '';
    var typeFilter = '';
    var agentRepos = [];
    var highlightSnapshot = '';
    var pollTimer = null;

    function formatBytes(b) {
        if (b >= 1099511627776) return (b / 1099511627776).toFixed(1) + ' TiB';
        if (b >= 1073741824) return (b / 1073741824).toFixed(1) + ' GiB';
        if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MiB';
        if (b >= 1024) return (b / 1024).toFixed(1) + ' KiB';
        return b + ' B';
    }

    function formatNumber(n) {
        if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
        if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
        return String(n);
    }

    function shortID(id) {
        return id && id.length > 8 ? id.substring(0, 8) : id;
    }

    function formatRelativeTime(iso) {
        if (!iso) return '';
        var d = new Date(iso);
        if (isNaN(d.getTime())) return iso;
        var secs = Math.floor((Date.now() - d.getTime()) / 1000);
        if (secs < 60) return 'just now';
        if (secs < 3600) return Math.floor(secs / 60) + 'm ago';
        if (secs < 86400) return Math.floor(secs / 3600) + 'h ago';
        if (secs < 604800) return Math.floor(secs / 86400) + 'd ago';
        return d.toLocaleString();
    }

    function escapeHTML(s) {
        var div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    function backupTypeLabel(bt) {
        return 'Files';
    }

    function sourceStatusClass(status, backupType) {
        if (status === 'ok') return 'badge-ok';
        if (status === 'stale' || status === 'suspended') return 'badge-stale';
        return 'badge-muted';
    }

    function loadSources() {
        var url = '/api/restores/sources';
        if (typeFilter) url += '?type=' + encodeURIComponent(typeFilter);
        return fetch(url)
            .then(function (r) { return r.json(); })
            .then(function (sources) {
                sourcesData = sources || [];
                renderSourceList();
                updateStats();
                var params = new URLSearchParams(window.location.search);
                var preselect = params.get('agent');
                var preType = params.get('type');
                var preSnapshot = params.get('snapshot');
                if (preType) {
                    typeFilter = preType;
                    var typeSel = document.getElementById('restores-type-filter');
                    if (typeSel) typeSel.value = preType;
                }
                if (preSnapshot) highlightSnapshot = preSnapshot;
                if (preselect) {
                    selectAgent(preselect);
                }
            })
            .catch(function () {
                document.getElementById('restores-agent-list').innerHTML =
                    '<li class="restores-agent-empty">Failed to load sources.</li>';
            });
    }

    function updateStats() {
        var totalSnaps = sourcesData.reduce(function (sum, s) {
            return sum + (s.snapshot_count || 0);
        }, 0);
        document.getElementById('restores-stat-agents').textContent = String(sourcesData.length);
        document.getElementById('restores-stat-snapshots').textContent = formatNumber(totalSnaps);
        var hint = document.getElementById('restores-toolbar-hint');
        if (hint) {
            if (sourcesData.length === 0) {
                hint.textContent = 'No recoverable snapshots yet';
            } else {
                hint.textContent = sourcesData.length + ' protected source' + (sourcesData.length === 1 ? '' : 's');
            }
        }
    }

    function renderSourceList(filter) {
        var list = document.getElementById('restores-agent-list');
        var q = (filter || '').toLowerCase().trim();
        var filtered = sourcesData.filter(function (s) {
            return !q || s.label.toLowerCase().indexOf(q) >= 0 || s.id.toLowerCase().indexOf(q) >= 0;
        });

        if (sourcesData.length === 0) {
            list.innerHTML =
                '<li class="restores-agent-empty">' +
                'No snapshots to restore yet. <a href="/backups">Create a backup job</a> first.' +
                '</li>';
            return;
        }

        if (filtered.length === 0) {
            list.innerHTML = '<li class="restores-agent-empty">No sources match your search.</li>';
            return;
        }

        list.innerHTML = filtered.map(function (s) {
            var active = s.id === selectedAgent ? ' restores-agent-item--active' : '';
            return (
                '<li class="restores-agent-item' + active + '" role="option" data-agent="' + escapeHTML(s.id) + '" tabindex="0">' +
                '<div class="restores-agent-item-top">' +
                '<span class="restores-agent-name">' + escapeHTML(s.label) + '</span>' +
                '<span class="badge ' + sourceStatusClass(s.status, s.backup_type) + '">' + escapeHTML(backupTypeLabel(s.backup_type)) + '</span>' +
                '</div>' +
                '<div class="restores-agent-meta">' +
                '<span>' + formatNumber(s.snapshot_count) + ' snapshots</span>' +
                '</div>' +
                '</li>'
            );
        }).join('');

        list.querySelectorAll('.restores-agent-item').forEach(function (el) {
            function activate() { selectAgent(el.getAttribute('data-agent')); }
            el.addEventListener('click', activate);
            el.addEventListener('keydown', function (e) {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    activate();
                }
            });
        });
    }

    function selectAgent(name) {
        selectedAgent = name;
        repoFilter = '';
        agentRepos = [];
        var repoSelect = document.getElementById('restores-repo-filter');
        if (repoSelect) repoSelect.value = '';

        renderSourceList(document.getElementById('restores-agent-search').value);

        var source = sourcesData.find(function (s) { return s.id === name; });
        var title = document.getElementById('restores-snapshots-title');
        if (title) title.textContent = 'Snapshots — ' + (source ? source.label : name);

        var params = new URLSearchParams(window.location.search);
        params.set('agent', name);
        if (typeFilter) params.set('type', typeFilter); else params.delete('type');
        if (highlightSnapshot) params.set('snapshot', highlightSnapshot); else params.delete('snapshot');
        history.replaceState(null, '', '/restores?' + params.toString());

        loadSnapshots(name);
    }

    function loadSnapshots(agentName) {
        var empty = document.getElementById('restores-snapshots-empty');
        var tableWrap = document.getElementById('restores-table-wrap');
        var tbody = document.getElementById('restores-tbody');

        empty.hidden = true;
        tableWrap.hidden = true;
        tbody.innerHTML = '<tr><td colspan="8">Loading snapshots...</td></tr>';
        tableWrap.hidden = false;

        var url = '/api/agents/' + encodeURIComponent(agentName) + '/snapshots?limit=100';
        if (repoFilter) url += '&repo=' + encodeURIComponent(repoFilter);
        if (typeFilter) url += '&type=' + encodeURIComponent(typeFilter);

        fetch(url)
            .then(function (r) { return r.json(); })
            .then(function (data) { renderSnapshots(data); })
            .catch(function () {
                tableWrap.hidden = true;
                empty.hidden = false;
                empty.querySelector('h3').textContent = 'Failed to load snapshots';
                empty.querySelector('p').textContent = 'Could not fetch snapshots for this source. Try again later.';
            });
    }

    function renderSnapshots(data) {
        var empty = document.getElementById('restores-snapshots-empty');
        var tableWrap = document.getElementById('restores-table-wrap');
        var tbody = document.getElementById('restores-tbody');
        var repoWrap = document.getElementById('restores-repo-filter-wrap');
        var repoSelect = document.getElementById('restores-repo-filter');

        var snaps = data.snapshots || [];
        if (!snaps.length) {
            tableWrap.hidden = true;
            empty.hidden = false;
            empty.querySelector('h3').textContent = 'No snapshots found';
            empty.querySelector('p').textContent = repoFilter
                ? 'No snapshots in the selected repository for this filter.'
                : 'No recoverable snapshots match the current filters.';
            if (repoWrap) repoWrap.hidden = true;
            return;
        }

        if (agentRepos.length === 0 && data.repos) {
            agentRepos = data.repos.slice();
        }
        if (repoWrap && agentRepos.length > 1) {
            repoWrap.hidden = false;
            var current = repoSelect.value;
            repoSelect.innerHTML = '<option value="">All repositories</option>' +
                agentRepos.map(function (r) {
                    return '<option value="' + escapeHTML(r) + '">' + escapeHTML(r) + '</option>';
                }).join('');
            repoSelect.value = current || '';
        } else if (repoWrap) {
            repoWrap.hidden = true;
        }

        tbody.innerHTML = snaps.map(function (s) {
            var snapType = s.type || 'full';
            var bt = s.backup_type || 'files';
            var rowClass = highlightSnapshot && (s.id === highlightSnapshot || s.id.indexOf(highlightSnapshot) === 0)
                ? ' class="restores-row-highlight"' : '';
            return (
                '<tr' + rowClass + '>' +
                '<td><code class="restores-snap-id">' + escapeHTML(shortID(s.id)) + '</code></td>' +
                '<td><span class="badge badge-muted">' + escapeHTML(s.repo_name || '') + '</span></td>' +
                '<td><span title="' + escapeHTML(s.time) + '">' + escapeHTML(formatRelativeTime(s.time)) + '</span></td>' +
                '<td><span class="badge badge-muted">' + escapeHTML(backupTypeLabel(bt)) + '</span></td>' +
                '<td><span class="badge badge-' + escapeHTML(snapType) + '">' + escapeHTML(snapType) + '</span></td>' +
                '<td>' + formatBytes(s.total_size || 0) + '</td>' +
                '<td>' + formatNumber(s.total_files || 0) + '</td>' +
                '<td class="restores-actions-cell">' +
                '<a href="/snapshots/' + encodeURIComponent(s.id) + '?restore=1" class="btn btn-sm">Restore</a>' +
                '</td>' +
                '</tr>'
            );
        }).join('');

        empty.hidden = true;
        tableWrap.hidden = false;

        if (highlightSnapshot) {
            var row = tbody.querySelector('.restores-row-highlight');
            if (row) row.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
        }
    }

    function loadActiveRestores() {
        fetch('/api/restores')
            .then(function (r) { return r.json(); })
            .then(function (sessions) {
                var running = (sessions || []).filter(function (s) {
                    return s.status === 'running';
                });

                var statCard = document.getElementById('restores-stat-active-card');
                var statVal = document.getElementById('restores-stat-active');
                var panel = document.getElementById('restores-active-panel');
                var list = document.getElementById('restores-active-list');

                if (running.length === 0) {
                    statCard.hidden = true;
                    panel.hidden = true;
                    return;
                }

                statCard.hidden = false;
                statVal.textContent = String(running.length);
                panel.hidden = false;

                list.innerHTML = running.map(function (s) {
                    var pct = s.files_total > 0
                        ? Math.min(100, Math.round((s.files_restored / s.files_total) * 100))
                        : 0;
                    return (
                        '<div class="restores-active-card">' +
                        '<div class="restores-active-top">' +
                        '<strong>' + escapeHTML(s.agent_name || 'Restore') + '</strong>' +
                        '<span class="badge badge-running">running</span>' +
                        '</div>' +
                        '<div class="restores-active-meta">' +
                        'Snapshot <code>' + escapeHTML(shortID(s.snapshot_id)) + '</code> — ' +
                        formatNumber(s.files_restored) + '/' + formatNumber(s.files_total) + ' files' +
                        '</div>' +
                        '<div class="progress-bar-container">' +
                        '<div class="progress-bar"><div class="progress-bar-fill" style="width:' + pct + '%"></div></div>' +
                        '<span class="progress-bar-label">' + pct + '%</span>' +
                        '</div>' +
                        '<a href="/snapshots/' + encodeURIComponent(s.snapshot_id) + '?restore=1" class="restores-active-link">Continue restore</a>' +
                        '</div>'
                    );
                }).join('');
            })
            .catch(function () {});
    }

    function init() {
        var search = document.getElementById('restores-agent-search');
        if (search) {
            search.addEventListener('input', function () {
                renderSourceList(search.value);
            });
        }

        var repoSelect = document.getElementById('restores-repo-filter');
        if (repoSelect) {
            repoSelect.addEventListener('change', function () {
                repoFilter = repoSelect.value;
                if (selectedAgent) loadSnapshots(selectedAgent);
            });
        }

        var typeSelect = document.getElementById('restores-type-filter');
        if (typeSelect) {
            typeSelect.addEventListener('change', function () {
                typeFilter = typeSelect.value;
                selectedAgent = '';
                agentRepos = [];
                highlightSnapshot = '';
                loadSources();
            });
        }

        loadSources();
        loadActiveRestores();
        pollTimer = setInterval(loadActiveRestores, 2000);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();