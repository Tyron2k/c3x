# CI / forge integration overview

Every supported forge follows the same pattern: c3x runs after
`terraform plan`, computes the cost, and posts a marker-tagged PR
comment. Marker tagging means re-runs **edit the same comment in
place**, never stacking duplicates.

| Forge | Subcommand | Auto-detect env vars |
|---|---|---|
| GitHub | `c3x comment github` | `GITHUB_REPOSITORY`, `GITHUB_REF`, `GITHUB_TOKEN` |
| GitLab | `c3x comment gitlab` | `CI_PROJECT_ID`, `CI_MERGE_REQUEST_IID`, `CI_API_V4_URL`, `GITLAB_TOKEN` / `CI_JOB_TOKEN` |
| Bitbucket Cloud | `c3x comment bitbucket` | `BITBUCKET_REPO_FULL_NAME`, `BITBUCKET_PR_ID`, `BITBUCKET_USERNAME`, `BITBUCKET_APP_PASSWORD` |
| Azure DevOps | `c3x comment azuredevops` | `SYSTEM_TEAMFOUNDATIONCOLLECTIONURI`, `SYSTEM_TEAMPROJECT`, `BUILD_REPOSITORY_NAME`, `SYSTEM_PULLREQUEST_PULLREQUESTID`, `SYSTEM_ACCESSTOKEN` |
| Atlantis | (post-plan custom step) | Pipes through whichever forge token Atlantis itself uses |

## Marker / coexistence

The comment marker is `<!-- c3x-comment:v1 -->`, scoped to c3x so it
updates its own comment in place and does not collide with other
bots' posts.

## CI gate patterns

### Absolute budget

Fails the workflow when the project total exceeds a threshold:

```bash
c3x estimate --path . --budget 1000
```

### Per-PR delta

Fails when a PR adds more than a threshold above its baseline.
Requires a baseline file that you persist between PRs (artifact
store, S3, ...):

```bash
c3x diff --baseline base.json --path . --budget-delta 50
```

### Policy gate (Rego)

For nuanced rules ("no GPU instances on weekends", "EBS gp3 only
for prod") use the policy engine:

```bash
c3x policy eval --policy ./policies --path .
```

See [`/guide/policy`](/guide/policy) and the sample policies under
`examples/policies/` in the repo.
