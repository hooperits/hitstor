(function () {
    'use strict';

    function initSidebar() {
        var btn = document.getElementById('sidebar-toggle');
        var backdrop = document.getElementById('sidebar-backdrop');
        if (!btn || !backdrop) return;

        function close() {
            document.body.classList.remove('sidebar-open');
        }

        btn.addEventListener('click', function () {
            document.body.classList.toggle('sidebar-open');
        });
        backdrop.addEventListener('click', close);
        document.querySelectorAll('.sidebar .nav-item').forEach(function (el) {
            el.addEventListener('click', close);
        });
    }

    function initPasswordToggle() {
        var btn = document.getElementById('password-toggle');
        var input = document.getElementById('login-password');
        if (!btn || !input) return;
        btn.addEventListener('click', function () {
            var show = input.type === 'password';
            input.type = show ? 'text' : 'password';
            btn.textContent = show ? t('Hide') : t('Show');
        });
    }

    document.addEventListener('DOMContentLoaded', function () {
        initSidebar();
        initPasswordToggle();
    });
})();