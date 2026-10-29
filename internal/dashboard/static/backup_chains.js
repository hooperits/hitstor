// Backup chains of a workload job: for each database, each full backup
// with the differentials based on it and the log backups that follow, so
// it is plain which full a differential or log belongs to and how far a
// chain restores.
(function () {
    'use strict';

    function esc(s) {
        return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
    }

    // SHOWN is how many differentials and logs a chain lists before "more".
    var SHOWN = 8;

    function item(p, full) {
        var note = '';
        if (p.kind === 'differential') note = full ? t('based on the full of {when}', { when: full.when }) : t('based on a full HitStor does not hold');
        if (p.gap) note = t('gap: the log backups before it are missing');
        return '<li class="chain__item chain__item--' + esc(p.kind) + (p.gap ? ' chain__item--gap' : '') + '">' +
            HitStorPlan.badge(p.kind) +
            '<span class="chain__kind">' + esc(p.label) + '</span>' +
            '<span class="chain__when">' + esc(p.when) + '</span>' +
            (note ? '<span class="chain__note">' + esc(note) + '</span>' : '') +
            '<span class="chain__size">' + esc(p.size) + '</span>' +
            '<code class="chain__snap" title="' + esc(t('Snapshot')) + '">' + esc(p.snapshot) + '</code></li>';
    }

    function chain(c) {
        var rest = c.differentials.concat(c.logs).sort(function (a, b) { return new Date(b.at) - new Date(a.at); });
        var tags = '';
        if (c.current) tags += '<span class="chain__tag chain__tag--current">' + esc(t('current chain')) + '</span>';
        if (c.full.copy_only) tags += '<span class="chain__tag">' + esc(t('copy only')) + '</span>';
        if (c.broken) tags += '<span class="chain__tag chain__tag--broken">' + esc(t('log chain broken')) + '</span>';
        var counts = [];
        if (c.differentials.length) counts.push(t('{n} differential(s)', { n: c.differentials.length }));
        if (c.logs.length) counts.push(t('{n} log backup(s)', { n: c.logs.length }));
        return '<article class="chain' + (c.current ? ' chain--current' : '') + (c.broken ? ' chain--broken' : '') + '">' +
            '<header class="chain__full">' + HitStorPlan.badge('full') +
            '<div class="chain__full-main"><div><strong>' + esc(c.full.label) + '</strong> · ' + esc(c.full.when) + tags + '</div>' +
            '<div class="chain__full-meta">' + esc(c.full.size) + ' · <code>' + esc(c.full.snapshot) + '</code>' +
            (counts.length ? ' · ' + esc(counts.join(', ')) : '') + '</div></div>' +
            '<div class="chain__restorable"><span>' + esc(t('Restores up to')) + '</span><strong>' + esc(c.restorable_to) + '</strong></div>' +
            '</header>' +
            (rest.length ? '<ol class="chain__items">' + rest.map(function (p, i) {
                return (i === SHOWN ? '</ol><details class="chain__more"><summary>' + esc(t('{n} more', { n: rest.length - SHOWN })) + '</summary><ol class="chain__items">' : '') + item(p, c.full);
            }).join('') + '</ol>' + (rest.length > SHOWN ? '</details>' : '') : '') +
            '</article>';
    }

    function database(db) {
        var current = db.chains.filter(function (c) { return c.current; })[0];
        var head = '<div class="chain-db__head"><code class="path-chip">' + esc(db.object) + '</code>' +
            (current ? '<span class="chain-db__ok">' + esc(t('Restores up to {when}', { when: current.restorable_to })) + '</span>' : '') + '</div>';
        var html = head;
        var older = [];
        db.chains.forEach(function (c, i) {
            if (i === 0 || c.current) html += chain(c);
            else older.push(c);
        });
        if (older.length) {
            html += '<details class="chain-db__older"><summary>' + esc(t('Earlier chains ({n})', { n: older.length })) + '</summary>' +
                older.map(chain).join('') + '</details>';
        }
        if (db.orphans && db.orphans.length) {
            html += '<div class="chain-orphans"><strong>' + esc(t('Backups without a base in HitStor')) + '</strong>' +
                '<p>' + esc(t('These differential or log backups build on a full backup HitStor does not hold, so they cannot be restored from HitStor.')) + '</p>' +
                '<ol class="chain__items">' + db.orphans.map(function (p) { return item(p, null); }).join('') + '</ol></div>';
        }
        return '<section class="chain-db">' + html + '</section>';
    }

    function render(el, jobID) {
        fetch('/api/backups/' + encodeURIComponent(jobID) + '/chains').then(function (r) {
            if (!r.ok) return r.text().then(function (m) { throw new Error(m); });
            return r.json();
        }).then(function (dbs) {
            if (!dbs.length) {
                el.innerHTML = '<p class="chains__empty">' + esc(t('No backups yet. The first full backup starts the chain.')) + '</p>';
                return;
            }
            el.innerHTML = dbs.map(database).join('');
        }).catch(function (err) {
            el.innerHTML = '<p class="plugin-error">' + esc(err.message) + '</p>';
        });
    }

    window.HitStorChains = { render: render };
})();
