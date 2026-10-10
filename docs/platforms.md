# Git platforms

A site lives in a git repository, and cl-cms works with any repository git
can reach: a hosted platform, your own server, or a folder on disk. Where it
knows the platform, it also speaks its language: webhooks so a push rebuilds
at once, and commit statuses so a pull request shows its preview.

| Platform | Clone | Push webhooks | Preview link on PRs | CI template |
|---|---|---|---|---|
| GitHub, GitHub Enterprise | yes | `X-Hub-Signature-256` | commit status | `cl-cms ci github` |
| GitLab (gitlab.com or self-hosted) | yes | `X-Gitlab-Token` | commit status | `cl-cms ci gitlab` |
| Gitea | yes | `X-Gitea-Signature` | commit status | `cl-cms ci gitea` |
| Forgejo, Codeberg | yes | `X-Forgejo-Signature` | commit status | `cl-cms ci forgejo` |
| Gogs | yes | `X-Gogs-Signature` | (Gogs has no statuses) | (no CI) |
| Bitbucket Cloud | yes | `X-Hub-Signature` | build status | `cl-cms ci bitbucket` |
| Woodpecker CI (with any of the above) | | | | `cl-cms ci woodpecker` |
| Plain git: your own server, SourceHut, Azure DevOps, anything | yes | `X-CLCMS-Token` from a `post-receive` hook | | `cl-cms ci git` |

The platform is worked out from the repository URL for github.com,
gitlab.com, gitea.com, codeberg.org and bitbucket.org. For a self-hosted
server, say which it is with `-platform gitea` (or `gitlab`, `github`,
`forgejo`, `gogs`).

## Secrets

All from the environment, never from flags, so they don't land in shell
history or the process list:

| Variable | For |
|---|---|
| `CLCMS_GIT_TOKEN` | Cloning private repositories over HTTPS. SSH URLs use your keys and agent instead. |
| `CLCMS_GIT_USER` | The user name to go with the token. Defaults per platform: `x-access-token` (GitHub), `oauth2` (GitLab), `x-token-auth` (Bitbucket); set it to your account name on Gitea, Forgejo and Gogs. |
| `CLCMS_HOOK_SECRET` | The secret the platform signs webhooks with. Without it, the webhook endpoint is off. |
| `CLCMS_FORGE_TOKEN` | Posting commit statuses. Give it only that permission (GitHub: "Commit statuses: write"; GitLab: `api` on a project token; Gitea/Forgejo: repository write). |

## Webhooks

Point the platform's push webhook at `https://<your preview domain>/_cl/hook`
(for `cl-cms preview`) or at the address given to `cl-cms pull -hook`, with
the same secret as `CLCMS_HOOK_SECRET`, content type JSON.

A delivery that isn't signed with that secret is refused. One that is signed
but isn't a branch push (a ping, a tag, an issue) is acknowledged and ignored.
Previews and pull agents keep their scheduled check too, so a missed webhook
only delays a rebuild, it never loses one.

## Plain git

Without a platform, a `post-receive` hook in the bare repository does the
webhook's job:

    cl-cms ci git > site.git/hooks/post-receive && chmod +x site.git/hooks/post-receive

It reads its token from a header file, so the token never appears on a
command line, and it never blocks or fails a push: if cl-cms can't be reached,
the next scheduled check catches up.

## CI

`cl-cms ci <platform>` prints the pipeline file, and `-write` puts it in the
repository. Every template checks the site on each pull request, and on the
publishing branch runs `cl-cms publish -all` with the targets in
`publish.yaml`. Actions are pinned to commits. For an rsync target the pipeline
loads a deploy key and checks the server against a known_hosts entry kept as a
secret; no template turns host key checking off.
