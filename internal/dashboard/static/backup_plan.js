// Backup plan editor. A workload job backs up one selection with several
// kinds, each on its own schedule, as database backup tools do: weekly
// fulls, daily differentials, frequent log backups. Each kind is a row
// with an on/off switch and a schedule; below, the next runs of the whole
// plan in order.
(function () {
    'use strict';

    function esc(s) {
        return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
    }

    // KINDS describes the kinds plugins commonly offer; others get a
    // generic row.
    var KINDS = {
        full: { cron: '0 1 * * 0', on: true },
        // Weekday lists, not ranges: the schedule builder shows those.
        differential: { cron: '0 1 * * 1,2,3,4,5,6', on: true },
        log: { cron: '*/15 * * * *', on: false }
    };
    var ORDER = ['full', 'differential', 'log'];

    function label(kind) {
        return { full: t('Full'), differential: t('Differential'), log: t('Transaction log') }[kind] || kind;
    }

    function describe(kind) {
        return {
            full: t('A complete copy of the databases. Differentials and the log chain start from it.'),
            differential: t("Only what changed since this job's last full backup: smaller and faster, and it needs that full to restore."),
            log: t('Every change since the previous log backup, to restore to an exact minute. The database must use the FULL recovery model.')
        }[kind] || '';
    }

    // badge is the kind's short mark: F, D, L.
    function badge(kind) {
        var letter = { full: 'F', differential: 'D', log: 'L' }[kind] || kind.charAt(0).toUpperCase();
        return '<span class="bp-badge bp-badge--' + esc(kind) + '" aria-hidden="true">' + esc(letter) + '</span>';
    }

    function sortKinds(kinds) {
        return kinds.slice().sort(function (a, b) {
            var x = ORDER.indexOf(a), y = ORDER.indexOf(b);
            return (x < 0 ? 9 : x) - (y < 0 ? 9 : y);
        });
    }

    // mount renders the editor into host for the plugin's kinds. plan holds
    // the job's entries ({kind, enabled, cron_expression, timezone}), or
    // nothing for the usual defaults. Returns { value() } with the plan, or
    // throws an Error saying what to fix.
    function mount(host, opts) {
        var kinds = sortKinds(opts.kinds && opts.kinds.length ? opts.kinds : ['full']);
        var existing = {};
        (opts.plan || []).forEach(function (e) { existing[e.kind] = e; });
        host.innerHTML = '<div class="bp">' + kinds.map(function (k) {
            return '<section class="bp-kind" data-kind="' + esc(k) + '">' +
                '<div class="bp-head">' + badge(k) +
                '<div class="bp-title"><strong>' + esc(label(k)) + '</strong>' +
                (describe(k) ? '<span class="bp-desc">' + esc(describe(k)) + '</span>' : '') + '</div>' +
                '<label class="bp-switch"><input type="checkbox" class="bp-on" aria-label="' + esc(t('Schedule {kind} backups', { kind: label(k) })) + '"><span></span></label>' +
                '</div>' +
                '<div class="bp-when"><span class="bp-summary"></span>' +
                '<button type="button" class="bp-edit">' + esc(t('Change')) + '</button></div>' +
                '<div class="bp-picker" hidden></div>' +
                '</section>';
        }).join('') +
            '<div class="bp-warning" hidden></div>' +
            '<div class="bp-upcoming"><div class="bp-upcoming-title">' + esc(t('Next runs of the plan')) + '</div><ol class="bp-runs"></ol></div>' +
            '</div>';

        var rows = {};
        kinds.forEach(function (k) {
            var sec = host.querySelector('.bp-kind[data-kind="' + k + '"]');
            var e = existing[k];
            var def = KINDS[k] || { cron: '0 2 * * *', on: k === kinds[0] };
            var on = e ? !!e.enabled : (opts.plan && opts.plan.length ? false : def.on);
            var picker = HitStorSchedule.mount(sec.querySelector('.bp-picker'), {
                cron: e && e.cron_expression, defaultCron: def.cron, timezone: e && e.timezone, repeatOnly: true
            });
            rows[k] = { sec: sec, picker: picker };
            sec.querySelector('.bp-on').checked = on;
            sec.querySelector('.bp-edit').addEventListener('click', function () {
                var p = sec.querySelector('.bp-picker');
                p.hidden = !p.hidden;
                this.textContent = p.hidden ? t('Change') : t('Done');
            });
        });

        var seq = 0;
        var timer = null;
        function refresh() {
            clearTimeout(timer);
            timer = setTimeout(draw, 250);
        }

        // draw updates each row's summary and the plan's next runs.
        function draw() {
            var mine = ++seq;
            var lookups = kinds.map(function (k) {
                var r = rows[k];
                var on = r.sec.querySelector('.bp-on').checked;
                r.sec.classList.toggle('bp-kind--off', !on);
                r.sec.querySelector('.bp-edit').hidden = !on;
                if (!on) {
                    r.sec.querySelector('.bp-picker').hidden = true;
                    r.sec.querySelector('.bp-edit').textContent = t('Change');
                    r.sec.querySelector('.bp-summary').textContent = t('On demand only');
                    return Promise.resolve(null);
                }
                var v;
                try {
                    v = r.picker.value();
                } catch (err) {
                    r.sec.querySelector('.bp-summary').textContent = err.message;
                    return Promise.resolve(null);
                }
                var url = '/api/backups/validate-cron?expr=' + encodeURIComponent(v.cron_expression) + (v.timezone ? '&tz=' + encodeURIComponent(v.timezone) : '');
                return fetch(url).then(function (res) { return res.json(); }).then(function (d) {
                    if (mine !== seq) return null;
                    r.sec.querySelector('.bp-summary').textContent = d.valid ? d.description : t('Invalid expression');
                    return d.valid ? { kind: k, next: d.next_3 || [] } : null;
                }).catch(function () { return null; });
            });
            var on = {};
            kinds.forEach(function (k) { on[k] = rows[k].sec.querySelector('.bp-on').checked; });
            var warn = host.querySelector('.bp-warning');
            var needsFull = kinds.indexOf('full') >= 0 && !on.full && (on.differential || on.log);
            warn.hidden = !needsFull;
            warn.textContent = needsFull ? t('Without scheduled full backups, differential and log backups fail until you run a full backup by hand.') : '';

            Promise.all(lookups).then(function (res) {
                if (mine !== seq) return;
                var runs = [];
                res.forEach(function (r) {
                    if (r) r.next.forEach(function (n) { runs.push({ kind: r.kind, at: new Date(n) }); });
                });
                runs.sort(function (a, b) { return a.at - b.at || ORDER.indexOf(a.kind) - ORDER.indexOf(b.kind); });
                // Frequent log backups would crowd out the rest: list the next one only.
                var logShown = false;
                runs = runs.filter(function (r) {
                    if (r.kind !== 'log') return true;
                    if (logShown) return false;
                    return (logShown = true);
                });
                var fulls = {};
                runs.forEach(function (r) { if (r.kind === 'full') fulls[+r.at] = true; });
                var list = host.querySelector('.bp-runs');
                if (!runs.length) {
                    list.innerHTML = '<li class="bp-run bp-run--none">' + esc(t('Nothing runs on its own: every kind is on demand.')) + '</li>';
                    return;
                }
                list.innerHTML = runs.slice(0, 6).map(function (r) {
                    // A full at the same time holds what a differential would.
                    var covered = r.kind === 'differential' && fulls[+r.at];
                    return '<li class="bp-run' + (covered ? ' bp-run--covered' : '') + '">' + badge(r.kind) +
                        '<span class="bp-run-kind">' + esc(label(r.kind)) + '</span>' +
                        '<time>' + esc(r.at.toLocaleString(document.documentElement.lang || undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })) + '</time>' +
                        (covered ? '<span class="bp-run-note">' + esc(t('covered by the full')) + '</span>' : '') + '</li>';
                }).join('');
            });
        }
        host.addEventListener('change', refresh);
        host.addEventListener('input', refresh);
        draw();

        return {
            value: function () {
                return kinds.map(function (k) {
                    var r = rows[k];
                    var on = r.sec.querySelector('.bp-on').checked;
                    var v;
                    try {
                        v = r.picker.value();
                    } catch (err) {
                        if (on) throw new Error(label(k) + ': ' + err.message);
                        v = { cron_expression: '', timezone: '' };
                    }
                    return { kind: k, enabled: on, cron_expression: v.cron_expression, timezone: v.timezone };
                });
            }
        };
    }

    window.HitStorPlan = { mount: mount, label: label, badge: badge, sortKinds: sortKinds };
})();
