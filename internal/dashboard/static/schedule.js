// Schedule picker for backup jobs, modeled on the schedule sets of
// enterprise backup tools: a job is repeating or on demand, and a repeating
// job runs every day, on days of the week (optionally only some weeks of
// the month), on days of the month (including the last one), or every N
// minutes or hours within an optional window. It produces the job's cron
// expression and time zone; the server describes it and lists the next runs.
(function () {
    'use strict';

    function esc(s) {
        var div = document.createElement('div');
        div.textContent = s == null ? '' : String(s);
        return div.innerHTML;
    }

    function pad(n) { return (n < 10 ? '0' : '') + n; }

    // Monday first, as on a calendar; values are cron weekdays.
    var WEEKDAYS = [[1, 'Mon'], [2, 'Tue'], [3, 'Wed'], [4, 'Thu'], [5, 'Fri'], [6, 'Sat'], [0, 'Sun']];
    var WEEKS = [['1', '1st'], ['2', '2nd'], ['3', '3rd'], ['4', '4th'], ['5', '5th'], ['L', 'Last']];
    var MINUTE_STEPS = [5, 10, 15, 20, 30];
    var HOUR_STEPS = [1, 2, 3, 4, 6, 8, 12];

    function chips(name, items, multi) {
        return '<div class="sched-chips" role="group">' + items.map(function (it) {
            return '<label class="sched-chip"><input type="' + (multi ? 'checkbox' : 'radio') + '" name="' + name + '" value="' + esc(it[0]) + '"><span>' +
                esc(it[1]) + '</span></label>';
        }).join('') + '</div>';
    }

    function hourOptions(sel) {
        var out = '';
        for (var h = 0; h < 24; h++) out += '<option value="' + h + '"' + (h === sel ? ' selected' : '') + '>' + pad(h) + '</option>';
        return out;
    }

    // parse recognizes the expressions this picker writes, so an existing
    // job opens in the matching view. Anything else opens as advanced.
    function parse(cron) {
        var f = (cron || '').trim().split(/\s+/);
        if (f.length !== 5 || f[3] !== '*') return null;
        var min = f[0], hour = f[1], dom = f[2], dow = f[3 + 1];
        var num = /^\d+$/;
        var m;
        if ((m = /^\*\/(\d+)$/.exec(min)) && dom === '*') {
            var hr = /^(\d+)-(\d+)$/.exec(hour);
            if (hour !== '*' && !hr) return null;
            if (dow !== '*' && dow !== '1-5') return null;
            return { method: 'every', unit: 'minutes', step: +m[1], window: hr ? [+hr[1], +hr[2]] : null, weekdays: dow === '1-5' };
        }
        if (!num.test(min)) return null;
        if ((m = /^(?:\*|(\d+)-(\d+))\/(\d+)$/.exec(hour)) && dom === '*' && (dow === '*' || dow === '1-5')) {
            return { method: 'every', unit: 'hours', step: +m[3], minute: +min, window: m[1] ? [+m[1], +m[2]] : null, weekdays: dow === '1-5' };
        }
        if (!num.test(hour)) return null;
        var time = pad(+hour) + ':' + pad(+min);
        if (dom === '*' && dow === '*') return { method: 'daily', time: time };
        if (dom === '*') {
            var days = {}, weeks = {}, plain = false, nth = false;
            var ok = dow.split(',').every(function (it) {
                var x;
                if ((x = /^([0-6])#([1-5])$/.exec(it)) || (x = /^([0-6])(L)$/.exec(it))) {
                    days[x[1]] = true;
                    weeks[x[2]] = true;
                    nth = true;
                    return true;
                }
                if (/^[0-6]$/.test(it)) {
                    days[it] = true;
                    plain = true;
                    return true;
                }
                if (it === '1-5') {
                    [1, 2, 3, 4, 5].forEach(function (d) { days[d] = true; });
                    plain = true;
                    return true;
                }
                return false;
            });
            if (!ok || (plain && nth)) return null;
            // Week-of-month items must form a full grid of days × weeks.
            if (nth && Object.keys(days).length * Object.keys(weeks).length !== dow.split(',').length) return null;
            return { method: 'weekly', time: time, days: Object.keys(days), weeks: nth ? Object.keys(weeks) : [] };
        }
        if (dow === '*' && dom.split(',').every(function (d) { return d === 'L' || (/^\d+$/.test(d) && +d >= 1 && +d <= 31); })) {
            return { method: 'monthly', time: time, doms: dom.split(',') };
        }
        return null;
    }

    // mount renders the picker into el. opts.cron and opts.timezone set the
    // initial value (an empty cron means on demand unless opts.defaultCron
    // is given). Returns { value() } with schedule_enabled, cron_expression
    // and timezone, or throws an Error saying what to fix.
    function mount(host, opts) {
        opts = opts || {};
        // A fresh root per mount, so remounting leaves no stale listeners.
        var el = document.createElement('div');
        host.innerHTML = '';
        host.appendChild(el);
        var id = 'sched-' + Math.random().toString(36).slice(2, 8);
        var tz = opts.timezone || (Intl.DateTimeFormat().resolvedOptions().timeZone || '');
        var dom = [];
        for (var d = 1; d <= 31; d++) dom.push([String(d), String(d)]);
        dom.push(['L', t('Last day')]);

        el.innerHTML =
            '<div class="sched">' +
            '<div class="sched-seg" role="radiogroup">' +
            '<label><input type="radio" name="' + id + '-type" value="repeat" checked><span>' + esc(t('Repeating')) + '</span></label>' +
            '<label><input type="radio" name="' + id + '-type" value="manual"><span>' + esc(t('On demand only')) + '</span></label>' +
            '</div>' +
            '<div class="sched-repeat">' +
            '<div class="sched-seg sched-methods" role="radiogroup">' +
            [['daily', t('Every day')], ['weekly', t('Days of week')], ['monthly', t('Days of month')], ['every', t('Every…')]].map(function (m, i) {
                return '<label><input type="radio" name="' + id + '-method" value="' + m[0] + '"' + (i === 0 ? ' checked' : '') + '><span>' + esc(m[1]) + '</span></label>';
            }).join('') + '</div>' +
            '<div class="sched-pane" data-pane="weekly"><div class="sched-label">' + esc(t('On')) + '</div>' +
            chips(id + '-dow', WEEKDAYS.map(function (w) { return [String(w[0]), t(w[1])]; }), true) +
            '<div class="sched-label">' + esc(t('Weeks of the month')) + '</div>' +
            '<div class="sched-chips" role="group"><label class="sched-chip"><input type="checkbox" class="sched-allweeks" checked><span>' + esc(t('Every week')) + '</span></label>' +
            WEEKS.map(function (w) {
                return '<label class="sched-chip"><input type="checkbox" name="' + id + '-week" value="' + w[0] + '"><span>' + esc(t(w[1])) + '</span></label>';
            }).join('') + '</div></div>' +
            '<div class="sched-pane" data-pane="monthly"><div class="sched-label">' + esc(t('On days')) + '</div>' +
            '<div class="sched-dom">' + chips(id + '-dom', dom, true) + '</div>' +
            '<p class="form-hint">' + esc(t('Days a month does not have are skipped; use "Last day" for month ends.')) + '</p></div>' +
            '<div class="sched-pane" data-pane="every"><div class="sched-row">' + esc(t('Every')) +
            ' <select class="sched-step"></select> <select class="sched-unit"><option value="minutes">' + esc(t('minutes')) + '</option>' +
            '<option value="hours">' + esc(t('hours')) + '</option></select>' +
            '<span class="sched-hour-minute"> ' + esc(t('at minute')) + ' <select class="sched-minute"></select></span></div>' +
            '<div class="sched-row"><label class="sched-inline"><input type="checkbox" class="sched-window-on"> ' + esc(t('Only from')) + '</label> ' +
            '<select class="sched-from">' + hourOptions(8) + '</select> ' + esc(t('to')) + ' <select class="sched-to">' + hourOptions(18) + '</select> h</div>' +
            '<div class="sched-row"><label class="sched-inline"><input type="checkbox" class="sched-weekdays"> ' + esc(t('Monday to Friday only')) + '</label></div></div>' +
            '<div class="sched-row sched-time-row"><label>' + esc(t('Start time')) + ' <input type="time" class="sched-time" value="02:00" step="60" required></label></div>' +
            '<div class="sched-row"><label>' + esc(t('Time zone')) + ' <select class="sched-tz"><option value="">' + esc(t('Server local')) + '</option></select></label></div>' +
            '</div>' +
            '<div class="sched-advanced" hidden><label>' + esc(t('Cron expression')) + ' <input type="text" class="sched-cron" placeholder="0 2 * * *" spellcheck="false"></label></div>' +
            '<div class="sched-preview" aria-live="polite"></div>' +
            '<button type="button" class="sched-adv-toggle sched-link">' + esc(t('Edit as cron expression')) + '</button>' +
            '</div>';

        var q = function (s) { return el.querySelector(s); };
        var qa = function (s) { return Array.prototype.slice.call(el.querySelectorAll(s)); };
        var advanced = false;

        function radio(name) {
            var r = el.querySelector('input[name="' + id + '-' + name + '"]:checked');
            return r ? r.value : '';
        }
        function checked(name) {
            return qa('input[name="' + id + '-' + name + '"]:checked').map(function (i) { return i.value; });
        }
        function setChecked(name, values) {
            qa('input[name="' + id + '-' + name + '"]').forEach(function (i) { i.checked = values.indexOf(i.value) >= 0; });
        }

        function fillStep() {
            var unit = q('.sched-unit').value;
            var cur = +q('.sched-step').value || (unit === 'minutes' ? 15 : 4);
            var steps = unit === 'minutes' ? MINUTE_STEPS : HOUR_STEPS;
            if (steps.indexOf(cur) < 0) cur = steps[unit === 'minutes' ? 2 : 3];
            q('.sched-step').innerHTML = steps.map(function (s) { return '<option' + (s === cur ? ' selected' : '') + '>' + s + '</option>'; }).join('');
            q('.sched-hour-minute').hidden = unit !== 'hours';
        }
        var mins = '';
        for (var mm = 0; mm < 60; mm += 5) mins += '<option value="' + mm + '">' + pad(mm) + '</option>';
        q('.sched-minute').innerHTML = mins;
        fillStep();

        function layout() {
            var repeat = radio('type') === 'repeat';
            var method = radio('method');
            q('.sched-repeat').hidden = !repeat || advanced;
            q('.sched-advanced').hidden = !repeat || !advanced;
            q('.sched-adv-toggle').hidden = !repeat;
            q('.sched-adv-toggle').textContent = advanced ? t('Use the schedule builder') : t('Edit as cron expression');
            qa('.sched-pane').forEach(function (p) { p.hidden = p.getAttribute('data-pane') !== method; });
            q('.sched-time-row').hidden = method === 'every';
            var win = q('.sched-window-on').checked;
            q('.sched-from').disabled = !win;
            q('.sched-to').disabled = !win;
            qa('.sched-chip input').forEach(function (i) { i.closest('.sched-chip').classList.toggle('on', i.checked); });
        }

        // build returns the cron expression for the builder, or throws.
        function build() {
            var method = radio('method');
            if (method === 'every') {
                var step = +q('.sched-step').value;
                var days = q('.sched-weekdays').checked ? '1-5' : '*';
                var range = '*';
                if (q('.sched-window-on').checked) {
                    var from = +q('.sched-from').value, to = +q('.sched-to').value;
                    if (from > to) throw new Error(t('The window must end after it starts.'));
                    range = from + '-' + to;
                }
                if (q('.sched-unit').value === 'minutes') return '*/' + step + ' ' + range + ' * * ' + days;
                return q('.sched-minute').value + ' ' + (range === '*' ? '*' : range) + '/' + step + ' * * ' + days;
            }
            var time = q('.sched-time').value;
            if (!/^\d\d:\d\d$/.test(time)) throw new Error(t('Choose the start time.'));
            var hm = +time.slice(3) + ' ' + +time.slice(0, 2);
            if (method === 'daily') return hm + ' * * *';
            if (method === 'weekly') {
                var dows = checked('dow');
                if (!dows.length) throw new Error(t('Select at least one day.'));
                var weeks = q('.sched-allweeks').checked ? [] : checked('week');
                if (!q('.sched-allweeks').checked && !weeks.length) throw new Error(t('Select at least one week of the month.'));
                var items = [];
                dows.forEach(function (d) {
                    if (!weeks.length) items.push(d);
                    weeks.forEach(function (w) { items.push(w === 'L' ? d + 'L' : d + '#' + w); });
                });
                return hm + ' * * ' + items.join(',');
            }
            var doms = checked('dom');
            if (!doms.length) throw new Error(t('Select at least one day of the month.'));
            return hm + ' ' + doms.join(',') + ' * *';
        }

        function cronValue() {
            return advanced ? q('.sched-cron').value.trim() : build();
        }

        var seq = 0;
        function preview() {
            var box = q('.sched-preview');
            box.classList.remove('sched-preview-error');
            if (radio('type') === 'manual') {
                box.textContent = t('The job runs only when started with Backup Now.');
                return;
            }
            var expr;
            try {
                expr = cronValue();
            } catch (err) {
                box.textContent = err.message;
                box.classList.add('sched-preview-error');
                return;
            }
            if (!expr) {
                box.textContent = '';
                return;
            }
            var mine = ++seq;
            var url = '/api/backups/validate-cron?expr=' + encodeURIComponent(expr);
            if (q('.sched-tz').value) url += '&tz=' + encodeURIComponent(q('.sched-tz').value);
            fetch(url).then(function (r) { return r.json(); }).then(function (data) {
                if (mine !== seq) return;
                if (!data.valid) {
                    box.textContent = t('Invalid expression') + (data.error ? ': ' + data.error : '');
                    box.classList.add('sched-preview-error');
                    return;
                }
                var next = (data.next_3 || []).map(function (s) {
                    var dt = new Date(s);
                    return dt.toLocaleString(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
                });
                box.innerHTML = '<strong>' + esc(data.description) + '</strong>' +
                    (next.length ? '<span class="sched-next">' + esc(t('Next runs')) + ': ' + next.map(esc).join(' · ') + '</span>' : '');
            }).catch(function () { if (mine === seq) box.textContent = ''; });
        }

        // load sets the picker from a cron expression.
        function load(cron) {
            var p = parse(cron);
            if (!p) {
                advanced = true;
                q('.sched-cron').value = cron;
                return;
            }
            setChecked('method', [p.method]);
            if (p.time) q('.sched-time').value = p.time;
            if (p.method === 'weekly') {
                setChecked('dow', p.days);
                q('.sched-allweeks').checked = !p.weeks.length;
                setChecked('week', p.weeks);
            } else if (p.method === 'monthly') {
                setChecked('dom', p.doms);
            } else if (p.method === 'every') {
                q('.sched-unit').value = p.unit;
                fillStep();
                q('.sched-step').value = String(p.step);
                if (p.unit === 'hours') q('.sched-minute').value = String(p.minute - p.minute % 5);
                q('.sched-window-on').checked = !!p.window;
                if (p.window) {
                    q('.sched-from').value = String(p.window[0]);
                    q('.sched-to').value = String(p.window[1]);
                }
                q('.sched-weekdays').checked = p.weekdays;
            }
        }

        // Sensible starting points for each method.
        setChecked('dow', ['0']);
        setChecked('dom', ['1']);
        var initial = opts.cron || opts.defaultCron || '';
        if (initial) {
            load(initial);
        } else {
            setChecked('type', ['manual']);
        }

        fetch('/api/timezones').then(function (r) { return r.json(); }).then(function (zones) {
            var sel = q('.sched-tz');
            (zones || []).forEach(function (z) {
                var o = document.createElement('option');
                o.value = z;
                o.textContent = z;
                sel.appendChild(o);
            });
            if (tz && zones.indexOf(tz) < 0) {
                var o = document.createElement('option');
                o.value = tz;
                o.textContent = tz;
                sel.appendChild(o);
            }
            sel.value = tz;
            preview();
        }).catch(function () {});

        el.addEventListener('change', function (ev) {
            var tgt = ev.target;
            if (tgt.classList.contains('sched-unit')) fillStep();
            if (tgt.classList.contains('sched-allweeks') && tgt.checked) setChecked('week', []);
            if (tgt.name === id + '-week') q('.sched-allweeks').checked = !checked('week').length;
            layout();
            preview();
        });
        el.addEventListener('input', function (ev) {
            if (ev.target.classList.contains('sched-cron') || ev.target.classList.contains('sched-time')) preview();
        });
        q('.sched-adv-toggle').addEventListener('click', function () {
            if (!advanced) {
                try {
                    q('.sched-cron').value = build();
                } catch (err) { /* start from an empty expression */ }
                advanced = true;
            } else {
                var p = parse(q('.sched-cron').value);
                if (!p && q('.sched-cron').value.trim()) {
                    var box = q('.sched-preview');
                    box.textContent = t('This expression cannot be shown in the builder.');
                    box.classList.add('sched-preview-error');
                    return;
                }
                if (p) load(q('.sched-cron').value);
                advanced = false;
            }
            layout();
            preview();
        });
        layout();
        preview();

        return {
            value: function () {
                if (radio('type') === 'manual') return { schedule_enabled: false, cron_expression: '', timezone: '' };
                var expr = cronValue();
                if (!expr) throw new Error(t('Enter a cron expression.'));
                return { schedule_enabled: true, cron_expression: expr, timezone: q('.sched-tz').value };
            }
        };
    }

    window.HitStorSchedule = { mount: mount, parse: parse };
})();
