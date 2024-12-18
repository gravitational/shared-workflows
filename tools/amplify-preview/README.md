# amplify-preview


This gha-tool is basically re-implements what [AWS Amplify's GitHub integration should be doing](https://docs.aws.amazon.com/amplify/latest/userguide/pr-previews.html),
however because of following limitations, we can't really use it for some of the repos:
- [No way to filter for which PRs to generate preview deployments](https://github.com/aws-amplify/amplify-hosting/issues/3960)
- [Hard limit of 50 preview branches per amplify app][https://docs.aws.amazon.com/amplify/latest/userguide/quotas-chapter.html]
- [No way to create PR preview programmatically](https://github.com/aws-amplify/amplify-hosting/issues/3963)

This action accepts of AWS Amplify App IDs, checks if current git branch is connected to the apps and posts deployment status and PR preview in PR comments.

If `--create-branches` is enabled, then it will also connect git branch to one of the AWS Amplify apps (where hard limit of 50 branches hasn't been reached yet) and kick of new build.
If `--wait` is enabled, then it will also wait for deployment to be completed and fail the GHA run if deployment had failed.

## Usage

```shell
usage: amplify-preview --amplify-app-ids=AMPLIFY-APP-IDS --git-branch-name=GIT-BRANCH-NAME [<flags>]

Flags:
  --[no-]help            Show context-sensitive help (also try --help-long and --help-man).
  --amplify-app-ids=AMPLIFY-APP-IDS ...  
                         List of Amplify App IDs ($AMPLIFY_APP_IDS)
  --git-branch-name=GIT-BRANCH-NAME  
                         Git branch name ($GIT_BRANCH_NAME)
  --[no-]create-branches  Defines whether Amplify branches should be created if missing, or just lookup existing ones ($CREATE_BRANCHES)
  --[no-]wait            Wait for pending/running job to complete ($WAIT)
```

Example GHA workflow:

```yaml
name: Amplify Preview
on:
  pull_request:
    paths:
      - 'docs/**'
  workflow_dispatch:
  
permissions:
  # Permissions to write PR comment
  pull-requests: write
  id-token: write
  
jobs:
  amplify-preview:
    name: Get and post Amplify preview URL
    runs-on: ubuntu-22.04-2core-arm64
    environment: docs-amplify
    steps:
    - name: Checkout shared-workflow
      uses: actions/checkout@v4
      with:
        repository: gravitational/shared-workflows
        sparse-checkout: |
          tools

    - name: Configure AWS credentials
      uses: aws-actions/configure-aws-credentials@e3dd6a429d7300a6a4c196c26e071d42e0343502 # v4
      with:
        aws-region: us-west-2
        role-to-assume: ${{ vars.IAM_ROLE }}

    - name: Check Amplify job status test
      uses: ./tools/amplify-preview
      with:
        app_ids: ${{ vars.AMPLIFY_APP_IDS }}
        create_branches: "true"
        github_token: ${{ secrets.GITHUB_TOKEN }}
        wait: "true"
```

## AWS Permissions

For this action to work, AWS role with following IAM permissions is required:
```json
{
    "Statement": [
        {
            "Action": [
                "amplify:CreateBranch",
                "amplify:GetBranch",
                "amplify:ListJobs"
                "amplify:StartJob",
            ],
            "Effect": "Allow",
            "Resource": [
                "arn:aws:amplify:<region>:<account_id>:apps/<app_id>/branches/*"
            ]
        }
    ],
    "Version": "2012-10-17"
}
```

Where `amplify:CreateBranch` and `amplify:StartJob` are needed only when `--create-branches` is enabled.
