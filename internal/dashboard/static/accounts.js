// User accounts in the dashboard: re-authentication before sensitive
// actions (any page), session timeouts, the user menu, the user's own
// account, user administration and the audit log.
(function () {
    'use strict';

    function esc(s) {
        return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
    }

    function dialog(title, bodyHTML) {
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

    // --- Re-authentication: the server answers 403 with X-HitStor-Reauth
    // when a sensitive action needs a recent sign-in; ask for the password
    // and a code, then repeat the request.
    var nativeFetch = window.fetch.bind(window);

    function reauth() {
        return new Promise(function (resolve) {
            var d = dialog(t('Confirm it is you'),
                '<form class="reauth-form">' +
                '<p class="form-hint">' + esc(t('This action needs your password and a code from your authenticator app.')) + '</p>' +
                '<div class="form-group"><label>' + esc(t('Password')) + '</label><input type="password" class="ra-password" autocomplete="current-password" required></div>' +
                '<div class="form-group"><label>' + esc(t('Code')) + '</label><input type="text" class="ra-code" inputmode="numeric" autocomplete="one-time-code" required maxlength="23"></div>' +
                '<div class="form-error" style="display:none"></div>' +
                '<div class="dialog-actions"><button type="button" class="btn btn-secondary ra-cancel">' + esc(t('Cancel')) + '</button>' +
                '<button type="submit" class="btn">' + esc(t('Confirm')) + '</button></div></form>');
            var done = function (ok) { d.close(); resolve(ok); };
            d.q('.dialog-close').addEventListener('click', function () { resolve(false); });
            d.q('.ra-cancel').addEventListener('click', function () { done(false); });
            d.q('.ra-password').focus();
            d.q('form').addEventListener('submit', function (ev) {
                ev.preventDefault();
                nativeFetch('/api/reauth', {
                    method: 'POST', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ password: d.q('.ra-password').value, code: d.q('.ra-code').value })
                }).then(function (r) {
                    if (r.ok) return done(true);
                    var e = d.q('.form-error');
                    e.textContent = t('Wrong password or code.');
                    e.style.display = 'block';
                });
            });
        });
    }

    window.fetch = function (input, init) {
        return nativeFetch(input, init).then(function (r) {
            if (r.status !== 403 || !r.headers.get('X-HitStor-Reauth')) return r;
            return reauth().then(function (ok) { return ok ? nativeFetch(input, init) : r; });
        });
    };

    // --- Session timeout: auto-refresh does not keep a session alive, so
    // report the user's own activity and warn before the session ends.
    function sessionWatch() {
        var WARN = 120, PING_EVERY = 60000;
        var deadline = 0, absolute = false, lastPing = Date.now(), warning = null;

        function fmt(sec) {
            sec = Math.max(0, Math.round(sec));
            return Math.floor(sec / 60) + ':' + String(sec % 60).padStart(2, '0');
        }
        function ended() { location.href = '/login?expired=1'; }
        function sync(r) {
            if (r.status === 401) return ended();
            if (!r.ok) return;
            return r.json().then(function (s) {
                if (s.accounts === false) return;
                deadline = Date.now() + s.expires_in * 1000;
                absolute = s.absolute;
                if (warning && deadline - Date.now() > WARN * 1000) closeWarning();
            });
        }
        function check() { return nativeFetch('/api/session').then(sync); }
        function ping() {
            lastPing = Date.now();
            return nativeFetch('/api/session/activity', { method: 'POST' }).then(sync);
        }
        function closeWarning() {
            if (warning) warning.close();
            warning = null;
        }
        function showWarning() {
            warning = dialog(t('Your session is about to end'),
                '<p class="session-warning-text"></p>' +
                '<div class="dialog-actions">' +
                '<button type="button" class="btn btn-secondary sw-logout">' + esc(t('Sign out')) + '</button>' +
                (absolute ? '' : '<button type="button" class="btn sw-stay">' + esc(t('Stay signed in')) + '</button>') +
                '</div>');
            warning.q('.dialog-close').addEventListener('click', function () { warning = null; });
            warning.q('.sw-logout').addEventListener('click', function () {
                nativeFetch('/logout', { method: 'POST' }).then(function () { location.href = '/login'; });
            });
            var stay = warning.q('.sw-stay');
            if (stay) {
                stay.addEventListener('click', function () { ping().then(closeWarning); });
                stay.focus();
            }
            tick();
        }
        function tick() {
            var left = (deadline - Date.now()) / 1000;
            if (warning) {
                warning.q('.session-warning-text').textContent = absolute
                    ? t('Sessions last at most 12 hours. This one ends in {time}: save your work, then sign in again.', { time: fmt(left) })
                    : t('Because of inactivity, your session ends in {time}.', { time: fmt(left) });
            }
            if (left <= 0) {
                // Another tab may have kept it alive.
                check().then(function () { if (deadline <= Date.now()) ended(); });
            } else if (left <= WARN && !warning) {
                check().then(function () { if (!warning && deadline - Date.now() <= WARN * 1000) showWarning(); });
            }
        }
        function onActivity() {
            if (warning || absolute || Date.now() - lastPing < PING_EVERY) return;
            ping();
        }
        ['keydown', 'mousedown', 'wheel', 'touchstart'].forEach(function (ev) {
            document.addEventListener(ev, onActivity, { passive: true, capture: true });
        });
        check().then(function () { setInterval(tick, 1000); });
    }

    function api(method, url, body) {
        var init = { method: method, headers: {} };
        if (body !== undefined) {
            init.headers['Content-Type'] = 'application/json';
            init.body = JSON.stringify(body);
        }
        return window.fetch(url, init).then(function (r) {
            if (!r.ok) return r.text().then(function (m) { throw new Error(m || r.statusText); });
            return r.status === 204 ? null : r.json();
        });
    }

    var LANG = document.documentElement.lang || undefined;

    // when is a date and time in the page's language.
    function when(iso) {
        if (!iso) return '—';
        return new Date(iso).toLocaleString(LANG, { dateStyle: 'medium', timeStyle: 'short' });
    }

    // ago is a time relative to now ("5 minutes ago", "yesterday"), or the
    // date when it is long past.
    function ago(iso) {
        if (!iso) return t('never');
        var sec = (new Date(iso).getTime() - Date.now()) / 1000;
        var abs = Math.abs(sec);
        if (abs > 30 * 86400) return new Date(iso).toLocaleDateString(LANG, { dateStyle: 'medium' });
        var rtf = new Intl.RelativeTimeFormat(LANG, { numeric: 'auto' });
        if (abs < 60) return rtf.format(0, 'second');
        if (abs < 3600) return rtf.format(Math.round(sec / 60), 'minute');
        if (abs < 86400) return rtf.format(Math.round(sec / 3600), 'hour');
        return rtf.format(Math.round(sec / 86400), 'day');
    }

    function relTimes(root) {
        (root || document).querySelectorAll('time[data-rel]').forEach(function (el) {
            el.textContent = ago(el.getAttribute('datetime'));
            el.title = when(el.getAttribute('datetime'));
        });
    }

    function roleName(role) {
        return {
            admin: t('Administrator'), backup_operator: t('Backup operator'),
            restore_operator: t('Restore operator'), viewer: t('Viewer')
        }[role] || role;
    }

    function roleHelp(role) {
        return {
            admin: t('Everything, on every agent: users, storage, settings, deleting snapshots.'),
            backup_operator: t('Runs, schedules and edits backup jobs on the agents it is given. Cannot restore.'),
            restore_operator: t('Browses snapshots and restores on the agents it is given. Cannot change jobs.'),
            viewer: t('Reads jobs, results and reports. Changes nothing.')
        }[role] || '';
    }

    function allAgents(g) { return g.role === 'admin' || !(g.agents && g.agents.length); }

    function grantsHTML(grants) {
        return (grants || []).map(function (g) {
            return '<div><span class="role-chip role-chip--' + esc(g.role) + '">' + esc(roleName(g.role)) + '</span> <span class="role-scope">' +
                esc(allAgents(g) ? t('all agents') : g.agents.join(', ')) + '</span></div>';
        }).join('');
    }

    function initials(u) {
        var words = (u.display_name || '').trim().split(/\s+/).filter(Boolean);
        if (!words.length) words = u.username.split(/[._-]/).filter(Boolean);
        var s = words.length > 1 ? words[0][0] + words[words.length - 1][0] : (words[0] || '?').slice(0, 2);
        return s.toUpperCase();
    }

    // status is where an account stands: disabled, locked, waiting for its
    // first sign-in, or active.
    function status(u) {
        if (u.disabled) return 'disabled';
        if (u.locked) return 'locked';
        if (!u.last_login_at || !u.totp_enabled) return 'pending';
        return 'active';
    }

    function statusBadge(u) {
        return {
            active: '<span class="badge badge-ok">' + esc(t('Active user')) + '</span>',
            pending: '<span class="badge badge-pending">' + esc(t('First sign-in pending')) + '</span>',
            locked: '<span class="badge badge-stale">' + esc(t('Locked')) + '</span>',
            disabled: '<span class="badge badge-disabled">' + esc(t('Disabled')) + '</span>'
        }[status(u)];
    }

    function mfaBadge(u) {
        return u.totp_enabled
            ? '<span class="badge badge-ok">' + esc(t('Set up')) + '</span>'
            : '<span class="badge badge-pending">' + esc(t('Not set up')) + '</span>';
    }

    function showCodes(codes) {
        var d = dialog(t('Your new recovery codes'),
            '<p class="form-hint">' + esc(t('Each code signs you in once if you lose your phone. Your previous codes no longer work.')) + '</p>' +
            '<ol class="recovery-codes">' + codes.map(function (c) { return '<li><code>' + esc(c) + '</code></li>'; }).join('') + '</ol>' +
            '<div class="dialog-actions"><button type="button" class="btn">' + esc(t('Done')) + '</button></div>');
        d.q('.dialog-actions .btn').addEventListener('click', function () { d.close(); location.reload(); });
    }

    // formDialog shows fields and calls submit(values) until it resolves.
    function formDialog(title, fieldsHTML, okLabel, submit) {
        var d = dialog(title, '<form>' + fieldsHTML +
            '<div class="form-error" hidden></div>' +
            '<div class="dialog-actions"><button type="button" class="btn btn-secondary fd-cancel">' + esc(t('Cancel')) + '</button>' +
            '<button type="submit" class="btn">' + esc(okLabel) + '</button></div></form>');
        d.q('.fd-cancel').addEventListener('click', d.close);
        var first = d.q('input, select');
        if (first) first.focus();
        d.q('form').addEventListener('submit', function (ev) {
            ev.preventDefault();
            var err = d.q('.form-error');
            err.hidden = true;
            Promise.resolve().then(function () { return submit(d); }).catch(function (e) {
                err.textContent = e.message;
                err.hidden = false;
            });
        });
        return d;
    }

    function sessionRow(s, endURL, onEnd) {
        var div = document.createElement('div');
        div.className = 'sess';
        div.innerHTML = '<div><strong>' + esc(s.browser) + '</strong>' + (s.current ? ' · ' + esc(t('this browser')) : '') +
            '<div class="set-hint">' + esc(s.ip || '—') + ' · ' + esc(t('signed in {when}', { when: ago(s.created) })) + ' · ' +
            esc(s.current ? t('active now') : t('last activity {when}', { when: ago(s.last_seen) })) + '</div></div>' +
            (s.current ? '<span class="badge badge-ok">' + esc(t('this browser')) + '</span>'
                : '<button type="button" class="btn btn-secondary btn-sm">' + esc(t('Sign out')) + '</button>');
        var b = div.querySelector('button');
        if (b) b.addEventListener('click', function () { api('DELETE', endURL(s.id)).then(onEnd); });
        return div;
    }

    function temporaryPasswordDialog(user, pw, after) {
        var d = dialog(t('Temporary password'),
            '<p>' + esc(t('Give this temporary password to {user}. It works once: they choose their own and set up two-step verification at first sign-in.', { user: user })) + '</p>' +
            '<p class="temp-password"><code>' + esc(pw) + '</code></p>' +
            '<div class="dialog-actions"><button type="button" class="btn">' + esc(t('Done')) + '</button></div>');
        d.q('.dialog-actions .btn').addEventListener('click', function () { d.close(); if (after) after(); });
    }

    // --- My account ---
    function account(root) {
        var name = root.getAttribute('data-username');
        relTimes(root);

        root.querySelector('#acct-edit-name').addEventListener('click', function () {
            formDialog(t('Full name'),
                '<div class="form-group"><label for="fd-name">' + esc(t('Full name')) + '</label><input type="text" id="fd-name" maxlength="100" value="' +
                esc(root.getAttribute('data-display-name')) + '"></div>', t('Save'), function (d) {
                    return api('PUT', '/api/account/profile', { display_name: d.q('#fd-name').value }).then(function () { location.reload(); });
                });
        });

        root.querySelector('#acct-change-password').addEventListener('click', function () {
            formDialog(t('Change password'),
                '<input type="text" name="username" value="' + esc(name) + '" autocomplete="username" hidden>' +
                '<div class="form-group"><label for="fd-cur">' + esc(t('Current password')) + '</label><input type="password" id="fd-cur" autocomplete="current-password" required></div>' +
                '<div class="form-group"><label for="fd-new">' + esc(t('New password')) + '</label><input type="password" id="fd-new" autocomplete="new-password" minlength="12" maxlength="128" required>' +
                '<p class="form-hint">' + esc(t('At least 12 characters. A phrase of several words is easy to remember and hard to guess.')) + '</p></div>' +
                '<div class="form-group"><label for="fd-confirm">' + esc(t('Repeat the new password')) + '</label><input type="password" id="fd-confirm" autocomplete="new-password" required></div>',
                t('Change password'), function (d) {
                    return api('POST', '/api/account/password', {
                        current: d.q('#fd-cur').value, password: d.q('#fd-new').value, confirm: d.q('#fd-confirm').value
                    }).then(function () {
                        d.close();
                        var ok = dialog(t('Password changed'), '<p>' + esc(t('Password changed. Your other sessions were closed.')) + '</p>' +
                            '<div class="dialog-actions"><button type="button" class="btn">' + esc(t('Done')) + '</button></div>');
                        ok.q('.dialog-actions .btn').addEventListener('click', function () { location.reload(); });
                    });
                });
        });

        root.querySelector('#account-codes').addEventListener('click', function () {
            if (!confirm(t('Generate new recovery codes? The ones you have stop working.'))) return;
            api('POST', '/api/account/recovery-codes').then(function (r) { showCodes(r.codes); })
                .catch(function (err) { alert(err.message); });
        });

        var box = root.querySelector('#account-sessions');
        var others = root.querySelector('#acct-end-others');
        function sessions() {
            api('GET', '/api/account/sessions').then(function (list) {
                box.innerHTML = '';
                list.forEach(function (s) {
                    box.appendChild(sessionRow(s, function (id) { return '/api/account/sessions/' + encodeURIComponent(id); }, sessions));
                });
                others.hidden = list.length < 2;
            });
        }
        others.addEventListener('click', function () { api('DELETE', '/api/account/sessions').then(sessions); });
        sessions();
    }

    // grantsEditor edits a user's roles as cards: a role, its explanation,
    // and every agent or some of them.
    function grantsEditor(el, agents, roles, grants, onChange) {
        var seq = 0;
        function card(g) {
            var id = 'g' + (++seq);
            var div = document.createElement('div');
            div.className = 'grant-card';
            div.innerHTML = '<div class="gc-head"><select class="grant-role" aria-label="' + esc(t('Role')) + '">' + roles.map(function (r) {
                return '<option value="' + esc(r) + '"' + (r === g.role ? ' selected' : '') + '>' + esc(roleName(r)) + '</option>';
            }).join('') + '</select><button type="button" class="grant-remove" aria-label="' + esc(t('Remove role')) + '">&times;</button></div>' +
                '<p class="set-hint gc-help"></p>' +
                '<div class="seg" role="radiogroup">' +
                '<label><input type="radio" name="' + id + '" value="all"' + (allAgents(g) ? ' checked' : '') + '> ' + esc(t('All agents')) + '</label>' +
                '<label><input type="radio" name="' + id + '" value="some"' + (allAgents(g) ? '' : ' checked') + '> ' + esc(t('Some')) + '</label></div>' +
                '<div class="agent-picks">' + (agents.length ? agents.map(function (a) {
                    return '<label><input type="checkbox" value="' + esc(a) + '"' + ((g.agents || []).indexOf(a) >= 0 ? ' checked' : '') + '> ' + esc(a) + '</label>';
                }).join('') : '<span class="set-hint">' + esc(t('No agents registered yet.')) + '</span>') + '</div>';
            function sync() {
                var role = div.querySelector('.grant-role').value;
                var isAdmin = role === 'admin';
                div.querySelector('.gc-help').textContent = roleHelp(role);
                if (isAdmin) div.querySelector('input[value=all]').checked = true;
                div.querySelector('.seg').hidden = isAdmin;
                var some = div.querySelector('input[value=some]').checked;
                div.querySelector('.agent-picks').hidden = isAdmin || !some;
                div.querySelectorAll('.seg label').forEach(function (l) { l.classList.toggle('on', l.querySelector('input').checked); });
                div.querySelectorAll('.agent-picks label').forEach(function (l) { l.classList.toggle('on', l.querySelector('input').checked); });
                if (onChange) onChange();
            }
            div.addEventListener('change', sync);
            div.querySelector('.grant-remove').addEventListener('click', function () { div.remove(); if (onChange) onChange(); });
            el.appendChild(div);
            sync();
        }
        el.innerHTML = '';
        (grants && grants.length ? grants : [{ role: 'viewer' }]).forEach(card);
        function value(strict) {
            var out = [];
            el.querySelectorAll('.grant-card').forEach(function (div) {
                var g = { role: div.querySelector('.grant-role').value };
                if (g.role !== 'admin' && div.querySelector('input[value=some]').checked) {
                    g.agents = [];
                    div.querySelectorAll('.agent-picks input:checked').forEach(function (i) { g.agents.push(i.value); });
                    if (!g.agents.length && strict) throw new Error(t('Choose at least one agent, or All agents.'));
                }
                out.push(g);
            });
            if (!out.length && strict) throw new Error(t('Give the user at least one role.'));
            return out;
        }
        return { add: function () { card({ role: 'backup_operator' }); }, value: function () { return value(true); }, draft: function () { return value(false); } };
    }

    // summary says in plain words what a set of roles allows.
    function summary(grants) {
        if (grants.some(function (g) { return g.role === 'admin'; })) return t('Can do everything, on every agent.');
        function scope(roles) {
            var all = false, set = {};
            grants.forEach(function (g) {
                if (roles.indexOf(g.role) < 0) return;
                if (allAgents(g)) all = true;
                else g.agents.forEach(function (a) { set[a] = true; });
            });
            if (all) return t('all agents');
            var list = Object.keys(set);
            return list.length ? list.join(', ') : '';
        }
        var parts = [];
        var sees = scope(['viewer', 'backup_operator', 'restore_operator']);
        var backs = scope(['backup_operator']);
        var restores = scope(['restore_operator']);
        parts.push(sees ? t('Sees: {agents}', { agents: sees }) : t('Sees nothing'));
        parts.push(backs ? t('Backs up: {agents}', { agents: backs }) : t('Does not back up'));
        parts.push(restores ? t('Restores: {agents}', { agents: restores }) : t('Does not restore'));
        parts.push(t('Does not administer'));
        return parts.join(' · ');
    }

    // --- Users ---
    function usersList(root) {
        var agents = JSON.parse(root.getAttribute('data-agents') || 'null') || [];
        var roles = JSON.parse(root.getAttribute('data-roles') || 'null') || [];
        var body = root.querySelector('#users-body');
        var search = root.querySelector('#users-search');
        var roleSel = root.querySelector('#users-role');
        var statusSel = root.querySelector('#users-status');
        var all = [];

        function render() {
            var q = search.value.trim().toLowerCase();
            var list = all.filter(function (u) {
                if (q && (u.username + ' ' + (u.display_name || '')).toLowerCase().indexOf(q) < 0) return false;
                if (roleSel.value && !u.grants.some(function (g) { return g.role === roleSel.value; })) return false;
                if (statusSel.value && status(u) !== statusSel.value) return false;
                return true;
            });
            body.innerHTML = list.map(function (u) {
                return '<tr class="clickable" data-user="' + esc(u.username) + '" tabindex="0">' +
                    '<td><div class="who"><span class="um-avatar" aria-hidden="true">' + esc(initials(u)) + '</span><div><strong>' + esc(u.username) + '</strong>' +
                    (u.display_name ? '<div class="set-hint">' + esc(u.display_name) + '</div>' : '') + '</div></div></td>' +
                    '<td>' + grantsHTML(u.grants) + '</td><td>' + mfaBadge(u) + '</td>' +
                    '<td title="' + esc(when(u.last_login_at)) + '">' + esc(ago(u.last_login_at)) + '</td>' +
                    '<td>' + statusBadge(u) + '</td><td class="col-actions" aria-hidden="true">›</td></tr>';
            }).join('') || '<tr><td colspan="6" class="text-muted">' + esc(all.length ? t('No user matches the filters.') : t('No users yet.')) + '</td></tr>';
        }
        function open(tr) { location.href = '/users/' + encodeURIComponent(tr.getAttribute('data-user')); }
        body.addEventListener('click', function (ev) { var tr = ev.target.closest('tr[data-user]'); if (tr) open(tr); });
        body.addEventListener('keydown', function (ev) { var tr = ev.target.closest('tr[data-user]'); if (tr && ev.key === 'Enter') open(tr); });
        [search, roleSel, statusSel].forEach(function (el) { el.addEventListener('input', render); });

        api('GET', '/api/users').then(function (list) { all = list; render(); })
            .catch(function (err) { body.innerHTML = '<tr><td colspan="6" class="form-error">' + esc(err.message) + '</td></tr>'; });

        root.querySelector('#user-add').addEventListener('click', function () {
            var d = formDialog(t('New user'),
                '<div class="form-group"><label for="fd-user">' + esc(t('User name')) + '</label><input type="text" id="fd-user" required autocomplete="off" pattern="[A-Za-z0-9][A-Za-z0-9._\\-]{1,63}"></div>' +
                '<div class="form-group"><label for="fd-display">' + esc(t('Full name (optional)')) + '</label><input type="text" id="fd-display" maxlength="100" autocomplete="off"></div>' +
                '<div class="form-group"><label>' + esc(t('Roles')) + '</label><div class="grants"></div>' +
                '<button type="button" class="btn btn-secondary btn-sm fd-add-grant">+ ' + esc(t('Add role')) + '</button></div>',
                t('Create user'), function (d) {
                    return api('POST', '/api/users', { username: d.q('#fd-user').value, display_name: d.q('#fd-display').value, grants: ed.value() })
                        .then(function (r) {
                            d.close();
                            temporaryPasswordDialog(r.user.username, r.temporary_password, function () {
                                location.href = '/users/' + encodeURIComponent(r.user.username);
                            });
                        });
                });
            var ed = grantsEditor(d.q('.grants'), agents, roles, [{ role: 'backup_operator' }]);
            d.q('.fd-add-grant').addEventListener('click', ed.add);
        });
    }

    // --- Audit rows, shared by the audit log and a user's activity ---
    function auditRows(events, withUser) {
        var icon = { ok: '<span class="ev ev-ok" title="' + esc(t('Succeeded')) + '">✓</span>', failed: '<span class="ev ev-fail" title="' + esc(t('Failed')) + '">!</span>',
            denied: '<span class="ev ev-deny" title="' + esc(t('Denied')) + '">⊘</span>' };
        return events.map(function (e) {
            var phrase = esc(e.phrase).split('{target}').join('<strong>' + esc(e.target) + '</strong>');
            return '<tr><td>' + (icon[e.result] || '') + '</td><td class="nowrap" title="' + esc(when(e.time)) + '">' + esc(ago(e.time)) + '</td>' +
                (withUser ? '<td>' + esc(e.user || '—') + '</td>' : '') +
                '<td>' + phrase + (e.detail ? '<div class="set-hint">' + esc(e.detail) + '</div>' : '') + '</td>' +
                '<td>' + esc(e.ip || '—') + '</td></tr>';
        }).join('');
    }

    // --- One user ---
    function userDetail(root) {
        var name = root.getAttribute('data-username');
        var enc = encodeURIComponent(name);
        var self = root.getAttribute('data-self') === '1';
        var agents = JSON.parse(root.getAttribute('data-agents') || 'null') || [];
        var roles = JSON.parse(root.getAttribute('data-roles') || 'null') || [];
        var u = JSON.parse(root.getAttribute('data-user'));
        var q = function (s) { return root.querySelector(s); };

        root.querySelectorAll('.tab').forEach(function (tab) {
            tab.addEventListener('click', function () {
                root.querySelectorAll('.tab').forEach(function (x) { x.classList.toggle('on', x === tab); });
                root.querySelectorAll('[data-panel]').forEach(function (p) { p.hidden = p.getAttribute('data-panel') !== tab.getAttribute('data-tab'); });
                if (tab.getAttribute('data-tab') === 'activity') activity();
            });
        });

        function fill() {
            q('#user-last-login').textContent = u.last_login_at ? t('last sign-in {when}', { when: ago(u.last_login_at) }) : t('has not signed in yet');
            q('#user-name').innerHTML = u.display_name ? esc(u.display_name) : '<span class="text-muted">—</span>';
            q('#user-status').innerHTML = statusBadge(u);
            q('#user-password').textContent = u.must_change_password ? t('Temporary: must change it') : t('Changed {when}', { when: ago(u.password_changed_at) });
            q('#user-mfa').innerHTML = mfaBadge(u) + (u.totp_enabled ? ' · ' + esc(t('{n} codes', { n: u.recovery_codes_left })) : '');
            var toggle = q('#user-toggle');
            if (toggle) {
                q('#user-toggle-label').textContent = u.disabled ? t('Enable') : t('Disable');
                q('#user-toggle-hint').textContent = u.disabled ? t('They can sign in again.') : t('They cannot sign in; the account is kept.');
                toggle.textContent = u.disabled ? t('Enable') : t('Disable');
            }
        }
        function reload() { return api('GET', '/api/users/' + enc).then(function (x) { u = x; fill(); }); }
        fill();

        var effective = q('#user-effective');
        var ed = grantsEditor(q('.grants'), agents, roles, u.grants, function () {
            if (ed) effective.innerHTML = '<strong>' + esc(t('In short:')) + '</strong> ' + esc(summary(ed.draft()));
        });
        effective.innerHTML = '<strong>' + esc(t('In short:')) + '</strong> ' + esc(summary(ed.draft()));
        q('#user-add-grant').addEventListener('click', ed.add);
        q('#user-save-roles').addEventListener('click', function () {
            var st = q('#user-roles-status');
            try {
                var grants = ed.value();
            } catch (e) { st.textContent = e.message; st.className = 'form-status text-critical'; return; }
            api('PUT', '/api/users/' + enc, { grants: grants }).then(function (x) {
                u = x; fill();
                st.textContent = t('Saved.'); st.className = 'form-status text-ok';
            }).catch(function (e) { st.textContent = e.message; st.className = 'form-status text-critical'; });
        });

        q('#user-edit-name').addEventListener('click', function () {
            formDialog(t('Full name'),
                '<div class="form-group"><label for="fd-name">' + esc(t('Full name')) + '</label><input type="text" id="fd-name" maxlength="100" value="' + esc(u.display_name) + '"></div>',
                t('Save'), function (d) {
                    return api('PUT', '/api/users/' + enc, { display_name: d.q('#fd-name').value }).then(function () { location.reload(); });
                });
        });
        q('#user-reset-password').addEventListener('click', function () {
            if (!confirm(t('Reset the password of {user}? Their sessions end.', { user: name }))) return;
            api('POST', '/api/users/' + enc + '/password').then(function (r) {
                temporaryPasswordDialog(name, r.temporary_password, reload);
            }).catch(function (e) { alert(e.message); });
        });
        q('#user-reset-mfa').addEventListener('click', function () {
            if (!confirm(t('Reset two-step verification of {user}? They set it up again at next sign-in.', { user: name }))) return;
            api('DELETE', '/api/users/' + enc + '/second-factor').then(reload).catch(function (e) { alert(e.message); });
        });
        var toggle = q('#user-toggle');
        if (toggle) toggle.addEventListener('click', function () {
            api('PUT', '/api/users/' + enc, { disabled: !u.disabled }).then(function (x) { u = x; fill(); }).catch(function (e) { alert(e.message); });
        });
        var remove = q('#user-remove');
        if (remove) remove.addEventListener('click', function () {
            var d = formDialog(t('Remove user'),
                '<p>' + esc(t('This deletes the account of {user} and ends their sessions. It cannot be undone.', { user: name })) + '</p>' +
                '<div class="form-group"><label for="fd-confirm-name">' + esc(t('Type {user} to confirm', { user: name })) + '</label>' +
                '<input type="text" id="fd-confirm-name" autocomplete="off"></div>',
                t('Remove'), function () {
                    return api('DELETE', '/api/users/' + enc).then(function () { location.href = '/users'; });
                });
            var ok = d.q('button[type=submit]');
            ok.className = 'btn btn-danger';
            ok.disabled = true;
            d.q('#fd-confirm-name').addEventListener('input', function (ev) { ok.disabled = ev.target.value !== name; });
        });

        var sessBox = q('#user-sessions');
        function sessions() {
            api('GET', '/api/users/' + enc + '/sessions').then(function (list) {
                q('#user-sessions-count').textContent = list.length ? '(' + list.length + ')' : '';
                sessBox.innerHTML = list.length ? '' : '<p class="set-hint">' + esc(t('No open sessions.')) + '</p>';
                list.forEach(function (s) {
                    s.current = false;
                    sessBox.appendChild(sessionRow(s, function (id) { return '/api/users/' + enc + '/sessions/' + encodeURIComponent(id); }, sessions));
                });
            });
        }
        sessions();

        var loaded = false;
        function activity() {
            if (loaded) return;
            loaded = true;
            api('GET', '/api/audit?user=' + enc + '&days=').then(function (r) {
                q('#user-activity').innerHTML = auditRows(r.events.slice(0, 20), false) ||
                    '<tr><td class="text-muted">' + esc(t('No activity yet.')) + '</td></tr>';
            });
        }
        if (self) q('#user-save-roles').title = t('You cannot remove your own administrator role if you are the last one.');
    }

    // --- Audit log ---
    function auditLog(root) {
        var page = 1, total = 0, size = 50;
        var fields = root.querySelectorAll('.toolbar [name]');
        var params = new URLSearchParams(location.search);
        fields.forEach(function (f) { if (params.has(f.name)) f.value = params.get(f.name); });

        function query() {
            var p = new URLSearchParams();
            fields.forEach(function (f) { if (f.value) p.set(f.name, f.value); });
            if (!root.querySelector('[name=days]').value) p.set('days', '');
            return p;
        }
        function load() {
            var p = query();
            root.querySelector('#audit-export').href = '/api/audit/export?' + p.toString();
            history.replaceState(null, '', '/audit?' + p.toString());
            p.set('page', page);
            api('GET', '/api/audit?' + p.toString()).then(function (r) {
                total = r.total; size = r.page_size;
                var chain = root.querySelector('#audit-chain');
                chain.className = 'audit-chain ' + (r.broken_at ? 'broken' : 'intact');
                chain.textContent = r.broken_at
                    ? t('The audit log was altered: the chain breaks at line {line}.', { line: r.broken_at })
                    : '✓ ' + t('Chain intact: no entry has been changed or removed.');
                root.querySelector('#audit-body').innerHTML = auditRows(r.events, true) ||
                    '<tr><td colspan="5" class="text-muted">' + esc(t('No events match the filters.')) + '</td></tr>';
                var from = total ? (page - 1) * size + 1 : 0;
                root.querySelector('#audit-count').textContent = t('{from}–{to} of {total}', { from: from, to: Math.min(page * size, total), total: total });
                root.querySelector('#audit-prev').disabled = page <= 1;
                root.querySelector('#audit-next').disabled = page * size >= total;
            }).catch(function (e) {
                root.querySelector('#audit-body').innerHTML = '<tr><td colspan="5" class="form-error">' + esc(e.message) + '</td></tr>';
            });
        }
        var timer;
        fields.forEach(function (f) {
            f.addEventListener('input', function () { clearTimeout(timer); timer = setTimeout(function () { page = 1; load(); }, f.type === 'search' ? 300 : 0); });
        });
        root.querySelector('#audit-prev').addEventListener('click', function () { page--; load(); });
        root.querySelector('#audit-next').addEventListener('click', function () { page++; load(); });
        load();
    }

    // --- The user menu in the header ---
    function userMenu() {
        var btn = document.getElementById('user-menu-btn');
        var pop = document.getElementById('user-menu-pop');
        if (!btn || !pop) return;
        function set(open) {
            pop.hidden = !open;
            btn.setAttribute('aria-expanded', open ? 'true' : 'false');
            btn.classList.toggle('open', open);
        }
        btn.addEventListener('click', function (ev) { ev.stopPropagation(); set(pop.hidden); });
        document.addEventListener('click', function (ev) { if (!pop.hidden && !pop.contains(ev.target)) set(false); });
        document.addEventListener('keydown', function (ev) { if (ev.key === 'Escape' && !pop.hidden) { set(false); btn.focus(); } });
    }

    document.addEventListener('DOMContentLoaded', function () {
        if (document.body.hasAttribute('data-signed-in')) sessionWatch();
        userMenu();
        var el;
        if ((el = document.getElementById('account-app'))) account(el);
        if ((el = document.getElementById('users-app'))) usersList(el);
        if ((el = document.getElementById('user-app'))) userDetail(el);
        if ((el = document.getElementById('audit-app'))) auditLog(el);
        var copy = document.getElementById('copy-codes');
        if (copy) copy.addEventListener('click', function () {
            var codes = Array.prototype.map.call(document.querySelectorAll('.recovery-codes code'), function (c) { return c.textContent; });
            navigator.clipboard.writeText(codes.join('\n'));
            copy.textContent = t('Copied');
        });
    });
})();
