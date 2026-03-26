# env-kvstore

This GitHub Action provides a secure mechanism for retrieving secrets and variables from AWS Secrets Manager for use in GHA workflows.

## Usage

Each GitHub workflow that needs access to secrets and variables must be granted permission to retrieve the identity token from the token service.

```yaml
jobs:
  example-job:
    permissions:
      id-token: write
```

TODO: add an example step using the action

## Storage and Retrieval of Secrets and Variables

Secrets and variables are stored in AWS Secrets Manager. In order for Secrets Manager secrets to be accessible by this action, a specific naming convention must be followed and a role must be configured with appropriate attribute-based permissions. Mechanism for managing and storing values is outside the scope of this action.

### Secret and Variable Naming Convention

Secrets and variables should be named as follows:
1. Repo scoped secrets/variables: `${enterprise}/repo/${repository}/repo/secrets` and `${enterprise}/repo/${repository}/repo/variables`
2. Environment scoped secrets/variables: `${enterprise}/repo/${repository}/env/${environment}/secrets` and `${enterprise}/repo/${repository}/env/${environment}/variables`

Each Secret Manager secret contains a JSON object with key-value pairs for each individual secret and variable.

### IAM

#### OIDC Provider

An OIDC provider is required to allow GitHub workflows to authenticate with AWS. Best practice is to include the github organization name in the URL of the OIDC provider to constrain trust to workflows within that organization. For example, `https://token.actions.githubusercontent.com/{org-name}`. The OIDC provider needs to include the audience `cognito-identity.amazonaws.com` so the Identity Pool can use tokens from GitHub.

#### Cognito Identity Pool

A Cognito Identity Pool is used to exchange GitHub OIDC tokens for a Cognito OIDC token. The Identity Pool should be configured to trust the OIDC provider for the GitHub organization. A principal mapping is needed to map claims from the GitHub token to claims in the Cognito token.

<details>
<summary>Example Cognito Identity Pool Principal Mapping</summary>

```hcl
resource "aws_cognito_identity_pool_provider_principal_tag" "gha" {
  identity_provider_name = "arn:aws:iam::${AWS_ACCOUNT_ID}$:oidc-provider/token.actions.githubusercontent.com/${GITHUB_ORG}"
  identity_pool_id       = aws_cognito_identity_pool.gha.id
  use_defaults           = false

  principal_tags = {
    "repository"  = "repository"
    "enterprise"  = "enterprise"
    "environment" = "environment"
    "event_name"  = "event_name"
    "run_id"      = "run_id"
    "actor"       = "actor"
    "sha"         = "sha"
    "workflow"    = "workflow"
  }
}
```
</details>

#### IAM Role

An IAM role that can be assumed using the Cognito token is required. The following constraints apply to the trust policy of the role:
- restrict the audience to the specific Cognito Identity Pool
- require a session name in the format of `runID@SHA` to ensure uniqueness and traceability of sessions
- prevent use of the role when the GitHub workflow is triggered by a `pull_request` event (parity with GitHub - `pull_request` workflows do not have access to secrets/variables)
- allow tagging of sessions so role and resourced policies can use ABAC based on claims mapped from the GitHub token

