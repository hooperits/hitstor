// Workload plugin UI: agent plugin inventory and remote install, the plugin
// package store, plugin backup jobs, and workload restores. Plugin-specific
// forms come from the fields each plugin declares; nothing here knows about
// any particular plugin.
(function () {
    'use strict';

    function esc(s) {
        var div = document.createElement('div');
        div.textContent = s == null ? '' : String(s);
        return div.innerHTML;
    }

    function formatBytes(b) {
        if (b >= 1073741824) return (b / 1073741824).toFixed(1) + ' GiB';
        if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MiB';
        if (b >= 1024) return (b / 1024).toFixed(1) + ' KiB';
        return b + ' B';
    }

    // api performs a JSON request and rejects with the server's message.
    function api(method, url, body, raw) {
        var opts = { method: method, headers: {} };
        if (body !== undefined) {
            if (raw) {
                opts.body = body;
                opts.headers['Content-Type'] = 'application/octet-stream';
            } else {
                opts.body = JSON.stringify(body);
                opts.headers['Content-Type'] = 'application/json';
            }
        }
        return fetch(url, opts).then(function (r) {
            if (r.status === 204) return null;
            return r.text().then(function (txt) {
                if (!r.ok) throw new Error(txt.trim() || r.statusText);
                return txt ? JSON.parse(txt) : null;
            });
        });
    }

    function localized(m) {
        if (!m) return '';
        return m[window.HITSTOR_LANG] || m.en || '';
    }

    function signerLabel(p) {
        if (!p.signer) return t('unsigned');
        return p.official ? t('official') : t('third-party');
    }

    // --- Declarative plugin fields ---

    // renderFields draws the plugin's fields of one scope into container.
    function renderFields(container, fields, scope, values) {
        values = values || {};
        var list = (fields || []).filter(function (f) { return f.scope === scope; });
        container.innerHTML = '';
        list.forEach(function (f) {
            var id = 'pf-' + scope + '-' + f.key;
            var val = values[f.key] !== undefined ? values[f.key] : (f.default || '');
            var group = document.createElement('div');
            group.className = 'form-group';
            var help = localized(f.help);
            var label = esc(localized(f.label) || f.key) + (f.required ? ' *' : '');
            var input;
            if (f.type === 'bool') {
                input = '<label class="plugin-field-check"><input type="checkbox" id="' + id + '" data-key="' + esc(f.key) + '" data-type="bool"' +
                    (val === 'true' ? ' checked' : '') + '> ' + label + '</label>';
                group.innerHTML = input;
            } else if (f.type === 'choice') {
                input = '<select id="' + id + '" data-key="' + esc(f.key) + '">' + (f.choices || []).map(function (c) {
                    return '<option value="' + esc(c) + '"' + (c === val ? ' selected' : '') + '>' + esc(c) + '</option>';
                }).join('') + '</select>';
                group.innerHTML = '<label for="' + id + '">' + label + '</label>' + input;
            } else {
                input = '<input type="' + (f.type === 'number' ? 'number' : 'text') + '" id="' + id + '" data-key="' + esc(f.key) + '" value="' + esc(val) + '"' +
                    (f.required ? ' required' : '') + '>';
                group.innerHTML = '<label for="' + id + '">' + label + '</label>' + input;
            }
            if (help) {
                var p = document.createElement('p');
                p.className = 'form-hint';
                p.textContent = help;
                group.appendChild(p);
            }
            container.appendChild(group);
        });
        if (!list.length) container.innerHTML = '';
    }

    // collectFields reads the values back, keeping only non-default ones so
    // plugins apply their own defaults.
    function collectFields(container, fields, scope) {
        var out = {};
        (fields || []).filter(function (f) { return f.scope === scope; }).forEach(function (f) {
            var el = container.querySelector('[data-key="' + CSS.escape(f.key) + '"]');
            if (!el) return;
            var v = el.getAttribute('data-type') === 'bool' ? (el.checked ? 'true' : 'false') : el.value.trim();
            if (v !== '' && v !== (f.default || '')) out[f.key] = v;
        });
        return out;
    }

    // --- Dialog scaffolding (reuses the dashboard's dialog styles) ---

    function openDialog(title, bodyHTML) {
        var overlay = document.createElement('div');
        overlay.className = 'dialog-overlay';
        overlay.style.display = 'flex';
        overlay.setAttribute('role', 'dialog');
        overlay.setAttribute('aria-modal', 'true');
        overlay.innerHTML = '<div class="dialog-box dialog-lg"><div class="dialog-header"><h2>' + esc(title) +
            '</h2><button type="button" class="dialog-close" aria-label="' + esc(t('Close')) + '">&times;</button></div>' +
            '<div class="dialog-body">' + bodyHTML + '</div></div>';
        document.body.appendChild(overlay);
        var close = function () { overlay.remove(); };
        overlay.querySelector('.dialog-close').addEventListener('click', close);
        return { el: overlay, close: close, q: function (s) { return overlay.querySelector(s); } };
    }

    function showError(el, err) {
        el.textContent = err ? (err.message || String(err)) : '';
        el.style.display = err ? 'block' : 'none';
    }

    // agentsWithPlugins returns agents that reported at least one plugin.
    function agentsWithPlugins() {
        return api('GET', '/api/agents').then(function (agents) {
            return (agents || []).filter(function (a) { return a.plugins && a.plugins.length; });
        });
    }

    // objectPicker renders a lazily expanding tree of the plugin's objects.
    // multi: checkboxes; otherwise radio buttons.
    function objectPicker(container, agent, plugin, multi) {
        var name = 'obj-' + Math.random().toString(36).slice(2);
        function load(ul, path, onLoaded) {
            ul.innerHTML = '<li class="form-hint">' + esc(t('Loading...')) + '</li>';
            api('GET', '/api/agents/' + encodeURIComponent(agent) + '/plugins/' + encodeURIComponent(plugin) +
                '/browse?path=' + encodeURIComponent(path)).then(function (entries) {
                ul.innerHTML = '';
                if (!entries.length) ul.innerHTML = '<li class="form-hint">' + esc(t('No objects')) + '</li>';
                entries.forEach(function (e) {
                    var li = document.createElement('li');
                    var html = '';
                    if (e.has_children) html += '<button type="button" class="plugin-tree-toggle" aria-label="' + esc(t('Expand')) + '">+</button> ';
                    if (e.selectable) {
                        html += '<label><input type="' + (multi ? 'checkbox' : 'radio') + '" name="' + name + '" value="' + esc(e.path) + '"> ' + esc(e.name) + '</label>';
                    } else {
                        html += '<span>' + esc(e.name) + '</span>';
                    }
                    if (e.kind) html += ' <span class="form-hint">' + esc(e.kind) + '</span>';
                    li.innerHTML = html;
                    var toggle = li.querySelector('.plugin-tree-toggle');
                    if (toggle) {
                        var sub = document.createElement('ul');
                        sub.className = 'plugin-tree';
                        li.appendChild(sub);
                        toggle.addEventListener('click', function () {
                            if (toggle.textContent === '+') {
                                toggle.textContent = '−';
                                load(sub, e.path);
                            } else {
                                toggle.textContent = '+';
                                sub.innerHTML = '';
                            }
                        });
                    }
                    ul.appendChild(li);
                });
                if (onLoaded) onLoaded(entries);
            }).catch(function (err) {
                ul.innerHTML = '<li class="plugin-error">' + esc(err.message) + '</li>';
            });
        }
        var root = document.createElement('ul');
        root.className = 'plugin-tree';
        container.innerHTML = '';
        container.appendChild(root);
        // A single top-level container (one SQL Server instance, say) opens
        // by itself, so its objects are one click away.
        load(root, '', function (entries) {
            if (entries.length === 1 && entries[0].has_children) {
                var toggle = root.querySelector('.plugin-tree-toggle');
                if (toggle) toggle.click();
            }
        });
        return function selected() {
            return Array.prototype.map.call(container.querySelectorAll('input[name="' + name + '"]:checked'), function (i) { return i.value; });
        };
    }

    // --- Agent detail: installed plugins and remote install ---

    function agentPanel(el, agentName) {
        function render() {
            Promise.all([api('GET', '/api/agents'), api('GET', '/api/plugin-packages')]).then(function (res) {
                var agent = (res[0] || []).filter(function (a) { return a.name === agentName; })[0] || {};
                var installed = agent.plugins || [];
                var pkgs = res[1] || [];
                var html = '';
                if (!installed.length) {
                    html += '<p class="form-hint">' + esc(t('No plugins installed on this agent.')) + '</p>';
                } else {
                    html += '<div class="table-wrap"><table><thead><tr><th>' + esc(t('Plugin')) + '</th><th>' + esc(t('Version')) +
                        '</th><th>' + esc(t('Status')) + '</th><th>' + esc(t('Signed')) + '</th><th></th></tr></thead><tbody>';
                    installed.forEach(function (p) {
                        html += '<tr><td><strong>' + esc(p.name || p.id) + '</strong><div class="form-hint">' + esc(p.id) + '</div></td><td>' +
                            esc(p.version) + '</td><td><span class="badge ' + (p.status === 'active' ? 'badge-ok' : 'badge-stale') + '">' + esc(t(p.status || 'active')) +
                            '</span></td><td>' + esc(p.signer ? t('signed') : t('unsigned')) + '</td><td><button type="button" class="btn btn-secondary btn-sm" data-remove="' +
                            esc(p.id) + '">' + esc(t('Remove')) + '</button></td></tr>';
                    });
                    html += '</tbody></table></div>';
                }
                if (pkgs.length) {
                    html += '<div class="plugin-install-row"><select class="plugin-install-select">' + pkgs.map(function (p) {
                        var have = installed.filter(function (i) { return i.id === p.id; })[0];
                        var note = have ? (have.version === p.version ? ' (' + t('installed') + ')' : ' (' + t('upgrade from {v}', { v: have.version }) + ')') : '';
                        return '<option value="' + esc(p.file) + '" data-id="' + esc(p.id) + '">' + esc(p.name + ' ' + p.version + ' — ' + p.platforms.join(', ') + note) + '</option>';
                    }).join('') + '</select> <button type="button" class="btn plugin-install-btn">' + esc(t('Install')) + '</button></div>';
                } else {
                    html += '<p class="form-hint">' + esc(t('Upload plugin packages in Settings to install them here.')) + '</p>';
                }
                html += '<div class="plugin-error" style="display:none"></div>';
                el.innerHTML = html;
                var errEl = el.querySelector('.plugin-error');
                el.querySelectorAll('[data-remove]').forEach(function (b) {
                    b.addEventListener('click', function () {
                        var id = b.getAttribute('data-remove');
                        if (!confirm(t('Remove plugin {id} from {agent}? Its snapshots stay in the repository.', { id: id, agent: agentName }))) return;
                        b.disabled = true;
                        api('DELETE', '/api/agents/' + encodeURIComponent(agentName) + '/plugins/' + encodeURIComponent(id))
                            .then(render).catch(function (err) { b.disabled = false; showError(errEl, err); });
                    });
                });
                var btn = el.querySelector('.plugin-install-btn');
                if (btn) btn.addEventListener('click', function () {
                    var sel = el.querySelector('.plugin-install-select');
                    var opt = sel.options[sel.selectedIndex];
                    var upgrade = installed.some(function (i) { return i.id === opt.getAttribute('data-id'); });
                    btn.disabled = true;
                    btn.textContent = t('Installing...');
                    api('POST', '/api/agents/' + encodeURIComponent(agentName) + '/plugins', { package: sel.value, upgrade: upgrade })
                        .then(render).catch(function (err) {
                            btn.disabled = false;
                            btn.textContent = t('Install');
                            showError(errEl, err);
                        });
                });
            }).catch(function (err) {
                el.innerHTML = '<p class="plugin-error">' + esc(err.message) + '</p>';
            });
        }
        render();
    }

    // --- Settings: plugin package store ---

    function packagesPanel(el) {
        function render() {
            api('GET', '/api/plugin-packages').then(function (pkgs) {
                var html = '<div class="plugin-upload-row"><input type="file" accept=".hpk" class="plugin-upload-file"> ' +
                    '<button type="button" class="btn plugin-upload-btn">' + esc(t('Upload')) + '</button></div>' +
                    '<div class="plugin-error" style="display:none"></div>';
                if (!pkgs.length) {
                    html += '<p class="form-hint">' + esc(t('No plugin packages uploaded yet.')) + '</p>';
                } else {
                    html += '<div class="table-wrap"><table><thead><tr><th>' + esc(t('Plugin')) + '</th><th>' + esc(t('Version')) + '</th><th>' +
                        esc(t('Platforms')) + '</th><th>' + esc(t('Signed')) + '</th><th>' + esc(t('Size')) + '</th><th></th></tr></thead><tbody>';
                    pkgs.forEach(function (p) {
                        html += '<tr><td><strong>' + esc(p.name) + '</strong><div class="form-hint">' + esc(p.file) + '</div></td><td>' + esc(p.version) +
                            '</td><td>' + esc(p.platforms.join(', ')) + '</td><td>' + esc(signerLabel(p)) + '</td><td>' + esc(formatBytes(p.size)) +
                            '</td><td><button type="button" class="btn btn-secondary btn-sm" data-delete="' + esc(p.file) + '">' + esc(t('Delete')) + '</button></td></tr>';
                    });
                    html += '</tbody></table></div>';
                }
                el.innerHTML = html;
                var errEl = el.querySelector('.plugin-error');
                el.querySelector('.plugin-upload-btn').addEventListener('click', function () {
                    var file = el.querySelector('.plugin-upload-file').files[0];
                    if (!file) return;
                    var btn = this;
                    btn.disabled = true;
                    btn.textContent = t('Uploading...');
                    api('POST', '/api/plugin-packages', file, true).then(render).catch(function (err) {
                        btn.disabled = false;
                        btn.textContent = t('Upload');
                        showError(errEl, err);
                    });
                });
                el.querySelectorAll('[data-delete]').forEach(function (b) {
                    b.addEventListener('click', function () {
                        var f = b.getAttribute('data-delete');
                        if (!confirm(t('Delete package {file}? Installed plugins are not affected.', { file: f }))) return;
                        api('DELETE', '/api/plugin-packages/' + encodeURIComponent(f)).then(render).catch(function (err) { showError(errEl, err); });
                    });
                });
            }).catch(function (err) {
                el.innerHTML = '<p class="plugin-error">' + esc(err.message) + '</p>';
            });
        }
        render();
    }

    // --- Backups: new plugin backup job ---

    function openJobDialog() {
        var d = openDialog(t('New Plugin Backup Job'),
            '<form class="plugin-job-form">' +
            '<div class="form-group"><label>' + esc(t('Job Name')) + '</label><input type="text" name="name" required></div>' +
            '<div class="form-group"><label>' + esc(t('Agent')) + '</label><select name="agent" required></select></div>' +
            '<div class="form-group"><label>' + esc(t('Plugin')) + '</label><select name="plugin" required></select></div>' +
            '<div class="form-group"><label>' + esc(t('Objects to back up')) + '</label><div class="plugin-objects plugin-tree-box"></div></div>' +
            '<div class="form-group"><label>' + esc(t('Backup kind')) + '</label><select name="kind"></select></div>' +
            '<fieldset class="plugin-fieldset"><legend>' + esc(t('Connection settings')) + '</legend><div class="plugin-settings"></div></fieldset>' +
            '<fieldset class="plugin-fieldset"><legend>' + esc(t('Backup options')) + '</legend><div class="plugin-backup-opts"></div></fieldset>' +
            '<div class="form-group"><label>' + esc(t('Repository')) + '</label><select name="repo" required></select></div>' +
            '<div class="form-group"><label>' + esc(t('Schedule (cron expression)')) + '</label><input type="text" name="cron" placeholder="0 2 * * *">' +
            '<p class="form-hint">' + esc(t('Leave empty to run only on demand.')) + '</p></div>' +
            '<div class="plugin-error" style="display:none"></div>' +
            '<div class="dialog-actions"><button type="submit" class="btn">' + esc(t('Create Job')) + '</button></div></form>');
        var form = d.q('form');
        var errEl = d.q('.plugin-error');
        var agents = [];
        var current = null;
        var selected = function () { return []; };

        function pluginChanged() {
            var a = agents.filter(function (x) { return x.name === form.elements.agent.value; })[0];
            current = a ? a.plugins.filter(function (p) { return p.id === form.elements.plugin.value; })[0] : null;
            if (!current) return;
            var kinds = (current.capabilities || []).filter(function (c) { return c.indexOf('backup.') === 0; }).map(function (c) { return c.slice(7); });
            if (!kinds.length) kinds = ['full'];
            form.elements.kind.innerHTML = kinds.map(function (k) { return '<option value="' + esc(k) + '">' + esc(t(k)) + '</option>'; }).join('');
            renderFields(d.q('.plugin-settings'), current.fields, 'settings');
            renderFields(d.q('.plugin-backup-opts'), current.fields, 'backup');
            selected = objectPicker(d.q('.plugin-objects'), a.name, current.id, true);
        }
        function agentChanged() {
            var a = agents.filter(function (x) { return x.name === form.elements.agent.value; })[0];
            form.elements.plugin.innerHTML = (a ? a.plugins : []).map(function (p) { return '<option value="' + esc(p.id) + '">' + esc(p.name || p.id) + '</option>'; }).join('');
            pluginChanged();
        }
        form.elements.agent.addEventListener('change', agentChanged);
        form.elements.plugin.addEventListener('change', pluginChanged);

        Promise.all([agentsWithPlugins(), api('GET', '/api/repos')]).then(function (res) {
            agents = res[0];
            if (!agents.length) {
                showError(errEl, new Error(t('No agent has reported a plugin yet. Install one from the agent page.')));
                return;
            }
            form.elements.agent.innerHTML = agents.map(function (a) { return '<option>' + esc(a.name) + '</option>'; }).join('');
            form.elements.repo.innerHTML = (res[1] || []).map(function (r) { return '<option>' + esc(r.name) + '</option>'; }).join('');
            agentChanged();
        }).catch(function (err) { showError(errEl, err); });

        form.addEventListener('submit', function (ev) {
            ev.preventDefault();
            var sel = selected();
            if (!sel.length) {
                showError(errEl, new Error(t('Select at least one object.')));
                return;
            }
            var cron = form.elements.cron.value.trim();
            var body = {
                name: form.elements.name.value.trim(), agent_name: form.elements.agent.value, repo_name: form.elements.repo.value,
                job_type: 'workload', plugin: current.id, selection: sel, backup_kind: form.elements.kind.value,
                plugin_settings: collectFields(d.q('.plugin-settings'), current.fields, 'settings'),
                backup_options: collectFields(d.q('.plugin-backup-opts'), current.fields, 'backup'),
                schedule_enabled: cron !== '', cron_expression: cron,
                timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || ''
            };
            api('POST', '/api/backups', body).then(function () {
                d.close();
                location.reload();
            }).catch(function (err) { showError(errEl, err); });
        });
    }

    // --- Restores: workload restore ---

    function openRestoreDialog() {
        var d = openDialog(t('Restore from a Plugin Backup'),
            '<form class="plugin-restore-form">' +
            '<div class="form-group"><label>' + esc(t('Restore on agent')) + '</label><select name="agent" required></select></div>' +
            '<div class="form-group"><label>' + esc(t('Plugin')) + '</label><select name="plugin" required></select></div>' +
            '<div class="form-group"><label>' + esc(t('Backups from agent')) + '</label><select name="source"></select></div>' +
            '<div class="form-group"><label>' + esc(t('Object')) + '</label><input type="text" name="object" required>' +
            '<div class="plugin-objects plugin-tree-box"></div></div>' +
            '<div class="dialog-actions"><button type="button" class="btn btn-secondary plugin-points-btn">' + esc(t('Show restore points')) + '</button></div>' +
            '<div class="plugin-points"></div>' +
            '<div class="form-group"><label><input type="radio" name="when" value="latest" checked> ' + esc(t('Latest point')) + '</label> ' +
            '<label><input type="radio" name="when" value="at"> ' + esc(t('Point in time')) + '</label> ' +
            '<input type="datetime-local" name="at" step="1"></div>' +
            '<fieldset class="plugin-fieldset"><legend>' + esc(t('Restore options')) + '</legend><div class="plugin-restore-opts"></div></fieldset>' +
            '<div class="plugin-error" style="display:none"></div>' +
            '<div class="plugin-progress"></div>' +
            '<div class="dialog-actions"><button type="submit" class="btn">' + esc(t('Start Restore')) + '</button></div></form>');
        var form = d.q('form');
        var errEl = d.q('.plugin-error');
        var agents = [];
        var current = null;
        var picked = function () { return []; };

        function pluginChanged() {
            var a = agents.filter(function (x) { return x.name === form.elements.agent.value; })[0];
            current = a ? a.plugins.filter(function (p) { return p.id === form.elements.plugin.value; })[0] : null;
            if (!current) return;
            renderFields(d.q('.plugin-restore-opts'), current.fields, 'restore');
            picked = objectPicker(d.q('.plugin-objects'), a.name, current.id, false);
        }
        d.q('.plugin-objects').addEventListener('change', function () {
            var v = picked();
            if (v.length) form.elements.object.value = v[0];
        });
        form.elements.agent.addEventListener('change', function () {
            var a = agents.filter(function (x) { return x.name === form.elements.agent.value; })[0];
            form.elements.plugin.innerHTML = (a ? a.plugins : []).map(function (p) { return '<option value="' + esc(p.id) + '">' + esc(p.name || p.id) + '</option>'; }).join('');
            pluginChanged();
        });
        form.elements.plugin.addEventListener('change', pluginChanged);

        Promise.all([api('GET', '/api/agents')]).then(function (res) {
            var all = res[0] || [];
            agents = all.filter(function (a) { return a.plugins && a.plugins.length; });
            if (!agents.length) {
                showError(errEl, new Error(t('No agent has reported a plugin yet. Install one from the agent page.')));
                return;
            }
            form.elements.agent.innerHTML = agents.map(function (a) { return '<option>' + esc(a.name) + '</option>'; }).join('');
            form.elements.source.innerHTML = '<option value="">' + esc(t('Same agent')) + '</option>' +
                all.map(function (a) { return '<option>' + esc(a.name) + '</option>'; }).join('');
            form.elements.agent.dispatchEvent(new Event('change'));
        }).catch(function (err) { showError(errEl, err); });

        d.q('.plugin-points-btn').addEventListener('click', function () {
            var box = d.q('.plugin-points');
            var src = form.elements.source.value || form.elements.agent.value;
            box.innerHTML = '<p class="form-hint">' + esc(t('Loading...')) + '</p>';
            api('GET', '/api/workload/points?agent=' + encodeURIComponent(src) + '&plugin=' + encodeURIComponent(form.elements.plugin.value) +
                '&object=' + encodeURIComponent(form.elements.object.value.trim())).then(function (pts) {
                if (!pts.length) {
                    box.innerHTML = '<p class="form-hint">' + esc(t('No backups of this object were found.')) + '</p>';
                    return;
                }
                box.innerHTML = '<div class="table-wrap"><table><thead><tr><th>' + esc(t('Time')) + '</th><th>' + esc(t('Kind')) + '</th><th>' +
                    esc(t('Size')) + '</th><th>' + esc(t('Snapshot')) + '</th></tr></thead><tbody>' + pts.slice().reverse().map(function (p) {
                    return '<tr class="plugin-point" data-time="' + esc(p.created_at) + '"><td>' + esc(new Date(p.created_at).toLocaleString()) + '</td><td>' +
                        esc(t(p.kind || 'full')) + '</td><td>' + esc(formatBytes(p.size)) + '</td><td>' + esc(p.snapshot_id.slice(0, 8)) + '</td></tr>';
                }).join('') + '</tbody></table></div><p class="form-hint">' + esc(t('Click a backup to restore to its time.')) + '</p>';
                box.querySelectorAll('.plugin-point').forEach(function (row) {
                    row.addEventListener('click', function () {
                        var dt = new Date(row.getAttribute('data-time'));
                        var local = new Date(dt.getTime() - dt.getTimezoneOffset() * 60000).toISOString().slice(0, 19);
                        form.elements.at.value = local;
                        form.querySelector('input[name="when"][value="at"]').checked = true;
                    });
                });
            }).catch(function (err) { box.innerHTML = '<p class="plugin-error">' + esc(err.message) + '</p>'; });
        });

        form.addEventListener('submit', function (ev) {
            ev.preventDefault();
            showError(errEl, null);
            var body = {
                agent_name: form.elements.agent.value, source_agent: form.elements.source.value, plugin: current.id,
                object: form.elements.object.value.trim(), options: collectFields(d.q('.plugin-restore-opts'), current.fields, 'restore')
            };
            if (form.querySelector('input[name="when"]:checked').value === 'at') {
                if (!form.elements.at.value) {
                    showError(errEl, new Error(t('Choose the point in time.')));
                    return;
                }
                body.target = new Date(form.elements.at.value).toISOString();
            }
            var submit = form.querySelector('button[type="submit"]');
            submit.disabled = true;
            api('POST', '/api/workload/restores', body).then(function (sess) { poll(sess.id); })
                .catch(function (err) { submit.disabled = false; showError(errEl, err); });
        });

        function poll(id) {
            var box = d.q('.plugin-progress');
            api('GET', '/api/workload/restores/' + encodeURIComponent(id)).then(function (s) {
                var pct = s.bytes_total ? Math.floor(100 * s.bytes_sent / s.bytes_total) : 0;
                var html = '<p><span class="badge ' + (s.status === 'completed' ? 'badge-ok' : s.status === 'failed' ? 'badge-critical' : 'badge-running') + '">' +
                    esc(t(s.status)) + '</span> ' + (s.bytes_total ? esc(formatBytes(s.bytes_sent) + ' / ' + formatBytes(s.bytes_total) + ' (' + pct + '%)') : '') + '</p>';
                if (s.plan && s.plan.length) html += '<p class="form-hint">' + esc(t('Plan')) + ': ' + s.plan.map(esc).join(' → ') + '</p>';
                if (s.message) html += '<p>' + esc(s.message) + '</p>';
                if (s.error) html += '<p class="plugin-error">' + esc(s.error) + '</p>';
                box.innerHTML = html;
                if (s.status !== 'completed' && s.status !== 'failed') setTimeout(function () { poll(id); }, 1500);
            }).catch(function (err) { showError(errEl, err); });
        }
    }

    window.HitStorPlugins = {
        agentPanel: agentPanel,
        packagesPanel: packagesPanel,
        openJobDialog: openJobDialog,
        openRestoreDialog: openRestoreDialog
    };
})();
