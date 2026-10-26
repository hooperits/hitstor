#!/bin/sh
# grant-hitstor-journal-access.sh — allow the hitstor service user to read journald.
# Used by server packages/installers so View Logs can run journalctl without warnings.

grant_hitstor_journal_access() {
	if ! getent passwd hitstor >/dev/null 2>&1; then
		return 0
	fi
	if ! getent group systemd-journal >/dev/null 2>&1; then
		return 0
	fi
	if id -nG hitstor 2>/dev/null | tr ' ' '\n' | grep -qx systemd-journal; then
		return 0
	fi
	usermod -aG systemd-journal hitstor 2>/dev/null || true
}

grant_hitstor_journal_access