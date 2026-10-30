// User accounts in the dashboard: re-authentication before sensitive
// actions (any page), the user's own account, and user administration.
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

    function when(iso) {
        return iso ? new Date(iso).toLocaleString(document.documentElement.lang || undefined) : '—';
    }

    var ROLE_LABEL = function (role) {
        return {
            admin: t('Administrator'), backup_operator: t('Backup operator'),
            restore_operator: t('Restore operator'), viewer: t('Viewer')
        }[role] || role;
    };

    function grantsText(grants) {
        return (grants || []).map(function (g) {
            var scope = g.role === 'admin' || !(g.agents && g.agents.length) ? t('all agents') : g.agents.join(', ');
            return '<span class="role-chip role-chip--' + esc(g.role) + '">' + esc(ROLE_LABEL(g.role)) + '</span> <span class="role-scope">' + esc(scope) + '</span>';
        }).join('<br>');
    }

    function showCodes(codes) {
        var d = dialog(t('Your new recovery codes'),
            '<p class="form-hint">' + esc(t('Each code signs you in once if you lose your phone. Your previous codes no longer work.')) + '</p>' +
            '<ol class="recovery-codes">' + codes.map(function (c) { return '<li><code>' + esc(c) + '</code></li>'; }).join('') + '</ol>' +
            '<div class="dialog-actions"><button type="button" class="btn">' + esc(t('Done')) + '</button></div>');
        d.q('.dialog-actions .btn').addEventListener('click', d.close);
    }

    // --- My account ---
    function account(root) {
        var form = root.querySelector('#account-password');
        form.addEventListener('submit', function (ev) {
            ev.preventDefault();
            var status = root.querySelector('#account-password-status');
            api('POST', '/api/account/password', {
                current: form.current.value, password: form.password.value, confirm: form.confirm.value
            }).then(function () {
                form.reset();
                status.textContent = t('Password changed. Your other sessions were closed.');
                status.className = 'form-status text-ok';
            }).catch(function (err) {
                status.textContent = err.message;
                status.className = 'form-status text-critical';
            });
        });
        root.querySelector('#account-codes').addEventListener('click', function () {
            api('POST', '/api/account/recovery-codes').then(function (r) { showCodes(r.codes); })
                .catch(function (err) { alert(err.message); });
        });
        function sessions() {
            api('GET', '/api/account/sessions').then(function (list) {
                root.querySelector('#account-sessions').innerHTML = list.map(function (s) {
                    return '<tr><td>' + esc(when(s.created)) + '</td><td>' + esc(when(s.last_seen)) + '</td><td>' + esc(s.ip) +
                        '</td><td class="ua">' + esc(s.user_agent) + '</td><td>' +
                        (s.current ? '<span class="badge badge-ok">' + esc(t('this browser')) + '</span>'
                            : '<button type="button" class="btn btn-secondary btn-sm" data-end="' + esc(s.id) + '">' + esc(t('End')) + '</button>') +
                        '</td></tr>';
                }).join('');
                root.querySelectorAll('[data-end]').forEach(function (b) {
                    b.addEventListener('click', function () {
                        api('DELETE', '/api/account/sessions/' + encodeURIComponent(b.getAttribute('data-end'))).then(sessions);
                    });
                });
            });
        }
        sessions();
    }

    // --- Users (administrators) ---
    function usersAdmin(root) {
        var agents = JSON.parse(root.getAttribute('data-agents') || 'null') || [];
        var roles = JSON.parse(root.getAttribute('data-roles') || 'null') || [];
        var body = root.querySelector('#users-body');

        function load() {
            api('GET', '/api/users').then(function (list) {
                body.innerHTML = list.map(function (u) {
                    var status = u.disabled ? '<span class="badge badge-disabled">' + esc(t('disabled')) + '</span>'
                        : u.locked ? '<span class="badge badge-stale">' + esc(t('locked')) + '</span>'
                            : '<span class="badge badge-ok">' + esc(t('active')) + '</span>';
                    var mfa = u.totp_enabled ? '<span class="badge badge-ok">' + esc(t('on')) + '</span> <span class="role-scope">' +
                        esc(t('{n} recovery codes', { n: u.recovery_codes_left })) + '</span>'
                        : '<span class="badge badge-stale">' + esc(t('pending')) + '</span>';
                    return '<tr><td><strong>' + esc(u.username) + '</strong>' + (u.display_name ? '<br><span class="role-scope">' + esc(u.display_name) + '</span>' : '') +
                        (u.must_change_password ? '<br><span class="role-scope">' + esc(t('must change password')) + '</span>' : '') + '</td>' +
                        '<td>' + grantsText(u.grants) + '</td><td>' + mfa + '</td><td>' + esc(when(u.last_login_at)) + '</td><td>' + status + '</td>' +
                        '<td class="col-actions"><button type="button" class="btn btn-secondary btn-sm" data-user="' + esc(u.username) + '">' + esc(t('Manage')) + '</button></td></tr>';
                }).join('') || '<tr><td colspan="6" class="text-muted">' + esc(t('No users yet.')) + '</td></tr>';
                body.querySelectorAll('[data-user]').forEach(function (b) {
                    var u = list.filter(function (x) { return x.username === b.getAttribute('data-user'); })[0];
                    b.addEventListener('click', function () { manage(u); });
                });
            }).catch(function (err) { body.innerHTML = '<tr><td colspan="6" class="form-error">' + esc(err.message) + '</td></tr>'; });
        }

        // grantsEditor renders rows of (role, agents) into el.
        function grantsEditor(el, grants) {
            function row(g) {
                var div = document.createElement('div');
                div.className = 'grant-row';
                div.innerHTML = '<select class="grant-role">' + roles.map(function (r) {
                    return '<option value="' + esc(r) + '"' + (r === g.role ? ' selected' : '') + '>' + esc(ROLE_LABEL(r)) + '</option>';
                }).join('') + '</select>' +
                    '<div class="grant-scope"><label class="grant-all"><input type="checkbox" class="grant-all-box"' + (!(g.agents && g.agents.length) ? ' checked' : '') + '> ' + esc(t('All agents')) + '</label>' +
                    '<div class="grant-agents">' + agents.map(function (a) {
                        return '<label><input type="checkbox" value="' + esc(a) + '"' + ((g.agents || []).indexOf(a) >= 0 ? ' checked' : '') + '> ' + esc(a) + '</label>';
                    }).join('') + '</div></div>' +
                    '<button type="button" class="grant-remove" aria-label="' + esc(t('Remove role')) + '">&times;</button>';
                function sync() {
                    var isAdmin = div.querySelector('.grant-role').value === 'admin';
                    var all = div.querySelector('.grant-all-box');
                    if (isAdmin) all.checked = true;
                    all.disabled = isAdmin;
                    div.querySelector('.grant-agents').hidden = all.checked;
                }
                div.querySelector('.grant-role').addEventListener('change', sync);
                div.querySelector('.grant-all-box').addEventListener('change', sync);
                div.querySelector('.grant-remove').addEventListener('click', function () { div.remove(); });
                sync();
                el.appendChild(div);
            }
            el.innerHTML = '';
            (grants && grants.length ? grants : [{ role: 'viewer' }]).forEach(row);
            return {
                add: function () { row({ role: 'backup_operator', agents: [] }); },
                value: function () {
                    var out = [];
                    el.querySelectorAll('.grant-row').forEach(function (div) {
                        var role = div.querySelector('.grant-role').value;
                        var g = { role: role };
                        if (role !== 'admin' && !div.querySelector('.grant-all-box').checked) {
                            g.agents = [];
                            div.querySelectorAll('.grant-agents input:checked').forEach(function (i) { g.agents.push(i.value); });
                            if (!g.agents.length) throw new Error(t('Choose at least one agent, or All agents.'));
                        }
                        out.push(g);
                    });
                    if (!out.length) throw new Error(t('Give the user at least one role.'));
                    return out;
                }
            };
        }

        function showPassword(d, user, pw) {
            d.q('.dialog-body').innerHTML = '<p>' + esc(t('Give this temporary password to {user}. It works once: they choose their own and set up two-step verification at first sign-in.', { user: user })) + '</p>' +
                '<p class="temp-password"><code>' + esc(pw) + '</code></p>' +
                '<div class="dialog-actions"><button type="button" class="btn">' + esc(t('Done')) + '</button></div>';
            d.q('.dialog-actions .btn').addEventListener('click', function () { d.close(); load(); });
        }

        root.querySelector('#user-add').addEventListener('click', function () {
            var d = dialog(t('New user'),
                '<form class="user-form">' +
                '<div class="form-group"><label>' + esc(t('User name')) + '</label><input type="text" class="u-name" required autocomplete="off" pattern="[A-Za-z0-9][A-Za-z0-9._\\-]{1,63}"></div>' +
                '<div class="form-group"><label>' + esc(t('Full name (optional)')) + '</label><input type="text" class="u-display" autocomplete="off"></div>' +
                '<div class="form-group"><label>' + esc(t('Roles')) + '</label><div class="grants"></div>' +
                '<button type="button" class="btn btn-secondary btn-sm u-add-grant">' + esc(t('Add another role')) + '</button></div>' +
                '<div class="form-error" style="display:none"></div>' +
                '<div class="dialog-actions"><button type="button" class="btn btn-secondary u-cancel">' + esc(t('Cancel')) + '</button>' +
                '<button type="submit" class="btn">' + esc(t('Create user')) + '</button></div></form>');
            var ed = grantsEditor(d.q('.grants'), [{ role: 'backup_operator', agents: [] }]);
            d.q('.u-add-grant').addEventListener('click', ed.add);
            d.q('.u-cancel').addEventListener('click', d.close);
            d.q('form').addEventListener('submit', function (ev) {
                ev.preventDefault();
                var errEl = d.q('.form-error');
                var grants;
                try { grants = ed.value(); } catch (err) { errEl.textContent = err.message; errEl.style.display = 'block'; return; }
                api('POST', '/api/users', { username: d.q('.u-name').value, display_name: d.q('.u-display').value, grants: grants })
                    .then(function (r) { showPassword(d, r.user.username, r.temporary_password); })
                    .catch(function (err) { errEl.textContent = err.message; errEl.style.display = 'block'; });
            });
        });

        function manage(u) {
            var enc = encodeURIComponent(u.username);
            var d = dialog(t('User {user}', { user: u.username }),
                '<div class="form-group"><label>' + esc(t('Roles')) + '</label><div class="grants"></div>' +
                '<button type="button" class="btn btn-secondary btn-sm u-add-grant">' + esc(t('Add another role')) + '</button> ' +
                '<button type="button" class="btn btn-sm u-save">' + esc(t('Save roles')) + '</button></div>' +
                '<div class="user-actions">' +
                '<button type="button" class="btn btn-secondary u-reset-pw">' + esc(t('Reset password')) + '</button>' +
                '<button type="button" class="btn btn-secondary u-reset-mfa">' + esc(t('Reset two-step verification')) + '</button>' +
                '<button type="button" class="btn btn-secondary u-toggle">' + esc(u.disabled ? t('Enable') : t('Disable')) + '</button>' +
                '<button type="button" class="btn btn-danger u-remove">' + esc(t('Remove')) + '</button></div>' +
                '<h3 class="user-sessions-title">' + esc(t('Signed-in sessions')) + '</h3><div class="u-sessions text-muted">' + esc(t('Loading...')) + '</div>' +
                '<div class="form-error" style="display:none"></div>');
            var errEl = d.q('.form-error');
            var fail = function (err) { errEl.textContent = err.message; errEl.style.display = 'block'; };
            var ed = grantsEditor(d.q('.grants'), u.grants);
            d.q('.u-add-grant').addEventListener('click', ed.add);
            d.q('.u-save').addEventListener('click', function () {
                var grants;
                try { grants = ed.value(); } catch (err) { return fail(err); }
                api('PUT', '/api/users/' + enc, { grants: grants }).then(function () { d.close(); load(); }).catch(fail);
            });
            d.q('.u-reset-pw').addEventListener('click', function () {
                if (!confirm(t('Reset the password of {user}? Their sessions end.', { user: u.username }))) return;
                api('POST', '/api/users/' + enc + '/password').then(function (r) { showPassword(d, u.username, r.temporary_password); }).catch(fail);
            });
            d.q('.u-reset-mfa').addEventListener('click', function () {
                if (!confirm(t('Reset two-step verification of {user}? They set it up again at next sign-in.', { user: u.username }))) return;
                api('DELETE', '/api/users/' + enc + '/second-factor').then(function () { d.close(); load(); }).catch(fail);
            });
            d.q('.u-toggle').addEventListener('click', function () {
                api('PUT', '/api/users/' + enc, { disabled: !u.disabled }).then(function () { d.close(); load(); }).catch(fail);
            });
            d.q('.u-remove').addEventListener('click', function () {
                if (!confirm(t('Remove the user {user}? This cannot be undone.', { user: u.username }))) return;
                api('DELETE', '/api/users/' + enc).then(function () { d.close(); load(); }).catch(fail);
            });
            api('GET', '/api/users/' + enc + '/sessions').then(function (list) {
                var box = d.q('.u-sessions');
                if (!list.length) { box.textContent = t('No open sessions.'); return; }
                box.innerHTML = '<ul class="user-sessions">' + list.map(function (s) {
                    return '<li>' + esc(when(s.last_seen)) + ' · ' + esc(s.ip) + ' <button type="button" class="btn btn-secondary btn-sm" data-end="' + esc(s.id) + '">' + esc(t('End')) + '</button></li>';
                }).join('') + '</ul>';
                box.querySelectorAll('[data-end]').forEach(function (b) {
                    b.addEventListener('click', function () {
                        api('DELETE', '/api/users/' + enc + '/sessions/' + encodeURIComponent(b.getAttribute('data-end'))).then(function () { b.parentNode.remove(); }).catch(fail);
                    });
                });
            }).catch(fail);
        }

        load();
    }

    document.addEventListener('DOMContentLoaded', function () {
        if (document.body.hasAttribute('data-signed-in')) sessionWatch();
        var a = document.getElementById('account-app');
        if (a) account(a);
        var u = document.getElementById('users-app');
        if (u) usersAdmin(u);
        var copy = document.getElementById('copy-codes');
        if (copy) copy.addEventListener('click', function () {
            var codes = Array.prototype.map.call(document.querySelectorAll('.recovery-codes code'), function (c) { return c.textContent; });
            navigator.clipboard.writeText(codes.join('\n'));
            copy.textContent = t('Copied');
        });
    });
})();
