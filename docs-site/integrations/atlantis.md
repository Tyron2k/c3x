# Running c3x in Atlantis

Atlantis runs `terraform plan` against PRs and posts the result as
a PR comment. With a small `atlantis.yaml` change, c3x runs after
`plan` to compute the monthly cost delta and post it alongside.

## How it works

Atlantis exposes a "custom workflow" hook that runs arbitrary
commands between or after the built-in `init`, `plan`, `apply`
steps. We wire c3x as a post-plan step:

1. Atlantis runs `terraform plan -out=plan.tfplan`
2. The c3x step runs `terraform show -json plan.tfplan > plan.json`
3. c3x reads `plan.json` (the parser handles plan JSON natively)
4. c3x posts a PR comment via `c3x comment <forge>` — Atlantis
   already has GH/GitLab/Bitbucket creds in the runner env, so the
   c3x comment poster picks them up automatically.

The comment carries c3x's marker (`<!-- c3x-comment:v1 -->`), which
is distinct from Atlantis's own marker — both comments live side by
side on the PR without conflicting.

## Configuration

Drop this into your repo's `atlantis.yaml`:

```yaml
version: 3
projects:
  - name: infra
    dir: .
    workflow: with-c3x

workflows:
  with-c3x:
    plan:
      steps:
        - init
        - plan
        - run: terraform show -json $PLANFILE > $PLANFILE.json
        - run: c3x estimate --path $PLANFILE.json --save-baseline /tmp/c3x-current.json
        - run: |
            if [ -n "$BASE_REPO_NAME" ]; then
              # Atlantis sets BASE_REPO_OWNER / BASE_REPO_NAME / PULL_NUM
              c3x comment github \
                --owner "$BASE_REPO_OWNER" \
                --repo "$BASE_REPO_NAME" \
                --pr "$PULL_NUM" \
                --path $PLANFILE.json \
                --token "$ATLANTIS_GH_TOKEN"
            fi
```

If you're on GitLab or Bitbucket, swap `c3x comment github` for the
matching subcommand and the env var Atlantis exposes (e.g.
`ATLANTIS_GITLAB_TOKEN` for GitLab).

## Running with `--budget` as a merge gate

Atlantis honours a non-zero exit from any custom step. To block
merges when a PR adds >$X/month, change the c3x step:

```yaml
        - run: c3x estimate --path $PLANFILE.json --budget 1000
```

c3x exits 1 when the project total exceeds the budget; Atlantis
surfaces the failure in the PR comment, and the merge stays
blocked until the budget passes.

For per-PR delta gating (rather than absolute budget), pair with
a baseline file saved earlier:

```yaml
        - run: |
            if [ -f /var/c3x-baseline/$BASE_REPO_NAME.json ]; then
              c3x diff \
                --baseline /var/c3x-baseline/$BASE_REPO_NAME.json \
                --path $PLANFILE.json \
                --budget-delta 50    # fail when PR adds >$50/mo
            fi
```

## Pre-built image

We ship a thin Atlantis-compatible image that has `c3x` + `terraform`
+ the standard Atlantis shell tools on its path:

```bash
docker pull ghcr.io/c3xdev/c3x-atlantis:latest
```

Reference from your Atlantis deployment:

```yaml
# docker-compose.yml
services:
  atlantis:
    image: ghcr.io/c3xdev/c3x-atlantis:latest
    environment:
      ATLANTIS_GH_USER: ${ATLANTIS_GH_USER}
      ATLANTIS_GH_TOKEN: ${ATLANTIS_GH_TOKEN}
      ...
```

(The image isn't published yet — pending the release pipeline in
ROADMAP item G. Build locally from `examples/atlantis/Dockerfile`
in the meantime.)

## Troubleshooting

- **c3x comment fails with "PR number is required"** — Atlantis's
  custom-step env passes `PULL_NUM`. Make sure your shell command
  references it (`--pr "$PULL_NUM"`).
- **No comment appears** — c3x silently no-ops if the token has no
  write scope. Atlantis tokens typically need `repo` (GitHub) or
  `api` (GitLab) scope to post comments.
- **Estimate shows $0 for everything** — the plan JSON may have
  empty `after` blocks for unsupported actions. Pass `--show-skipped`
  to surface the affected resources.