<details>
<summary>Example IAM Role Trust Policy</summary>

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "",
            "Effect": "Allow",
            "Principal": {
                "Federated": "cognito-identity.amazonaws.com"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "aws:RequestTag/enterprise": "${GITHUB_ORG}",
                    "cognito-identity.amazonaws.com:aud": "us-west-2:12345678-1234-1234-1234-1234567890ab",
                    "sts:RoleSessionName": "${aws:RequestTag/run_id}@${aws:RequestTag/sha}"
                },
                "StringNotEquals": {
                    "aws:RequestTag/event_name": "pull_request"
                }
            }
        },
        {
            "Sid": "",
            "Effect": "Allow",
            "Principal": {
                "Federated": "cognito-identity.amazonaws.com"
            },
            "Action": "sts:TagSession",
            "Condition": {
                "StringEquals": {
                    "cognito-identity.amazonaws.com:aud": "us-west-2:12345678-1234-1234-1234-1234567890ab"
                }
            }
        }
    ]
}
```
</details>

Role policy should allow `secretsmanager:GetSecretValue` for secrets that are tagged with the appropriate GitHub session tags.

<details>
<summary>Example IAM Role Policy</summary>

```json
{
    "Statement": [
        {
            "Sid": "AllowReadSecretsBasedOnSessionTags"
            "Action": "secretsmanager:GetSecretValue",
            "Effect": "Allow",
            "Resource": [
                "*"
            ],
            "Condition": {
                "StringEquals": {
                    "aws:ResourceTag/github:enterprise": "${aws:PrincipalTag/enterprise}",
                    "aws:ResourceTag/github:repository": "${aws:PrincipalTag/repository}"
                },
                "StringEqualsIfExists": {
                    "aws:ResourceTag/github:environment": "${aws:PrincipalTag/environment}"
                }
            },
        }
    ],
}
```

</details>

## Trust Model

Each workflow using this action is initially identified by the token provided by GitHub. The claims within the token identify properties of the workflow, such as the repository, environment, workflow name and more.

A Cognito Identity Pool is configured to trust GitHub as an OpenID Connect (OIDC) provider. The Identity Pool provide a Cognito token in exchange for a GitHub token. Claims from the GitHub token are mapped to a claim structure in the Cognito token that will become session tags when assuming an AWS role.

Once a Cognito token is used to assume an AWS role with session tags, IAM policies for that role can use ABAC to constrain permissions within namespaces associated with a workflow, such as repository or environment.

### GHA Token Claims

The following claims are included in the GitHub token and can be mapped to Cognito token claims and then session tags when assuming an AWS role:

```json
{
        "actor": "github_username",
        "actor_id": "12345678",
        "aud": "cognito-identity.amazonaws.com",
        "base_ref": "",
        "check_run_id": "65204598612",
        "enterprise": "github_org",
        "enterprise_id": "9266",
        "event_name": "workflow_dispatch",
        "exp": "2026-02-27T22:23:34Z",
        "head_ref": "",
        "iat": "2026-02-27T22:18:34Z",
        "iss": "https://token.actions.githubusercontent.com/github_org",
        "issuer_scope": "enterprise",
        "job_workflow_ref": "org_slug/repo/.github/workflows/dispatch.yaml@refs/heads/main",
        "job_workflow_sha": "1c6fb9ebf8734a38050775c4522f9e10f396efc4",
        "jti": "4d3dbc09-8da0-4065-b311-3b8481d3174b",
        "nbf": "2026-02-27T22:13:34Z",
        "ref": "refs/heads/main",
        "ref_protected": "false",
        "ref_type": "branch",
        "repository": "org_slug/repo",
        "repository_id": "12345678",
        "repository_owner": "org_slug",
        "repository_owner_id": "12345678",
        "repository_visibility": "internal",
        "run_attempt": "1",
        "run_id": "22505998282",
        "run_number": "90",
        "runner_environment": "github-hosted",
        "sha": "1c6fb9ebf8734a38050775c4522f9e10f396efc4",
        "sub": "repo:org_slug/repo:ref:refs/heads/main",
        "workflow": "dispatch",
        "workflow_ref": "org_slug/repo/.github/workflows/dispatch.yaml@refs/heads/main",
        "workflow_sha": "1c6fb9ebf8734a38050775c4522f9e10f396efc4"
    }
```

### Cognito Token Claims

When using a principal tag mapping similar to the [example above](#Cognito Identity Pool), the Cognito token will have claims that mirror the GitHub token claims, but structured so AWS will assign them as session tags during role assumption.

```json
  {
        "amr": [
            "authenticated",
            "token.actions.githubusercontent.com/github_org",
            "arn:aws:iam::123456789012:oidc-provider/token.actions.githubusercontent.com/github_org:OIDC:repo:org_slug/repo:ref:refs/heads/main"
        ],
        "aud": "us-west-2:12345678-1234-1234-1234-1234567890ab",
        "exp": "2026-02-27T22:28:34Z",
        "https://aws.amazon.com/tags": {
            "principal_tags": {
                "actor": [
                    "github_username"
                ],
                "enterprise": [
                    "github_org"
                ],
                "event_name": [
                    "workflow_dispatch"
                ],
                "repository": [
                    "org_slug/repo"
                ],
                "run_id": [
                    "22505998282"
                ],
                "sha": [
                    "1c6fb9ebf8734a38050775c4522f9e10f396efc4"
                ],
                "workflow": [
                    "dispatch"
                ]
            }
        },
        "https://cognito-identity.amazonaws.com/identity-pool-arn": "arn:aws:cognito-identity:us-west-2:123456789012:identitypool/us-west-2:12345678-1234-1234-1234-1234567890ab",
        "iat": "2026-02-27T22:18:34Z",
        "iss": "https://cognito-identity.amazonaws.com",
        "sub": "us-west-2:87654321-4321-4321-4321-1234567890ab"
    }
```