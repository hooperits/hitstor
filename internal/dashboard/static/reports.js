// Reports: download a report now, and the reports emailed on a schedule.
(function () {
    'use strict';

    var root = document.getElementById('reports-app');
    if (!root) return;

    var agents = JSON.parse(root.getAttribute('data-agents') || 'null') || [];
    var types = JSON.parse(root.getAttribute('data-types') || 'null') || [];
    var emailOn = root.getAttribute('data-email') === '1';
    var lang = root.getAttribute('data-lang') || 'en';
    var body = document.getElementById('rp-body');
    var list = [];

    function esc(s) {
        return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
    }

    function api(method, url, data) {
        var init = { method: method, headers: {} };
        if (data !== undefined) {
            init.headers['Content-Type'] = 'application/json';
            init.body = JSON.stringify(data);
        }
        return window.fetch(url, init).then(function (r) {
            if (!r.ok) return r.text().then(function (m) { throw new Error(m || r.statusText); });
            return r.json();
        });
    }

    function when(iso) {
        if (!iso) return '';
        return new Date(iso).toLocaleString(document.documentElement.lang || undefined,
            { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    }

    function dialog(title, html) {
        var overlay = document.createElement('div');
        overlay.className = 'dialog-overlay';
        overlay.style.display = 'flex';
        overlay.setAttribute('role', 'dialog');
        overlay.setAttribute('aria-modal', 'true');
        overlay.innerHTML = '<div class="dialog-box dialog-lg"><div class="dialog-header"><h2>' + esc(title) +
            '</h2><button type="button" class="dialog-close" aria-label="' + esc(t('Close')) + '">&times;</button></div>' +
            '<div class="dialog-body">' + html + '</div></div>';
        document.body.appendChild(overlay);
        var close = function () { overlay.remove(); };
        overlay.querySelector('.dialog-close').addEventListener('click', close);
        return { close: close, q: function (s) { return overlay.querySelector(s); }, qa: function (s) { return overlay.querySelectorAll(s); } };
    }

    function typeTitle(type) {
        for (var i = 0; i < types.length; i++) if (types[i].type === type) return types[i].title;
        return type;
    }

    // --- Download now ---
    root.querySelectorAll('.rp-type').forEach(function (card) {
        var type = card.getAttribute('data-type');
        var period = card.querySelector('.rp-period');
        function url(inline) {
            var p = new URLSearchParams({ type: type, lang: lang });
            if (period) p.set('period', period.value);
            if (inline) p.set('inline', '1');
            return '/api/reports/pdf?' + p.toString();
        }
        function refresh() {
            card.querySelector('.rp-view').href = url(true);
            card.querySelector('.rp-download').href = url(false);
        }
        if (period) period.addEventListener('change', refresh);
        refresh();
    });

    // --- Scheduled reports ---
    function status(r) {
        if (!r.enabled) return '<span class="badge badge-disabled">' + esc(t('Paused')) + '</span>';
        if (!r.last_sent) return '<span class="text-muted">' + esc(t('Not sent yet')) + '</span>';
        if (r.last_error) {
            return '<span class="badge badge-stale" title="' + esc(r.last_error) + '">' + esc(t('Not sent')) + '</span>' +
                '<div class="set-hint rp-error">' + esc(r.last_error) + '</div>';
        }
        return '<span class="badge badge-ok">' + esc(t('Sent')) + '</span> <span class="set-hint">' + esc(when(r.last_sent)) + '</span>';
    }

    function render() {
        if (!list.length) {
            body.innerHTML = '<tr><td colspan="5" class="rp-empty">' + esc(t('No scheduled reports yet. Create one to receive a report by email every day, week or month.')) + '</td></tr>';
            return;
        }
        body.innerHTML = list.map(function (r, i) {
            var scope = r.agents && r.agents.length ? t('Agents: {list}', { list: r.agents.join(', ') }) : t('All agents');
            var period = r.type === 'summary' ? (r.period === 'month' ? t('Last 30 days') : t('Last 7 days')) : t('Last 24 hours');
            return '<tr data-i="' + i + '">' +
                '<td><strong>' + esc(r.name || r.title) + '</strong><div class="set-hint">' + esc(r.title) + ' · ' + esc(period) + ' · ' + esc(scope) + '</div></td>' +
                '<td>' + esc(r.schedule) + (r.next_run ? '<div class="set-hint">' + esc(t('next: {when}', { when: when(r.next_run) })) + '</div>' : '') + '</td>' +
                '<td class="rp-recipients">' + r.recipients.map(esc).join('<br>') + '</td>' +
                '<td>' + status(r) + '</td>' +
                '<td class="rp-actions">' +
                '<button type="button" class="btn btn-secondary btn-sm rp-send"' + (emailOn ? '' : ' disabled title="' + esc(t('Email is not set up')) + '"') + '>' + esc(t('Send now')) + '</button> ' +
                '<a class="btn btn-secondary btn-sm" href="/api/reports/pdf?id=' + encodeURIComponent(r.id) + '&inline=1" target="_blank" rel="noopener">' + esc(t('View')) + '</a> ' +
                '<button type="button" class="btn btn-secondary btn-sm rp-edit">' + esc(t('Edit')) + '</button> ' +
                '<button type="button" class="btn btn-secondary btn-sm rp-delete" aria-label="' + esc(t('Delete')) + '">' + esc(t('Delete')) + '</button>' +
                '</td></tr>';
        }).join('');
    }

    function load() {
        return api('GET', '/api/reports').then(function (r) { list = r; render(); })
            .catch(function (e) { body.innerHTML = '<tr><td colspan="5" class="form-error">' + esc(e.message) + '</td></tr>'; });
    }

    body.addEventListener('click', function (ev) {
        var tr = ev.target.closest('tr[data-i]');
        if (!tr) return;
        var r = list[+tr.getAttribute('data-i')];
        if (ev.target.closest('.rp-edit')) editor(r);
        if (ev.target.closest('.rp-delete')) {
            if (!confirm(t('Delete the scheduled report {name}? Its recipients stop receiving it.', { name: r.name || r.title }))) return;
            api('DELETE', '/api/reports/' + encodeURIComponent(r.id)).then(load).catch(function (e) { alert(e.message); });
        }
        var send = ev.target.closest('.rp-send');
        if (send) {
            send.disabled = true;
            send.textContent = t('Sending...');
            api('POST', '/api/reports/' + encodeURIComponent(r.id) + '/send').then(function (res) {
                alert(t('Report sent to {to}.', { to: res.sent_to.join(', ') }));
            }).catch(function (e) { alert(e.message); }).then(load);
        }
    });

    var DEFAULT_CRON = { daily: '0 7 * * *', summary: '0 7 * * 1' };

    function editor(r) {
        var isNew = !r;
        r = r || { type: 'summary', period: 'week', enabled: true, recipients: [], agents: [], language: lang };
        var typeOptions = types.map(function (ty) {
            return '<label class="rp-choice"><input type="radio" name="rp-type" value="' + esc(ty.type) + '"' + (ty.type === r.type ? ' checked' : '') + '>' +
                '<span><strong>' + esc(ty.title) + '</strong><small>' + esc(ty.description) + '</small></span></label>';
        }).join('');
        var agentOptions = agents.map(function (a) {
            return '<label class="rp-agent"><input type="checkbox" value="' + esc(a) + '"' + (r.agents && r.agents.indexOf(a) >= 0 ? ' checked' : '') + '> ' + esc(a) + '</label>';
        }).join('');
        var d = dialog(isNew ? t('New scheduled report') : t('Edit scheduled report'),
            '<form class="rp-form">' +
            '<div class="form-group"><label>' + esc(t('Type of report')) + '</label><div class="rp-choices">' + typeOptions + '</div></div>' +
            '<div class="form-group rp-period-group"><label for="rp-period">' + esc(t('Period it covers')) + '</label>' +
            '<select id="rp-period"><option value="week">' + esc(t('Last 7 days')) + '</option><option value="month">' + esc(t('Last 30 days')) + '</option></select></div>' +
            '<div class="form-group"><label for="rp-name">' + esc(t('Name (optional)')) + '</label><input type="text" id="rp-name" maxlength="100" value="' + esc(r.name) + '" placeholder="' + esc(t('For example: Weekly report for management')) + '"></div>' +
            '<div class="form-group"><label for="rp-to">' + esc(t('Recipients')) + '</label><textarea id="rp-to" rows="3" placeholder="name@example.com">' + esc((r.recipients || []).join('\n')) + '</textarea>' +
            '<p class="form-hint">' + esc(t('One address per line.')) + '</p></div>' +
            '<div class="form-group"><label>' + esc(t('When to send it')) + '</label><div class="rp-picker"></div></div>' +
            '<div class="form-group"><label>' + esc(t('Agents')) + '</label><div class="rp-agents">' + (agentOptions || '<span class="text-muted">' + esc(t('No agents yet.')) + '</span>') + '</div>' +
            '<p class="form-hint">' + esc(t('Leave them all unticked to cover every agent, including the ones added later.')) + '</p></div>' +
            '<div class="form-group"><label for="rp-lang">' + esc(t('Language')) + '</label><select id="rp-lang"><option value="es">Español</option><option value="en">English</option></select></div>' +
            '<label class="rp-enabled"><input type="checkbox" id="rp-enabled"' + (r.enabled ? ' checked' : '') + '> ' + esc(t('Send it on schedule')) + '</label>' +
            '<div class="form-error" hidden></div>' +
            '<div class="dialog-actions"><button type="button" class="btn btn-secondary rp-cancel">' + esc(t('Cancel')) + '</button>' +
            '<button type="submit" class="btn">' + esc(isNew ? t('Create report') : t('Save changes')) + '</button></div></form>');
        d.q('#rp-period').value = r.period || 'week';
        d.q('#rp-lang').value = r.language || lang;
        var picker, touched = false;
        function mountPicker(type) {
            d.q('.rp-picker').innerHTML = '';
            picker = HitStorSchedule.mount(d.q('.rp-picker'), {
                cron: r.cron, timezone: r.timezone, defaultCron: DEFAULT_CRON[type] || DEFAULT_CRON.summary, repeatOnly: true
            });
        }
        mountPicker(r.type);
        d.q('.rp-picker').addEventListener('change', function () { touched = true; });
        function typeChanged() {
            var type = d.q('input[name=rp-type]:checked').value;
            d.q('.rp-period-group').hidden = type !== 'summary';
            // A new report starts from its type's usual schedule until the user picks one.
            if (isNew && !touched) mountPicker(type);
        }
        d.qa('input[name=rp-type]').forEach(function (el) { el.addEventListener('change', typeChanged); });
        d.q('.rp-period-group').hidden = r.type !== 'summary';
        d.q('.rp-cancel').addEventListener('click', d.close);
        d.q('form').addEventListener('submit', function (ev) {
            ev.preventDefault();
            var err = d.q('.form-error');
            err.hidden = true;
            try {
                var sched = picker.value();
                var data = {
                    name: d.q('#rp-name').value,
                    type: d.q('input[name=rp-type]:checked').value,
                    period: d.q('#rp-period').value,
                    cron: sched.cron_expression,
                    timezone: sched.timezone,
                    recipients: d.q('#rp-to').value.split(/[\s,;]+/).filter(Boolean),
                    agents: Array.prototype.map.call(d.qa('.rp-agents input:checked'), function (el) { return el.value; }),
                    language: d.q('#rp-lang').value,
                    enabled: d.q('#rp-enabled').checked
                };
            } catch (e) {
                err.textContent = e.message;
                err.hidden = false;
                return;
            }
            var req = isNew ? api('POST', '/api/reports', data) : api('PUT', '/api/reports/' + encodeURIComponent(r.id), data);
            req.then(function () { d.close(); load(); }).catch(function (e) {
                err.textContent = e.message;
                err.hidden = false;
            });
        });
        d.q('#rp-name').focus();
    }

    document.getElementById('rp-new').addEventListener('click', function () { editor(null); });
    load();
})();
