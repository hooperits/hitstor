// Backup job wizard and selection editor. Like the backup jobs of
// enterprise backup tools, a job starts from one tree: machines, their
// sources (the file system and each installed plugin), and what is in
// them. A job backs up one source of one machine; options, repository and
// schedule follow from it.
(function () {
    'use strict';

    var P = window.HitStorPlugins;
    var ui = P.ui;
    var esc = ui.esc;
    var api = ui.api;

    // A usual plan per backup kind until the operator changes the schedule:
    // weekly fulls, daily differentials, logs every 15 minutes; file
    // systems back up nightly.
    var DEFAULT_SCHEDULES = { files: '0 2 * * *', full: '0 1 * * 0', differential: '0 1 * * 1,2,3,4,5,6', log: '*/15 * * * *' };

    function splitSource(src) {
        var i = (src || '').indexOf('|');
        return i < 0 ? { agent: '', id: '' } : { agent: src.slice(0, i), id: src.slice(i + 1) };
    }

    function pluginOf(agents, src) {
        var s = splitSource(src);
        var a = (agents || []).filter(function (x) { return x.name === s.agent; })[0];
        return a && (a.plugins || []).filter(function (p) { return p.id === s.id; })[0];
    }

    function backupKinds(plugin) {
        var kinds = (plugin.capabilities || []).filter(function (c) { return c.indexOf('backup.') === 0; }).map(function (c) { return c.slice(7); });
        return kinds.length ? kinds : ['full'];
    }

    // sourceOptions renders the options of a source into el: excludes and
    // snapshots for a file system, the backup kind and the plugin's own
    // fields for a plugin. values holds the job's current options.
    function sourceOptions(el, plugin, values) {
        values = values || {};
        if (!plugin) {
            el.innerHTML =
                '<div class="form-group"><label>' + esc(t('Exclude Patterns (optional)')) + '</label>' +
                '<input type="text" class="bw-excludes" placeholder="*.tmp, *.log" value="' + esc((values.excludes || []).join(', ')) + '">' +
                '<span class="form-hint">' + esc(t('Comma-separated glob patterns.')) + '</span></div>' +
                '<label class="plugin-field-check"><input type="checkbox" class="bw-vss"' + (values.use_vss ? ' checked' : '') + '> ' +
                esc(t('Consistent snapshot (VSS / LVM / btrfs)')) + '</label>' +
                '<p class="form-hint">' + esc(t('Reads from a volume snapshot for crash-consistent open files. Windows: VSS. Linux: LVM or btrfs when available (agent needs privileges).')) + '</p>';
            return {
                kind: function () { return 'files'; },
                collect: function () {
                    var ex = el.querySelector('.bw-excludes').value.trim();
                    return {
                        excludes: ex ? ex.split(',').map(function (s) { return s.trim(); }).filter(Boolean) : [],
                        use_vss: el.querySelector('.bw-vss').checked
                    };
                }
            };
        }
        var kinds = backupKinds(plugin);
        var kind = kinds.indexOf(values.backup_kind) >= 0 ? values.backup_kind : kinds[0];
        el.innerHTML =
            '<div class="form-group"><label>' + esc(t('Backup kind')) + '</label><select class="bw-kind">' + kinds.map(function (k) {
                return '<option value="' + esc(k) + '"' + (k === kind ? ' selected' : '') + '>' + esc(t(k)) + '</option>';
            }).join('') + '</select></div>' +
            '<div class="bw-backup-fields"></div>' +
            // Plugins that keep credentials per object (set from the tree) have no job settings.
            '<fieldset class="plugin-fieldset bw-settings-set"><legend>' + esc(t('Connection settings')) + '</legend><div class="bw-settings"></div></fieldset>';
        var backupFields = ui.fieldsFor(plugin.fields, 'backup');
        var settingsFields = ui.fieldsFor(plugin.fields, 'settings');
        el.querySelector('.bw-settings-set').hidden = !settingsFields.length;
        ui.renderFields(el.querySelector('.bw-backup-fields'), backupFields, values.backup_options || {});
        ui.renderFields(el.querySelector('.bw-settings'), settingsFields, values.plugin_settings || {});
        return {
            kind: function () { return el.querySelector('.bw-kind').value; },
            collect: function () {
                return {
                    backup_kind: el.querySelector('.bw-kind').value,
                    backup_options: ui.collectFields(el.querySelector('.bw-backup-fields'), backupFields),
                    plugin_settings: ui.collectFields(el.querySelector('.bw-settings'), settingsFields)
                };
            }
        };
    }

    // selectionPanel puts the tree beside a summary of what it selects and
    // the options of the selected source. It reports changes of source to
    // onSource(src, plugin), after the options are drawn.
    function selectionPanel(el, opts) {
        el.innerHTML =
            '<div class="bw-cols">' +
            '<div class="wr-card bw-tree-card"><div class="wr-card-title">' + esc(t('Selections')) + '</div>' +
            '<div class="bw-tree plugin-tree-box"></div>' +
            '<p class="form-hint">' + esc(t('Checking a container, such as an instance or a folder, backs up everything in it, including what is added later.')) + '</p></div>' +
            '<div class="bw-side">' +
            '<div class="wr-card"><div class="wr-card-title">' + esc(t('What will be backed up')) + '</div><div class="bw-summary"></div></div>' +
            '<div class="wr-card bw-opts" hidden><div class="wr-card-title">' + esc(t('Options')) + '</div><div class="bw-opts-body"></div></div>' +
            '</div></div>';
        var summary = el.querySelector('.bw-summary');
        var optsCard = el.querySelector('.bw-opts');
        var optsBody = el.querySelector('.bw-opts-body');
        var current = null;
        var options = null;
        var tree = P.selectionTree(el.querySelector('.bw-tree'), {
            load: P.backupSources({ agent: opts.agent, source: opts.source }),
            initial: opts.initial,
            source: opts.source
        });

        function draw(sel, src) {
            if (!sel.length) {
                summary.innerHTML = '<p class="form-hint">' + esc(t('Expand a machine and check what to back up: folders of its file system, or databases and other objects of an installed plugin.')) + '</p>';
            } else {
                var s = splitSource(src);
                summary.innerHTML =
                    '<div class="bw-origin"><strong>' + esc(s.agent) + '</strong> › ' + esc(P.describeSource(src, opts.agents)) + '</div>' +
                    '<ul class="bw-picked">' + sel.map(function (v) {
                        return '<li><code>' + esc(v) + '</code><button type="button" class="bw-unpick" data-v="' + esc(v) + '" aria-label="' +
                            esc(t('Remove {item}', { item: v })) + '">&times;</button></li>';
                    }).join('') + '</ul>' +
                    (opts.source ? '' : '<p class="form-hint bw-lock">' + esc(t('A job backs up one source. To back up something else, create another job.')) + '</p>');
                summary.querySelectorAll('.bw-unpick').forEach(function (b) {
                    b.addEventListener('click', function () { tree.remove(b.getAttribute('data-v')); });
                });
            }
            var keep = opts.source || src;
            if (keep && keep !== current) {
                current = keep;
                var plugin = splitSource(keep).id === 'files' ? null : pluginOf(opts.agents, keep);
                options = sourceOptions(optsBody, plugin, opts.values);
                optsCard.hidden = false;
                if (opts.onSource) opts.onSource(keep, plugin);
            } else if (!keep && current) {
                current = null;
                options = null;
                optsCard.hidden = true;
                if (opts.onSource) opts.onSource('', null);
            }
        }
        tree.onChange(draw);
        draw([], '');
        return {
            value: function () { return tree.value(); },
            source: function () { return opts.source || tree.source(); },
            options: function () { return options; }
        };
    }

    // describeSchedule says in words when a job runs, as the server does.
    function describeSchedule(when) {
        if (!when.schedule_enabled) return Promise.resolve(t('On demand only'));
        var url = '/api/backups/validate-cron?expr=' + encodeURIComponent(when.cron_expression) + (when.timezone ? '&tz=' + encodeURIComponent(when.timezone) : '');
        return api('GET', url).then(function (d) {
            if (!d.valid) return t('Invalid expression');
            return d.next_3 && d.next_3.length ? t('{description} — Next: {next}', { description: d.description, next: new Date(d.next_3[0]).toLocaleString() }) : d.description;
        }).catch(function () { return when.cron_expression; });
    }

    function openWizard() {
        var steps = [t('1. Selection'), t('2. Target and schedule'), t('3. Review')];
        var d = ui.openDialog(t('New Backup Job'),
            '<form class="bw-form" novalidate>' +
            '<div class="form-stepper-track" aria-hidden="true">' + steps.map(function (s, i) {
                return '<div class="form-stepper-step' + (i ? '' : ' active') + '" data-step="' + (i + 1) + '">' + esc(s) + '</div>';
            }).join('') + '</div>' +
            '<section class="bw-step" data-step="1"><div class="bw-select"></div></section>' +
            '<section class="bw-step" data-step="2" hidden>' +
            '<div class="form-group"><label>' + esc(t('Repository')) + '</label><select class="bw-repo" required></select></div>' +
            '<fieldset class="plugin-fieldset"><legend>' + esc(t('Schedule')) + '</legend><div class="bw-schedule"></div></fieldset></section>' +
            '<section class="bw-step" data-step="3" hidden>' +
            '<div class="form-group"><label>' + esc(t('Job Name')) + '</label><input type="text" class="bw-name" required autocomplete="off"></div>' +
            '<dl class="bw-review"></dl></section>' +
            '<div class="plugin-error" style="display:none"></div>' +
            '<div class="dialog-actions">' +
            '<button type="button" class="btn btn-secondary bw-cancel">' + esc(t('Cancel')) + '</button>' +
            '<button type="button" class="btn btn-secondary bw-back" hidden>' + esc(t('Back')) + '</button>' +
            '<button type="button" class="btn bw-next">' + esc(t('Next')) + '</button>' +
            '<button type="submit" class="btn bw-create" hidden>' + esc(t('Create Job')) + '</button></div></form>');
        d.el.querySelector('.dialog-box').classList.add('dialog-xl');
        var form = d.q('form');
        var errEl = d.q('.plugin-error');
        var step = 1;
        var agents = [];
        var panel = null;
        var schedule = null;
        var scheduleFor = '';
        var scheduleTouched = false;
        var nameTouched = false;
        var scheduleBox = d.q('.bw-schedule');
        scheduleBox.addEventListener('change', function () { scheduleTouched = true; });
        d.q('.bw-name').addEventListener('input', function () { nameTouched = true; });
        d.q('.bw-cancel').addEventListener('click', d.close);

        Promise.all([api('GET', '/api/agents'), api('GET', '/api/repos')]).then(function (res) {
            agents = res[0] || [];
            d.q('.bw-repo').innerHTML = (res[1] || []).map(function (r) { return '<option>' + esc(r.name) + '</option>'; }).join('');
            if (!agents.length) {
                ui.showError(errEl, new Error(t('No agents registered yet. Add one on the Agents page.')));
                return;
            }
            panel = selectionPanel(d.q('.bw-select'), { agents: agents });
        }).catch(function (err) { ui.showError(errEl, err); });

        // The schedule follows the backup kind until the operator edits it.
        function mountSchedule() {
            var key = panel.options().kind();
            if (schedule && (scheduleTouched || key === scheduleFor)) return;
            scheduleFor = key;
            scheduleBox.innerHTML = '';
            var host = document.createElement('div');
            scheduleBox.appendChild(host);
            schedule = HitStorSchedule.mount(host, { defaultCron: DEFAULT_SCHEDULES[key] || DEFAULT_SCHEDULES.full });
        }

        function suggestName() {
            var src = panel.source();
            var s = splitSource(src);
            var sel = panel.value();
            var what = s.id === 'files' ? t('Files') : P.describeSource(src, agents);
            var parts = [s.agent, what];
            if (sel.length === 1) parts.push(sel[0].split(/[\/\\]/).filter(Boolean).pop() || sel[0]);
            if (s.id !== 'files') parts.push(t(panel.options().kind()));
            return parts.join(' – ');
        }

        function review() {
            var src = panel.source();
            var s = splitSource(src);
            var o = panel.options().collect();
            var when = schedule.value();
            if (!nameTouched) d.q('.bw-name').value = suggestName();
            var rows = [
                [t('Machine'), esc(s.agent)],
                [t('Source'), esc(s.id === 'files' ? t('File system') : P.describeSource(src, agents))],
                [t('Selection'), panel.value().map(function (v) { return '<code class="path-chip">' + esc(v) + '</code>'; }).join(' ')]
            ];
            if (s.id === 'files') {
                rows.push([t('Excludes'), o.excludes.length ? o.excludes.map(function (x) { return '<code class="path-chip path-chip--exclude">' + esc(x) + '</code>'; }).join(' ') : '—']);
                rows.push([t('Consistent snapshot'), esc(o.use_vss ? t('Yes') : t('No'))]);
            } else {
                rows.push([t('Backup kind'), esc(t(o.backup_kind))]);
                var set = Object.keys(o.backup_options || {});
                if (set.length) rows.push([t('Options'), esc(set.map(function (k) { return k + '=' + o.backup_options[k]; }).join(', '))]);
            }
            rows.push([t('Repository'), esc(d.q('.bw-repo').value)]);
            rows.push([t('Schedule'), '<span class="bw-when">' + esc(t('Loading...')) + '</span>']);
            d.q('.bw-review').innerHTML = rows.map(function (r) { return '<dt>' + esc(r[0]) + '</dt><dd>' + r[1] + '</dd>'; }).join('');
            describeSchedule(when).then(function (text) { d.q('.bw-when').textContent = text; });
        }

        // check validates the current step before moving on.
        function check() {
            if (step === 1) {
                if (!panel || !panel.value().length) throw new Error(t('Select at least one item to back up.'));
            } else if (step === 2) {
                if (!d.q('.bw-repo').value) throw new Error(t('Select a repository.'));
                schedule.value();
            }
        }

        function go(n) {
            step = n = Math.min(Math.max(n, 1), 3);
            form.querySelectorAll('.bw-step').forEach(function (s) { s.hidden = +s.getAttribute('data-step') !== n; });
            form.querySelectorAll('.form-stepper-step').forEach(function (s) {
                var k = +s.getAttribute('data-step');
                s.classList.toggle('active', k === n);
                s.classList.toggle('done', k < n);
            });
            d.q('.bw-back').hidden = n === 1;
            d.q('.bw-next').hidden = n === 3;
            d.q('.bw-create').hidden = n !== 3;
            if (n === 2) mountSchedule();
            if (n === 3) {
                review();
                d.q('.bw-name').focus();
            }
        }

        d.q('.bw-next').addEventListener('click', function () {
            try {
                check();
            } catch (err) {
                ui.showError(errEl, err);
                return;
            }
            ui.showError(errEl, null);
            go(step + 1);
        });
        d.q('.bw-back').addEventListener('click', function () {
            ui.showError(errEl, null);
            go(step - 1);
        });

        form.addEventListener('submit', function (ev) {
            ev.preventDefault();
            var name = d.q('.bw-name').value.trim();
            if (!name) {
                ui.showError(errEl, new Error(t('Enter a job name.')));
                return;
            }
            var s = splitSource(panel.source());
            var when = schedule.value();
            var body = {
                name: name, agent_name: s.agent, repo_name: d.q('.bw-repo').value,
                schedule_enabled: when.schedule_enabled, cron_expression: when.cron_expression, timezone: when.timezone
            };
            var o = panel.options().collect();
            if (s.id === 'files') {
                body.job_type = 'files';
                body.paths = panel.value();
                body.excludes = o.excludes;
                body.use_vss = o.use_vss;
            } else {
                body.job_type = 'workload';
                body.plugin = s.id;
                body.selection = panel.value();
                body.backup_kind = o.backup_kind;
                body.backup_options = o.backup_options;
                body.plugin_settings = o.plugin_settings;
            }
            var btn = d.q('.bw-create');
            btn.disabled = true;
            btn.textContent = t('Creating...');
            api('POST', '/api/backups', body).then(function () {
                d.close();
                location.reload();
            }).catch(function (err) {
                btn.disabled = false;
                btn.textContent = t('Create Job');
                ui.showError(errEl, err);
            });
        });
    }

    // editSelection lets a job's selection and source options change. The
    // machine and the source stay those of the job; another source is
    // another job.
    function editSelection(el, job) {
        var files = job.job_type !== 'workload';
        var src = job.agent_name + '|' + (files ? 'files' : job.plugin);
        el.innerHTML = '<div class="bw-edit"></div><div class="plugin-error" style="display:none"></div>' +
            '<div class="detail-accordion__actions"><button type="button" class="btn bw-save">' + esc(t('Save selection')) + '</button>' +
            '<span class="detail-form-status bw-status"></span></div>';
        var errEl = el.querySelector('.plugin-error');
        var status = el.querySelector('.bw-status');
        var panel = null;
        api('GET', '/api/agents').then(function (agents) {
            panel = selectionPanel(el.querySelector('.bw-edit'), {
                agents: agents, agent: job.agent_name, source: src,
                initial: files ? job.paths : job.selection,
                values: job
            });
        }).catch(function (err) { ui.showError(errEl, err); });

        el.querySelector('.bw-save').addEventListener('click', function () {
            ui.showError(errEl, null);
            if (!panel) return;
            var sel = panel.value();
            if (!sel.length) {
                ui.showError(errEl, new Error(t('Select at least one item to back up.')));
                return;
            }
            var body = panel.options().collect();
            body.job_type = files ? 'files' : 'workload';
            if (files) body.paths = sel; else body.selection = sel;
            status.textContent = t('Saving...');
            api('PUT', '/api/backups/' + encodeURIComponent(job.id), body).then(function () {
                status.textContent = t('Saved');
                setTimeout(function () { location.reload(); }, 600);
            }).catch(function (err) {
                status.textContent = '';
                ui.showError(errEl, err);
            });
        });
    }

    window.HitStorBackupJob = { openWizard: openWizard, editSelection: editSelection };
})();
