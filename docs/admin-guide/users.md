# Users and Access

The dashboard uses personal accounts. Each person signs in with their own user name, password and a code from an authenticator app, and only sees and changes what their role allows.

## Roles { #roles }

| Role | Can do |
|------|--------|
| **Administrator** | Everything, on every agent: users, storage, settings, deleting snapshots, disaster recovery. |
| **Backup operator** | Create, edit, schedule, run and cancel backup jobs; suspend and resume agents. Cannot restore. |
| **Restore operator** | Browse snapshots and restore them. Cannot change jobs. |
| **Viewer** | Read jobs, results, logs and reports. Changes nothing. |

Backup and restore are separate roles on purpose: a person who restores can read any data that was backed up, so give that role only to those who need it.

Every role except Administrator is given either on **all agents** or on a list of agents. A backup operator for `web01` sees only `web01`, its jobs and their results; pages that span every agent (Overview, Storage, Disaster recovery, Logs) need the role on all agents. A user can hold several roles, for example backup operator on two agents and viewer on all.

## Two-step verification { #two-step-verification }

Two-step verification is mandatory for every account. At the first sign-in each user:

1. Installs an authenticator app (Microsoft Authenticator, Google Authenticator, 1Password, Bitwarden...).
2. Scans the QR code the dashboard shows, or types the key under it.
3. Enters the 6-digit code to confirm, and saves the 10 **recovery codes** shown next. Each recovery code signs in once when the phone is not at hand.

From then on, signing in asks for the password and then a code. A code can be used only once. Under **My account** a user can generate new recovery codes (the old ones stop working) or move the authenticator to a new phone.

If a user loses both the phone and the recovery codes, an administrator resets their two-step verification (**Users → Manage → Reset two-step verification**) and the user sets it up again at the next sign-in.

## Passwords { #passwords }

- At least 12 characters and at most 128. A phrase of several words is easy to remember and hard to guess.
- Commonly used passwords and passwords containing the user name are refused.
- There are no composition rules and no periodic expiry, as NIST SP 800-63B recommends; change a password when there is a reason to.
- Passwords are stored as argon2id hashes.

Administrators never choose a user's password. Creating a user or resetting a password gives a **temporary password**: hand it over in person or by another channel. It works once: at the next sign-in the user chooses their own password and, for a new user, sets up the authenticator.

## Sessions and lockout { #sessions }

- A session ends after 30 minutes without activity (15 for administrators) and after 12 hours in any case.
- Deleting users, changing roles, resetting passwords, deleting snapshots, pruning, recovering a server and similar changes ask for the password and a code again if the last sign-in was more than 15 minutes ago.
- After 10 wrong passwords or codes an account is locked for 15 minutes. Repeated failures from one address are also slowed down.
- **My account** lists the user's open sessions and can end any of them. Administrators can end anybody's sessions under **Users**.

## The first administrator { #first-administrator }

When a server is upgraded from a version with a single dashboard password, the dashboard opens at **Create the first administrator**. Enter the current dashboard password (from `serve_dashboard_password`, or the one printed in the server log) and choose the administrator's user name and password. Then set up the authenticator. From that moment the shared password no longer signs in.

Accounts are stored next to the agent registry in the first repository, in `users.json`, encrypted with the repository key. Until the server has a repository, the dashboard still signs in with the shared password.

## Managing users { #managing-users }

Administrators manage accounts under **Users**: create users, change their roles and agents, disable or remove them, reset passwords or two-step verification, and end sessions. The last active administrator cannot be removed, disabled or lose the role.

### From the server console { #cli }

`hitstor serve user` manages accounts directly on the server, for example when no administrator can sign in. Run it with the same configuration and repository password as the service; changes apply to a running server immediately.

```bash
# Create an administrator; prints a temporary password
hitstor serve user add maria --grant admin --display-name "Maria Perez"

# A backup operator for two agents who can also read everything
hitstor serve user add ops1 --grant backup_operator=web01,db01 --grant viewer

hitstor serve user list
hitstor serve user grant ops1 --grant restore_operator=db01   # replaces the roles
hitstor serve user passwd maria        # new temporary password, ends sessions
hitstor serve user reset-2fa maria     # set up the authenticator again
hitstor serve user disable ops1
hitstor serve user enable ops1         # also lifts a lockout
hitstor serve user remove ops1
```

## Audit log { #audit-log }

**Audit** (administrators) lists sign-ins, failed attempts, changes to users and denied requests, with user, address and time. Each entry carries the hash of the previous one, so editing or deleting an entry breaks the chain, and the page says where. The log is `audit.log`, next to `users.json`; changes made with `hitstor serve user` appear as user `console`.
