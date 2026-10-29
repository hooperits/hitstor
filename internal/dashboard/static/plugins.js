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

    // fieldsFor returns the fields of one scope; for scope "object", only
    // those that apply to objects of kind.
    function fieldsFor(fields, scope, kind) {
        return (fields || []).filter(function (f) {
            if (f.scope !== scope) return false;
            return scope !== 'object' || !f.object_kinds || !f.object_kinds.length || f.object_kinds.indexOf(kind) >= 0;
        });
    }

    // renderFields draws fields into container. secretsSet lists password
    // fields that already have a saved value.
    function renderFields(container, list, values, secretsSet) {
        values = values || {};
        secretsSet = secretsSet || [];
        container.innerHTML = '';
        list.forEach(function (f) {
            var id = 'pf-' + f.scope + '-' + f.key + '-' + Math.random().toString(36).slice(2, 7);
            var val = values[f.key] !== undefined ? values[f.key] : (f.default || '');
            var group = document.createElement('div');
            group.className = 'form-group';
            if (f.show_if) group.setAttribute('data-show-if', f.show_if);
            var help = localized(f.help);
            var label = esc(localized(f.label) || f.key) + (f.required ? ' *' : '');
            var input;
            if (f.type === 'bool') {
                input = '<label class="plugin-field-check"><input type="checkbox" id="' + id + '" data-key="' + esc(f.key) + '" data-type="bool"' +
                    (val === 'true' ? ' checked' : '') + '> ' + label + '</label>';
                group.innerHTML = input;
            } else if (f.type === 'choice') {
                input = '<select id="' + id + '" data-key="' + esc(f.key) + '">' + (f.choices || []).map(function (c) {
                    return '<option value="' + esc(c) + '"' + (c === val ? ' selected' : '') + '>' + esc(t(c)) + '</option>';
                }).join('') + '</select>';
                group.innerHTML = '<label for="' + id + '">' + label + '</label>' + input;
            } else if (f.type === 'password') {
                var saved = secretsSet.indexOf(f.key) >= 0;
                input = '<input type="password" id="' + id + '" data-key="' + esc(f.key) + '" data-type="password" autocomplete="new-password"' +
                    (saved ? ' placeholder="' + esc(t('Saved — leave empty to keep it')) + '"' : '') + '>';
                group.innerHTML = '<label for="' + id + '">' + label + '</label>' + input;
            } else {
                input = '<input type="' + (f.type === 'number' ? 'number' : 'text') + '" id="' + id + '" data-key="' + esc(f.key) + '" value="' + esc(val) + '"' +
                    (f.required ? ' required' : '') + ' autocomplete="off">';
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
        applyShowIf(container);
        if (!container.hasAttribute('data-show-if-bound')) {
            container.setAttribute('data-show-if-bound', '');
            container.addEventListener('change', function () { applyShowIf(container); });
        }
    }

    // applyShowIf hides fields whose "key=v1|v2" condition does not hold.
    function applyShowIf(container) {
        container.querySelectorAll('[data-show-if]').forEach(function (group) {
            var cond = group.getAttribute('data-show-if').split('=');
            var ctl = container.querySelector('[data-key="' + CSS.escape(cond[0]) + '"]');
            var v = ctl ? (ctl.getAttribute('data-type') === 'bool' ? String(ctl.checked) : ctl.value) : '';
            group.hidden = (cond[1] || '').split('|').indexOf(v) < 0;
        });
    }

    // collectFields reads the values back, skipping fields hidden by their
    // show_if condition and keeping only non-default ones so plugins apply
    // their own defaults. An empty password is sent as "" (keep the saved
    // one).
    function collectFields(container, list) {
        var out = {};
        list.forEach(function (f) {
            var el = container.querySelector('[data-key="' + CSS.escape(f.key) + '"]');
            if (!el || el.closest('[data-show-if][hidden]')) return;
            var type = el.getAttribute('data-type');
            if (type === 'password') {
                out[f.key] = el.value;
                return;
            }
            var v = type === 'bool' ? (el.checked ? 'true' : 'false') : el.value.trim();
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

    var ICONS = {
        machine: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="12" rx="1.5"/><path d="M8 20h8M12 16v4"/></svg>',
        filesystem: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="6" width="18" height="12" rx="2"/><path d="M7 14h.01M11 14h6"/></svg>',
        plugin: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3v4M15 3v4M6 7h12v4a6 6 0 0 1-12 0zM12 17v4"/></svg>',
        instance: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="7" rx="1.5"/><rect x="3" y="13" width="18" height="7" rx="1.5"/><circle cx="7" cy="7.5" r="1"/><circle cx="7" cy="16.5" r="1"/></svg>',
        database: '<svg viewBox="0 0 24 24" aria-hidden="true"><ellipse cx="12" cy="5.5" rx="7.5" ry="2.8"/><path d="M4.5 5.5v13c0 1.5 3.4 2.8 7.5 2.8s7.5-1.3 7.5-2.8v-13"/><path d="M4.5 12c0 1.5 3.4 2.8 7.5 2.8s7.5-1.3 7.5-2.8"/></svg>',
        folder: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2h8.5A1.5 1.5 0 0 1 21 8.5v9a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5z"/></svg>',
        item: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/></svg>',
        key: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M16 7l3 3M14 9l2 2"/></svg>',
        chevron: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 6l6 6-6 6"/></svg>'
    };

    function iconFor(e) {
        if (ICONS[e.kind] && e.kind !== 'item') return ICONS[e.kind];
        if (e.kind === 'server') return ICONS.instance;
        if (/database$/.test(e.kind || '')) return ICONS.database;
        return e.has_children || e.kind === 'dir' ? ICONS.folder : ICONS.item;
    }

    // under reports whether path v lies inside container path base, for
    // both plugin paths (a/b) and filesystem paths (C:\a, /a).
    function under(v, base) {
        if (v === base || !base) return false;
        if (/[\/\\]$/.test(base)) return v.indexOf(base) === 0;
        return v.indexOf(base + '/') === 0 || v.indexOf(base + '\\') === 0;
    }

    // selectionTree renders a lazily expanding selection tree. Entries come
    // from opts.load(parentEntry) as {path, name, kind, has_children,
    // selectable, detail, status, configurable, configured, open}; the
    // loader may also resolve to {entries, detail} to annotate the parent.
    // Children inherit their parent's source (one machine's file system or
    // one plugin) and ctx ({agent, plugin, fields}, for Configure).
    //
    // multi gives checkboxes, where checking a container selects it as a
    // whole (everything in it, including objects added later); otherwise
    // radio buttons on leaf objects only. Like the selection sets of
    // enterprise backup tools, one selection holds objects of one source:
    // once something is checked, other sources lock until it is cleared.
    // opts.initial preselects values (of opts.source) and opens the tree
    // down to them; values not shown yet stay selected.
    function selectionTree(container, opts) {
        var multi = opts.multi !== false;
        var name = 'obj-' + Math.random().toString(36).slice(2);
        var pending = (opts.initial || []).slice();
        var fixedSource = opts.source || '';
        container.innerHTML = '<div class="ptree-root" role="tree"></div>';
        var rootEl = container.querySelector('.ptree-root');
        var listeners = [];

        function inputs() {
            return Array.prototype.slice.call(container.querySelectorAll('input[name="' + name + '"]'));
        }

        // selected lists the chosen values: checked boxes plus preselected
        // values not shown yet, without those inside a chosen container.
        function selected() {
            var shown = {};
            var out = [];
            inputs().forEach(function (i) {
                shown[i.value] = true;
                if (i.checked && !i.disabled) out.push(i.value);
            });
            pending.forEach(function (v) { if (!shown[v]) out.push(v); });
            return out.filter(function (v) {
                return !out.some(function (o) { return under(v, o); });
            });
        }

        function activeSource() {
            if (fixedSource) return fixedSource;
            var box = inputs().filter(function (i) { return i.checked && !i.disabled; })[0];
            return box ? box.getAttribute('data-source') : (pending.length ? fixedSource : '');
        }

        // refresh derives every row's state from the selection: included
        // (inside a checked container), locked (another source), partly
        // selected, and the per-machine and per-source counts.
        function refresh() {
            var sel = selected();
            var src = sel.length ? activeSource() : '';
            inputs().forEach(function (i) {
                var row = i.closest('.ptree-row');
                var node = row.parentNode;
                var included = false;
                for (var p = node.parentElement && node.parentElement.closest('.ptree-node'); p; p = p.parentElement && p.parentElement.closest('.ptree-node')) {
                    var pb = p.querySelector(':scope > .ptree-row > .ptree-check');
                    if (pb && pb.checked) { included = true; break; }
                }
                var locked = !!src && i.getAttribute('data-source') !== src;
                if (included) i.checked = false;
                i.disabled = included || locked || i.hasAttribute('data-unselectable');
                i.indeterminate = multi && !i.checked && !locked && sel.some(function (v) { return under(v, i.value); });
                row.classList.toggle('ptree-included', included);
                row.classList.toggle('ptree-locked', locked);
            });
            container.querySelectorAll('.ptree-count').forEach(function (c) {
                var on = !!src && (src === c.getAttribute('data-for') || src.indexOf(c.getAttribute('data-for') + '|') === 0);
                c.hidden = !on;
                c.textContent = on ? t('{n} selected', { n: sel.length }) : '';
                c.closest('.ptree-row').classList.toggle('ptree-locked', !!src && !on);
            });
            listeners.forEach(function (fn) { fn(sel, src); });
        }

        function load(childrenEl, parentNode, entry) {
            childrenEl.innerHTML = '<div class="ptree-msg form-hint">' + esc(t('Loading...')) + '</div>';
            return Promise.resolve(opts.load(entry)).then(function (res) {
                var entries = Array.isArray(res) ? res : (res.entries || []);
                if (parentNode && res.detail !== undefined) {
                    parentNode.querySelector(':scope > .ptree-row > .ptree-detail').textContent = res.detail;
                }
                childrenEl.innerHTML = '';
                if (!entries.length) {
                    childrenEl.innerHTML = '<div class="ptree-msg form-hint">' + esc(opts.emptyText || t('No objects')) + '</div>';
                    return entries;
                }
                if (entries.length > 8) {
                    var filter = document.createElement('input');
                    filter.type = 'search';
                    filter.className = 'ptree-filter';
                    filter.placeholder = t('Filter...');
                    filter.addEventListener('input', function () {
                        var q = filter.value.trim().toLowerCase();
                        childrenEl.querySelectorAll(':scope > .ptree-node').forEach(function (n) {
                            n.hidden = q !== '' && n.getAttribute('data-name').toLowerCase().indexOf(q) < 0;
                        });
                    });
                    childrenEl.appendChild(filter);
                }
                entries.forEach(function (e) {
                    if (entry) {
                        if (e.source === undefined) e.source = entry.source;
                        if (e.ctx === undefined) e.ctx = entry.ctx;
                    }
                    childrenEl.appendChild(node(e));
                });
                refresh();
                // Open containers on the way to preselected values, and those
                // the loader asks to open (a single instance, say).
                childrenEl.querySelectorAll(':scope > .ptree-node').forEach(function (n) {
                    if (n._entry.open || (n._entry.source === fixedSource && pending.some(function (v) { return under(v, n._entry.path); }))) {
                        var tg = n.querySelector(':scope > .ptree-row > .ptree-toggle');
                        if (tg) tg.click();
                    }
                });
                return entries;
            }).catch(function (err) {
                childrenEl.innerHTML = '';
                var msg = document.createElement('div');
                msg.className = 'ptree-msg plugin-error';
                msg.textContent = err.message;
                childrenEl.appendChild(msg);
                if (parentNode) parentNode.querySelector(':scope > .ptree-row').classList.add('ptree-unavailable');
                if (entry && entry.configurable && entry.ctx) {
                    var fix = document.createElement('button');
                    fix.type = 'button';
                    fix.className = 'btn btn-secondary btn-sm ptree-fix';
                    fix.textContent = t('Configure credentials');
                    fix.addEventListener('click', function () { configure(entry, parentNode); });
                    childrenEl.appendChild(fix);
                }
                return [];
            });
        }

        function configure(entry, nodeEl) {
            var c = entry.ctx;
            openObjectConfigDialog(c.agent, c.plugin, entry, c.fields || [], function (configured) {
                entry.configured = configured;
                var chip = nodeEl.querySelector(':scope > .ptree-row .ptree-config-state');
                if (chip) {
                    chip.className = 'ptree-config-state badge ' + (configured ? 'badge-ok' : 'badge-stale');
                    chip.textContent = configured ? t('Credentials saved') : t('Agent account');
                }
                nodeEl.querySelector(':scope > .ptree-row').classList.remove('ptree-unavailable');
                var kids = nodeEl.querySelector(':scope > .ptree-children');
                if (!kids) return;
                if (nodeEl.classList.contains('ptree-open')) {
                    load(kids, nodeEl, entry);
                } else {
                    nodeEl.querySelector(':scope > .ptree-row > .ptree-toggle').click();
                }
            });
        }

        function node(e) {
            var el = document.createElement('div');
            el.className = 'ptree-node';
            el._entry = e;
            el.setAttribute('role', 'treeitem');
            el.setAttribute('data-name', e.name || e.path);
            var row = document.createElement('div');
            row.className = 'ptree-row ptree-kind-' + esc((e.kind || 'item').replace(/[^a-z_]/gi, '')) + (e.status ? ' ptree-unavailable' : '');
            var html = e.has_children
                ? '<button type="button" class="ptree-toggle" aria-label="' + esc(t('Expand')) + '" aria-expanded="false">' + ICONS.chevron + '</button>'
                : '<span class="ptree-toggle-spacer"></span>';
            var pickable = multi ? e.selectable : (e.selectable && !e.has_children);
            var id = name + '-' + Math.random().toString(36).slice(2, 8);
            if (e.selectable !== undefined && (multi || pickable)) {
                html += '<input class="ptree-check" id="' + id + '" type="' + (multi ? 'checkbox' : 'radio') + '" name="' + name + '" value="' + esc(e.path) + '"' +
                    ' data-source="' + esc(e.source || '') + '"' + (pickable ? '' : ' disabled data-unselectable') + '>';
            } else if (e.selectable !== undefined) {
                html += '<span class="ptree-check-spacer"></span>';
            }
            html += '<span class="ptree-icon ptree-icon-' + esc((e.kind || 'item').replace(/[^a-z_]/gi, '')) + '">' + iconFor(e) + '</span>' +
                '<label class="ptree-name" for="' + id + '">' + esc(e.name || e.path) + '</label>';
            if (e.kind === 'system_database') html += '<span class="ptree-tag">' + esc(t('system')) + '</span>';
            if (e.tag) html += '<span class="ptree-tag">' + esc(e.tag) + '</span>';
            if (e.status) html += '<span class="badge badge-stale">' + esc(e.status) + '</span>';
            if (e.count_key) html += '<span class="ptree-count badge badge-ok" data-for="' + esc(e.count_key) + '" hidden></span>';
            html += '<span class="ptree-detail">' + esc(e.detail || '') + '</span>';
            if (e.configurable && e.ctx) {
                html += '<span class="ptree-config-state badge ' + (e.configured ? 'badge-ok' : 'badge-stale') + '">' +
                    esc(e.configured ? t('Credentials saved') : t('Agent account')) + '</span>' +
                    '<button type="button" class="ptree-configure btn btn-secondary btn-sm" title="' + esc(t('Configure credentials')) + '">' +
                    ICONS.key + '<span>' + esc(t('Configure')) + '</span></button>';
            }
            row.innerHTML = html;
            el.appendChild(row);

            var check = row.querySelector('.ptree-check');
            if (check) {
                if (e.source === fixedSource && pending.indexOf(e.path) >= 0) check.checked = true;
                check.addEventListener('change', function () {
                    // A choice made in the tree replaces the preselection
                    // at and below this object.
                    pending = pending.filter(function (v) { return v !== e.path && !under(v, e.path); });
                    refresh();
                });
            }
            var cfg = row.querySelector('.ptree-configure');
            if (cfg) cfg.addEventListener('click', function () { configure(e, el); });

            if (e.has_children) {
                var kids = document.createElement('div');
                kids.className = 'ptree-children';
                kids.setAttribute('role', 'group');
                kids.hidden = true;
                el.appendChild(kids);
                var toggle = row.querySelector('.ptree-toggle');
                var loaded = false;
                var expand = function () {
                    var open = !el.classList.contains('ptree-open');
                    el.classList.toggle('ptree-open', open);
                    toggle.setAttribute('aria-expanded', String(open));
                    kids.hidden = !open;
                    if (open && !loaded) {
                        loaded = true;
                        load(kids, el, e);
                    }
                };
                toggle.addEventListener('click', expand);
                if (!check) row.querySelector('.ptree-name').addEventListener('click', expand);
            }
            return el;
        }

        load(rootEl, null, null);
        return {
            value: selected,
            source: function () { return selected().length ? activeSource() : ''; },
            // remove drops one value, as the selection summary does.
            remove: function (v) {
                pending = pending.filter(function (p) { return p !== v; });
                inputs().forEach(function (i) { if (i.value === v) i.checked = false; });
                refresh();
            },
            onChange: function (fn) { listeners.push(fn); }
        };
    }

    // backupSources loads the backup selection tree: machines, then each
    // machine's sources (its file system and every plugin it reported),
    // then their contents. opts.agent and opts.source narrow it to one
    // machine and one source, for editing a job.
    function backupSources(opts) {
        opts = opts || {};
        var agents = {};
        function agentURL(n) { return '/api/agents/' + encodeURIComponent(n); }

        function machines() {
            return api('GET', '/api/agents').then(function (list) {
                return (list || []).filter(function (a) { return !opts.agent || a.name === opts.agent; }).map(function (a) {
                    agents[a.name] = a;
                    return {
                        path: 'agent:' + a.name, name: a.name, kind: 'machine', has_children: !a.suspended,
                        status: a.suspended ? t('suspended') : '', count_key: a.name, machine: a.name, open: !!opts.agent
                    };
                });
            });
        }

        // sources asks the agent who it is, which also tells whether it is
        // reachable, and lists what it can back up.
        function sources(m) {
            var a = agents[m.machine];
            return api('GET', agentURL(a.name) + '/info').then(function (info) {
                var list = [{
                    path: 'files:' + a.name, name: t('File system'), kind: 'filesystem', has_children: true,
                    source: a.name + '|files', count_key: a.name + '|files', fs: a.name,
                    detail: info.hostname || ''
                }];
                (a.plugins || []).forEach(function (p) {
                    var active = !p.status || p.status === 'active';
                    list.push({
                        path: 'plugin:' + a.name + '/' + p.id, name: p.name || p.id, kind: 'plugin', has_children: active,
                        source: a.name + '|' + p.id, count_key: a.name + '|' + p.id, plugin: p,
                        ctx: { agent: a.name, plugin: p.id, fields: p.fields || [] },
                        detail: 'v' + p.version, status: active ? '' : t(p.status)
                    });
                });
                if (opts.source) list = list.filter(function (s) { return s.source === opts.source; });
                list.forEach(function (s) { s.open = !!opts.source; });
                var os = [info.os === 'windows' ? 'Windows' : info.os === 'linux' ? 'Linux' : info.os, info.arch].filter(Boolean).join(' · ');
                return { entries: list, detail: [os, info.version ? 'v' + info.version : ''].filter(Boolean).join(' · ') };
            }, function (err) {
                throw new Error(t('Agent unreachable: {message}', { message: err.message }));
            });
        }

        function files(agent, path) {
            return api('POST', agentURL(agent) + '/browse', { path: path || '' }).then(function (list) {
                return (list || []).map(function (f) {
                    return {
                        path: f.path, name: f.name, kind: f.is_dir ? 'dir' : 'file',
                        has_children: f.is_dir && f.has_children, selectable: f.is_dir && f.readable !== false,
                        detail: f.is_dir ? '' : formatBytes(f.size || 0),
                        status: f.readable === false ? t('Not readable') : '', fs: agent
                    };
                });
            });
        }

        function objects(e) {
            var c = e.ctx;
            var path = e.kind === 'plugin' ? '' : e.path;
            return api('GET', agentURL(c.agent) + '/plugins/' + encodeURIComponent(c.plugin) + '/browse?path=' + encodeURIComponent(path)).then(function (list) {
                // A single top-level container (one SQL Server instance, say)
                // opens by itself, so its objects are one click away.
                if (e.kind === 'plugin' && list.length === 1 && list[0].has_children) list[0].open = true;
                return list;
            });
        }

        return function (e) {
            if (!e) return machines();
            if (e.kind === 'machine') return sources(e);
            if (e.fs) return files(e.fs, e.kind === 'filesystem' ? '' : e.path);
            return objects(e);
        };
    }

    // describeSource names a source key ("agent|files" or "agent|plugin").
    function describeSource(src, agentsList) {
        var parts = (src || '').split('|');
        if (parts[1] === 'files') return t('File system');
        var a = (agentsList || []).filter(function (x) { return x.name === parts[0]; })[0];
        var p = a && (a.plugins || []).filter(function (x) { return x.id === parts[1]; })[0];
        return p ? (p.name || p.id) : parts[1];
    }

    // openObjectConfigDialog edits the settings an object keeps on the agent,
    // such as a database instance's credentials. The agent tests them before
    // saving; secrets never come back. onDone(configured) runs after a save
    // or removal.
    function openObjectConfigDialog(agent, plugin, entry, fields, onDone) {
        var list = fieldsFor(fields, 'object', entry.kind);
        var url = '/api/agents/' + encodeURIComponent(agent) + '/plugins/' + encodeURIComponent(plugin) + '/object-config';
        var d = openDialog(t('Configure {name}', { name: entry.name || entry.path }),
            '<form class="plugin-object-form">' +
            '<p class="form-hint">' + esc(t('These settings stay on agent {agent}, encrypted. Jobs and the server never hold them.', { agent: agent })) + '</p>' +
            '<div class="plugin-object-fields"><p class="form-hint">' + esc(t('Loading...')) + '</p></div>' +
            '<div class="plugin-error" style="display:none"></div>' +
            '<div class="plugin-ok" style="display:none"></div>' +
            '<div class="dialog-actions">' +
            '<button type="button" class="btn btn-secondary plugin-object-remove" hidden>' + esc(t('Use the agent account')) + '</button>' +
            '<button type="submit" class="btn">' + esc(t('Test and save')) + '</button></div></form>');
        d.el.classList.add('dialog-stacked');
        var form = d.q('form');
        var box = d.q('.plugin-object-fields');
        var errEl = d.q('.plugin-error');
        var okEl = d.q('.plugin-ok');
        var submit = form.querySelector('button[type="submit"]');
        var removeBtn = d.q('.plugin-object-remove');

        api('GET', url + '?path=' + encodeURIComponent(entry.path)).then(function (cfg) {
            renderFields(box, list, cfg.configured ? cfg.values : {}, cfg.secrets_set);
            removeBtn.hidden = !cfg.configured;
            var first = box.querySelector('select, input');
            if (first) first.focus();
        }).catch(function (err) { box.innerHTML = ''; showError(errEl, err); });

        function busy(on, label) {
            submit.disabled = on;
            removeBtn.disabled = on;
            submit.textContent = on ? label : t('Test and save');
        }

        form.addEventListener('submit', function (ev) {
            ev.preventDefault();
            showError(errEl, null);
            okEl.style.display = 'none';
            busy(true, t('Testing connection...'));
            api('PUT', url, { path: entry.path, values: collectFields(box, list) }).then(function (res) {
                busy(false);
                okEl.textContent = t('Connection OK, settings saved.') + (res && res.message ? ' (' + res.message + ')' : '');
                okEl.style.display = 'block';
                onDone(true);
                setTimeout(d.close, 900);
            }).catch(function (err) { busy(false); showError(errEl, err); });
        });

        removeBtn.addEventListener('click', function () {
            if (!confirm(t('Remove the saved credentials of {name}? The agent account will be used.', { name: entry.name || entry.path }))) return;
            busy(true, t('Saving...'));
            api('PUT', url, { path: entry.path, remove: true }).then(function () {
                onDone(false);
                d.close();
            }).catch(function (err) { busy(false); showError(errEl, err); });
        });
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

    // --- Restores: workload restore ---

    var KIND_LABEL = { full: 'Full', differential: 'Differential', log: 'Log' };

    function fmtTime(s) {
        var d = new Date(s);
        return isNaN(d) ? '' : d.toLocaleString(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
    }

    // toLocalInput formats a time for a datetime-local input.
    function toLocalInput(s) {
        var d = new Date(s);
        return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 19);
    }

    // workloadRestore renders the restore of a plugin backup, the way
    // database tools do it: a tree of what was backed up (agent → instance →
    // objects), the backup sets of the chosen object and the point to
    // restore to, where and as what to restore, and a preview from the
    // plugin of the files it will write and anything to watch out for.
    // opts: plugin, sourceAgent, snapshot (only that snapshot's objects).
    function workloadRestore(el, opts) {
        var plugin = opts.plugin;
        el.innerHTML =
            '<div class="wr">' +
            '<div class="wr-col wr-left">' +
            '<div class="wr-card"><div class="wr-card-title">' + esc(t('Restore tree')) + '</div><div class="wr-tree plugin-tree-box"><p class="form-hint">' + esc(t('Loading...')) + '</p></div></div>' +
            '<div class="wr-card wr-sets" hidden><div class="wr-card-title">' + esc(t('Backup sets')) + '</div>' +
            '<div class="wr-when">' +
            '<label><input type="radio" name="wr-when" value="set" checked> ' + esc(t('The selected backup')) + '</label>' +
            '<label><input type="radio" name="wr-when" value="latest"> ' + esc(t('Latest point')) + '</label>' +
            '<label><input type="radio" name="wr-when" value="at"> ' + esc(t('Point in time')) + '</label> ' +
            '<input type="datetime-local" class="wr-at" step="1" disabled></div>' +
            '<div class="table-wrap"><table class="wr-set-table"><thead><tr><th></th><th>' + esc(t('Kind')) + '</th><th>' + esc(t('Time')) + '</th><th>' +
            esc(t('Size')) + '</th><th>' + esc(t('Snapshot')) + '</th></tr></thead><tbody></tbody></table></div></div>' +
            '</div>' +
            '<div class="wr-col wr-right" hidden>' +
            '<div class="wr-card"><div class="wr-card-title">' + esc(t('Destination')) + '</div>' +
            '<div class="form-group"><label>' + esc(t('Restore on agent')) + '</label><select class="wr-agent"></select></div>' +
            '<div class="wr-fields"></div></div>' +
            '<div class="wr-card"><div class="wr-card-title">' + esc(t('Files')) + '</div>' +
            '<div class="table-wrap"><table class="wr-files"><thead><tr><th>' + esc(t('Logical name')) + '</th><th>' + esc(t('Kind')) + '</th><th>' +
            esc(t('Original path')) + '</th><th>' + esc(t('Restore as')) + '</th></tr></thead><tbody></tbody></table></div>' +
            '<button type="button" class="sched-link wr-reset-files" hidden>' + esc(t('Reset file paths')) + '</button></div>' +
            '<div class="wr-card wr-preview"><div class="wr-card-title">' + esc(t('Summary')) + '</div><div class="wr-preview-body"><p class="form-hint">—</p></div></div>' +
            '<div class="plugin-error" style="display:none"></div>' +
            '<div class="plugin-progress"></div>' +
            '<div class="dialog-actions"><button type="button" class="btn wr-start" disabled>' + esc(t('Start Restore')) + '</button></div>' +
            '</div></div>';
        var q = function (s) { return el.querySelector(s); };
        var errEl = q('.plugin-error');
        var catalog = [], points = [], current = null, fields = [], agents = [], overrides = {}, pending = 0, timer = null, lastPlan = null;

        function restoreFields() { return fieldsFor(fields, 'restore'); }

        function when() {
            var w = el.querySelector('input[name="wr-when"]:checked').value;
            if (w === 'latest') return null;
            if (w === 'at') return q('.wr-at').value ? new Date(q('.wr-at').value).toISOString() : '';
            var sel = el.querySelector('input[name="wr-set"]:checked');
            return sel ? sel.getAttribute('data-time') : '';
        }

        function body() {
            var options = collectFields(q('.wr-fields'), restoreFields());
            Object.keys(overrides).forEach(function (k) { options['file:' + k] = overrides[k]; });
            var b = { agent_name: q('.wr-agent').value, source_agent: opts.sourceAgent, plugin: plugin, object: current, options: options };
            var w = when();
            if (w) b.target = w;
            return b;
        }

        function preview() {
            clearTimeout(timer);
            timer = setTimeout(runPreview, 350);
        }

        function runPreview() {
            if (!current) return;
            var w = when();
            var box = q('.wr-preview-body');
            if (w === '') {
                box.innerHTML = '<p class="form-hint">' + esc(t('Choose the point in time.')) + '</p>';
                q('.wr-start').disabled = true;
                return;
            }
            var mine = ++pending;
            box.innerHTML = '<p class="form-hint">' + esc(t('Checking the restore...')) + '</p>';
            q('.wr-start').disabled = true;
            api('POST', '/api/workload/restore-plan', body()).then(function (plan) {
                if (mine !== pending) return;
                lastPlan = plan;
                var html = '<p><strong>' + esc(plan.summary || '') + '</strong></p>';
                (plan.warnings || []).forEach(function (wn) { html += '<p class="wr-warning">' + esc(wn) + '</p>'; });
                html += '<ol class="wr-plan">' + (plan.items || []).map(function (it) {
                    return '<li>' + esc(t(KIND_LABEL[it.kind] || it.kind || 'Full')) + ' · ' + esc(fmtTime(it.created_at)) + ' · ' + esc(formatBytes(it.size)) + '</li>';
                }).join('') + '</ol>';
                box.innerHTML = html;
                renderFiles(plan.files || []);
                q('.wr-start').disabled = false;
            }).catch(function (err) {
                if (mine !== pending) return;
                lastPlan = null;
                box.innerHTML = '<p class="plugin-error">' + esc(err.message) + '</p>';
            });
        }

        function renderFiles(files) {
            var tb = q('.wr-files tbody');
            if (!files.length) {
                tb.innerHTML = '<tr><td colspan="4" class="form-hint">' + esc(t('The plugin reported no files.')) + '</td></tr>';
                return;
            }
            var focused = document.activeElement && document.activeElement.getAttribute('data-file');
            tb.innerHTML = files.map(function (f) {
                return '<tr><td><code>' + esc(f.key) + '</code></td><td>' + esc(f.kind || '') + '</td><td class="wr-path">' + esc(f.source || '') + '</td>' +
                    '<td><input type="text" class="wr-file-target' + (overrides[f.key] ? ' wr-edited' : '') + '" data-file="' + esc(f.key) + '" value="' + esc(f.target) + '" spellcheck="false"></td></tr>';
            }).join('');
            q('.wr-reset-files').hidden = !Object.keys(overrides).length;
            if (focused) {
                var again = tb.querySelector('[data-file="' + CSS.escape(focused) + '"]');
                if (again) again.focus();
            }
        }

        function selectObject(obj) {
            current = obj;
            overrides = {};
            points = catalog.filter(function (p) { return p.object === obj; });
            var tb = q('.wr-set-table tbody');
            // Start from this snapshot's backup, else the newest.
            var pick = 0;
            points.forEach(function (p, i) { if (opts.snapshot && p.snapshot_id.indexOf(opts.snapshot) === 0 && pick === 0) pick = i; });
            tb.innerHTML = points.map(function (p, i) {
                return '<tr><td><input type="radio" name="wr-set" data-time="' + esc(p.created_at) + '"' + (i === pick ? ' checked' : '') + '></td><td>' +
                    esc(t(KIND_LABEL[p.kind] || p.kind || 'Full')) + '</td><td>' + esc(fmtTime(p.created_at)) + '</td><td>' + esc(formatBytes(p.size)) + '</td><td><code>' +
                    esc(p.snapshot_id.slice(0, 8)) + '</code></td></tr>';
            }).join('');
            q('.wr-sets').hidden = false;
            q('.wr-right').hidden = false;
            fillInstances();
            var db = q('.wr-fields [data-key="database"]');
            if (db) db.placeholder = obj.split('/').pop();
            preview();
        }

        // tree draws agent → first path segment (an instance) → objects.
        function tree(list) {
            var byObject = {};
            list.forEach(function (p) { if (!byObject[p.object]) byObject[p.object] = p; });
            var groups = {};
            Object.keys(byObject).sort().forEach(function (o) {
                var g = o.indexOf('/') > 0 ? o.slice(0, o.indexOf('/')) : '';
                (groups[g] = groups[g] || []).push(o);
            });
            var row = function (icon, label, extra) {
                return '<div class="ptree-row"><span class="ptree-toggle-spacer"></span><span class="ptree-icon ptree-icon-' + icon + '">' + ICONS[icon] + '</span><span class="ptree-name">' + label + '</span>' + (extra || '') + '</div>';
            };
            var html = '<div class="ptree-node ptree-open">' + row('instance', esc(opts.sourceAgent), '<span class="ptree-detail">' + esc(t('Agent')) + '</span>') + '<div class="ptree-children">';
            Object.keys(groups).forEach(function (g) {
                html += '<div class="ptree-node ptree-open">' + (g ? row('instance', esc(g)) : '') + '<div class="' + (g ? 'ptree-children' : '') + '">';
                groups[g].forEach(function (o) {
                    var p = byObject[o];
                    var id = 'wr-o-' + Math.random().toString(36).slice(2, 8);
                    html += '<div class="ptree-node"><div class="ptree-row"><span class="ptree-toggle-spacer"></span><input class="ptree-check" type="radio" name="wr-object" id="' + id + '" value="' + esc(o) + '">' +
                        '<span class="ptree-icon ptree-icon-database">' + ICONS.database + '</span><label class="ptree-name" for="' + id + '">' + esc(g ? o.slice(g.length + 1) : o) + '</label>' +
                        '<span class="ptree-detail">' + esc(t(KIND_LABEL[p.kind] || p.kind || 'Full')) + ' · ' + esc(fmtTime(p.created_at)) + ' · ' + esc(formatBytes(p.size)) + '</span></div></div>';
                });
                html += '</div></div>';
            });
            html += '</div></div>';
            q('.wr-tree').innerHTML = html;
            var radios = el.querySelectorAll('input[name="wr-object"]');
            if (radios.length === 1) {
                radios[0].checked = true;
                selectObject(radios[0].value);
            }
        }

        // fillInstances lists the target agent's top-level objects in the
        // fields that take them as choices.
        function fillInstances() {
            restoreFields().filter(function (f) { return f.choices_from === 'browse'; }).forEach(function (f) {
                var sel = q('.wr-fields [data-key="' + CSS.escape(f.key) + '"]');
                if (!sel) return;
                var origin = current ? current.split('/')[0] : '';
                sel.innerHTML = '<option value="">' + esc(t('Original ({name})', { name: origin })) + '</option>';
                api('GET', '/api/agents/' + encodeURIComponent(q('.wr-agent').value) + '/plugins/' + encodeURIComponent(plugin) + '/browse?path=').then(function (entries) {
                    entries.forEach(function (e) {
                        if (e.path === origin) return;
                        var o = document.createElement('option');
                        o.value = e.path;
                        o.textContent = e.name + (e.detail ? ' — ' + e.detail : '');
                        sel.appendChild(o);
                    });
                }).catch(function () { /* the original instance stays available */ });
            });
        }

        function agentChanged() {
            var a = agents.filter(function (x) { return x.name === q('.wr-agent').value; })[0];
            var p = a ? a.plugins.filter(function (x) { return x.id === plugin; })[0] : null;
            fields = p ? p.fields || [] : [];
            renderFields(q('.wr-fields'), restoreFields());
            if (current) {
                var db = q('.wr-fields [data-key="database"]');
                if (db) db.placeholder = current.split('/').pop();
            }
            fillInstances();
            preview();
        }

        el.addEventListener('change', function (ev) {
            var tg = ev.target;
            if (tg.name === 'wr-object') selectObject(tg.value);
            else if (tg.classList.contains('wr-agent')) { agentChanged(); return; }
            else if (tg.name === 'wr-when') q('.wr-at').disabled = tg.value !== 'at';
            else if (tg.name === 'wr-set') el.querySelector('input[name="wr-when"][value="set"]').checked = true;
            if (tg.classList.contains('wr-file-target')) {
                // A path typed by hand sticks; an emptied one goes back to the plugin's.
                if (tg.value.trim()) overrides[tg.getAttribute('data-file')] = tg.value.trim();
                else delete overrides[tg.getAttribute('data-file')];
            }
            if (tg.name !== 'wr-object') preview();
        });
        el.addEventListener('input', function (ev) {
            if (ev.target.getAttribute('data-key') === 'database') {
                // A new name renames the files too, unless set by hand.
                preview();
            }
        });
        q('.wr-reset-files').addEventListener('click', function () {
            overrides = {};
            preview();
        });

        q('.wr-start').addEventListener('click', function () {
            var b = body();
            var name = (b.options.database || current.split('/').pop());
            var warn = lastPlan && (lastPlan.warnings || []).length ? '\n\n' + lastPlan.warnings.join('\n') : '';
            if (!confirm(t('Restore {object} as {name} on {agent}?', { object: current, name: name, agent: b.agent_name }) + warn)) return;
            showError(errEl, null);
            q('.wr-start').disabled = true;
            api('POST', '/api/workload/restores', b).then(function (sess) { poll(sess.id); })
                .catch(function (err) { q('.wr-start').disabled = false; showError(errEl, err); });
        });

        function poll(id) {
            var box = q('.plugin-progress');
            api('GET', '/api/workload/restores/' + encodeURIComponent(id)).then(function (s) {
                var pct = s.bytes_total ? Math.floor(100 * s.bytes_sent / s.bytes_total) : 0;
                var html = '<p><span class="badge ' + (s.status === 'completed' ? 'badge-ok' : s.status === 'failed' ? 'badge-critical' : 'badge-running') + '">' +
                    esc(t(s.status)) + '</span> ' + (s.bytes_total ? esc(formatBytes(s.bytes_sent) + ' / ' + formatBytes(s.bytes_total) + ' (' + pct + '%)') : '') + '</p>';
                if (s.bytes_total) html += '<div class="wr-bar"><div style="width:' + pct + '%"></div></div>';
                if (s.message) html += '<p>' + esc(s.message) + '</p>';
                if (s.error) html += '<p class="plugin-error">' + esc(s.error) + '</p>';
                box.innerHTML = html;
                if (s.status !== 'completed' && s.status !== 'failed') setTimeout(function () { poll(id); }, 1500);
                else q('.wr-start').disabled = false;
            }).catch(function (err) { showError(errEl, err); });
        }

        var catURL = '/api/workload/catalog?agent=' + encodeURIComponent(opts.sourceAgent) + '&plugin=' + encodeURIComponent(plugin);
        Promise.all([api('GET', catURL), agentsWithPlugins()]).then(function (res) {
            catalog = res[0] || [];
            agents = (res[1] || []).filter(function (a) { return a.plugins.some(function (p) { return p.id === plugin; }); });
            if (!agents.length) {
                showError(errEl, new Error(t('No agent has the {plugin} plugin installed.', { plugin: plugin })));
                q('.wr-tree').innerHTML = '';
                return;
            }
            q('.wr-agent').innerHTML = agents.map(function (a) {
                return '<option' + (a.name === opts.sourceAgent ? ' selected' : '') + '>' + esc(a.name) + '</option>';
            }).join('');
            var shown = opts.snapshot ? catalog.filter(function (p) { return p.snapshot_id.indexOf(opts.snapshot) === 0; }) : catalog;
            if (!shown.length) {
                q('.wr-tree').innerHTML = '<p class="form-hint">' + esc(t('No backups of this object were found.')) + '</p>';
                return;
            }
            agentChanged();
            tree(shown);
        }).catch(function (err) {
            q('.wr-tree').innerHTML = '<p class="plugin-error">' + esc(err.message) + '</p>';
        });
    }

    function openRestoreDialog() {
        var d = openDialog(t('Restore from a Plugin Backup'),
            '<div class="wr-pick"><div class="form-group"><label>' + esc(t('Backups from agent')) + '</label><select class="wr-src"></select></div>' +
            '<div class="form-group"><label>' + esc(t('Plugin')) + '</label><select class="wr-plugin"></select></div></div>' +
            '<div class="wr-host"></div>');
        d.el.querySelector('.dialog-box').classList.add('dialog-xl');
        var all = [];
        function mountIt() {
            var src = d.q('.wr-src').value, plugin = d.q('.wr-plugin').value;
            if (src && plugin) workloadRestore(d.q('.wr-host'), { plugin: plugin, sourceAgent: src });
        }
        function srcChanged() {
            var a = all.filter(function (x) { return x.name === d.q('.wr-src').value; })[0];
            d.q('.wr-plugin').innerHTML = (a && a.plugins ? a.plugins : []).map(function (p) { return '<option value="' + esc(p.id) + '">' + esc(p.name || p.id) + '</option>'; }).join('');
            mountIt();
        }
        d.q('.wr-src').addEventListener('change', srcChanged);
        d.q('.wr-plugin').addEventListener('change', mountIt);
        api('GET', '/api/agents').then(function (agents) {
            all = (agents || []).filter(function (a) { return a.plugins && a.plugins.length; });
            if (!all.length) {
                d.q('.wr-host').innerHTML = '<p class="plugin-error">' + esc(t('No agent has reported a plugin yet. Install one from the agent page.')) + '</p>';
                return;
            }
            d.q('.wr-src').innerHTML = all.map(function (a) { return '<option>' + esc(a.name) + '</option>'; }).join('');
            srcChanged();
        }).catch(function (err) { d.q('.wr-host').innerHTML = '<p class="plugin-error">' + esc(err.message) + '</p>'; });
    }

    window.HitStorPlugins = {
        agentPanel: agentPanel,
        packagesPanel: packagesPanel,
        // Plugin jobs are created in the backup job wizard.
        openJobDialog: function () { HitStorBackupJob.openWizard(); },
        openRestoreDialog: openRestoreDialog,
        workloadRestore: workloadRestore,
        selectionTree: selectionTree,
        backupSources: backupSources,
        describeSource: describeSource,
        ui: {
            esc: esc, api: api, formatBytes: formatBytes, fieldsFor: fieldsFor, renderFields: renderFields,
            collectFields: collectFields, openDialog: openDialog, showError: showError
        }
    };
})();
