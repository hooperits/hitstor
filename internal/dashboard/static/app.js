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
            btn.textContent = show ? 'Hide' : 'Show';
        });
    }

    window.HitStorWizard = {
        current: 1,
        max: 3,
        _bound: false,

        syncJobTypeFromUI: function () {
            var typeEl = document.getElementById('job-type');
            if (!typeEl) return '';
            var type = typeEl.value;
            if (!type) {
                var selected = document.querySelector('.job-type-card.selected');
                if (selected) {
                    type = selected.getAttribute('data-type') || '';
                    if (type) typeEl.value = type;
                }
            }
            return type;
        },

        init: function () {
            this.current = 1;
            var selected = document.querySelector('.job-type-card.selected');
            var typeEl = document.getElementById('job-type');
            if (selected && typeEl) {
                typeEl.value = selected.getAttribute('data-type') || typeEl.value;
            }
            if (typeof toggleJobTypeFields === 'function') toggleJobTypeFields();
            this.render();
            if (!this._bound) {
                document.querySelectorAll('.job-type-card').forEach(function (card) {
                    card.addEventListener('click', function () {
                        var type = card.getAttribute('data-type');
                        if (typeof selectJobType === 'function') {
                            selectJobType(type);
                        } else {
                            document.getElementById('job-type').value = type;
                            if (typeof toggleJobTypeFields === 'function') toggleJobTypeFields();
                            document.querySelectorAll('.job-type-card').forEach(function (c) {
                                c.classList.toggle('selected', c === card);
                            });
                        }
                    });
                });
                var nextBtn = document.getElementById('wizard-next');
                var backBtn = document.getElementById('wizard-back');
                if (nextBtn) nextBtn.addEventListener('click', this.onNext.bind(this));
                if (backBtn) backBtn.addEventListener('click', this.onBack.bind(this));
                this._bound = true;
            }
        },

        render: function () {
            document.querySelectorAll('.form-step').forEach(function (step) {
                step.classList.toggle('active', parseInt(step.getAttribute('data-step'), 10) === this.current);
            }, this);
            document.querySelectorAll('.form-stepper-step').forEach(function (el) {
                var n = parseInt(el.getAttribute('data-step'), 10);
                el.classList.toggle('active', n === this.current);
                el.classList.toggle('done', n < this.current);
            }, this);
            var back = document.getElementById('wizard-back');
            var next = document.getElementById('wizard-next');
            var submit = document.getElementById('new-job-btn');
            if (back) back.style.display = this.current > 1 ? '' : 'none';
            if (next) next.style.display = this.current < this.max ? '' : 'none';
            if (submit) submit.style.display = this.current === this.max ? '' : 'none';
        },

        onNext: function () {
            var type = this.syncJobTypeFromUI();
            if (this.current === 1) {
                if (!type) {
                    alert('Select a backup job type.');
                    return;
                }
                if (typeof toggleJobTypeFields === 'function') toggleJobTypeFields();
            }
            if (this.current === 2) {
                var name = document.getElementById('job-name').value.trim();
                if (!name) {
                    alert('Enter a job name.');
                    return;
                }
                if (type === 'files' || type === 'database') {
                    var agent = document.getElementById('job-agent').value;
                    if (!agent) {
                        alert('Select an agent for this backup job.');
                        return;
                    }
                }
            }
            if (this.current < this.max) {
                this.current += 1;
                this.render();
                if (this.current === 2 && typeof toggleJobTypeFields === 'function') {
                    toggleJobTypeFields();
                }
            }
        },

        onBack: function () {
            if (this.current > 1) {
                this.current -= 1;
                this.render();
            }
        },

        reset: function () {
            this.current = 1;
            if (typeof selectJobType === 'function') {
                selectJobType('files');
            } else {
                document.querySelectorAll('.job-type-card').forEach(function (c) {
                    c.classList.remove('selected');
                });
                var filesCard = document.querySelector('.job-type-card[data-type="files"]');
                if (filesCard) filesCard.classList.add('selected');
                var typeEl = document.getElementById('job-type');
                if (typeEl) typeEl.value = 'files';
            }
            if (typeof toggleJobTypeFields === 'function') toggleJobTypeFields();
            this.render();
        }
    };

    document.addEventListener('DOMContentLoaded', function () {
        initSidebar();
        initPasswordToggle();
    });
})();