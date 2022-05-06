# Teleport Shared GitHub Actions Workflows

GitHub Actions shared within the `gravitational` organization.

To use one of these workflows:

```yaml
jobs:
  call-workflow-passing-data:
    uses: gravitational/shared-workflows/.github/workflows/reusable-workflow.yml@main
    secrets: inherit
```
